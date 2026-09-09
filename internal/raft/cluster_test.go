package raft

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// TestCluster controla nodos reales con WAL independientes y RPC en memoria.
// Sus métodos de control se invocan desde la goroutine de la prueba.
// Stop es definitivo; para otro arranque se crea un clúster nuevo.
type TestCluster struct {
	t       testing.TB
	nodes   map[string]*Node
	ids     []string
	network *clusterNetwork
	workers sync.WaitGroup
	started bool
	stopped bool
}

type clusterNetwork struct {
	mu       sync.RWMutex
	handlers map[string]http.Handler
	blocked  map[string]bool
	stopped  bool
}

type clusterTransport struct {
	network *clusterNetwork
	source  string
}

// RoundTrip conserva la codificación y los handlers HTTP de producción.
// El bloqueo permite terminar las RPC aceptadas antes de aislar un nodo.
// No simula latencias, pérdida de paquetes ni conexiones TCP.
func (r *clusterTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req.Body != nil {
		defer req.Body.Close()
	}
	r.network.mu.RLock()
	defer r.network.mu.RUnlock()
	target := req.URL.Host
	if r.network.stopped || r.network.blocked[r.source] || r.network.blocked[target] {
		return nil, fmt.Errorf("enlace desconectado entre %s y %s", r.source, target)
	}
	if err := req.Context().Err(); err != nil {
		return nil, err
	}
	handler, ok := r.network.handlers[target]
	if !ok {
		return nil, fmt.Errorf("nodo desconocido: %s", target)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

// NewTestCluster registra Stop antes de crear recursos para cubrir fallos
// parciales. Cada prueba conserva sus datos en un directorio temporal propio.
func NewTestCluster(t testing.TB, size int) *TestCluster {
	t.Helper()
	if size < 1 {
		t.Fatal("el clúster debe tener al menos un nodo")
	}
	c := &TestCluster{
		t:     t,
		nodes: make(map[string]*Node),
		network: &clusterNetwork{
			handlers: make(map[string]http.Handler),
			blocked:  make(map[string]bool),
		},
	}
	dir := t.TempDir()
	t.Cleanup(c.Stop)
	for i := 0; i < size; i++ {
		c.ids = append(c.ids, fmt.Sprintf("nodo-%d", i+1))
	}
	for _, id := range c.ids {
		var peers []string
		for _, other := range c.ids {
			if other != id {
				peers = append(peers, "http://"+other)
			}
		}
		wal, err := NewWAL(filepath.Join(dir, id))
		if err != nil {
			t.Fatalf("no se pudo crear el WAL de %s: %v", id, err)
		}
		transport := NewTransport()
		transport.client.Transport = &clusterTransport{network: c.network, source: id}
		node := NewNode(id, peers, wal, transport, nil)
		c.nodes[id] = node
		mux := http.NewServeMux()
		RegisterRPCHandlers(mux, node)
		c.network.handlers[id] = mux
	}
	return c
}

func (c *TestCluster) Start() {
	c.t.Helper()
	if c.stopped {
		c.t.Fatal("no se puede iniciar un clúster detenido")
	}
	if c.started {
		return
	}
	c.started = true
	for _, id := range c.ids {
		node := c.nodes[id]
		node.mu.Lock()
		node.resetElectionTimer()
		node.mu.Unlock()
		c.workers.Add(1)
		go func(n *Node) {
			defer c.workers.Done()
			n.electionTicker()
		}(node)
	}
}

func (c *TestCluster) Stop() {
	if c.stopped {
		return
	}
	c.stopped = true
	c.network.mu.Lock()
	c.network.stopped = true
	c.network.mu.Unlock()
	for _, node := range c.nodes {
		close(node.stopCh)
	}
	// Espera las elecciones en curso antes de impedir nuevos efectos locales.
	c.workers.Wait()
	for _, node := range c.nodes {
		node.mu.Lock()
		node.state = Follower
		node.failPendingCommits(ErrNotLeader)
		node.mu.Unlock()
	}
}

// Disconnect corta las RPC entrantes y salientes; el nodo sigue ejecutándose.
// Las RPC completadas antes del corte aún pueden ser procesadas por el emisor.
func (c *TestCluster) Disconnect(nodeID string) {
	c.setDisconnected(nodeID, true)
}

func (c *TestCluster) Reconnect(nodeID string) {
	c.setDisconnected(nodeID, false)
}

func (c *TestCluster) setDisconnected(nodeID string, disconnected bool) {
	c.t.Helper()
	if _, ok := c.nodes[nodeID]; !ok {
		c.t.Fatalf("nodo desconocido: %s", nodeID)
	}
	c.network.mu.Lock()
	c.network.blocked[nodeID] = disconnected
	c.network.mu.Unlock()
}

// WaitForLeader busca un líder conectado que confirme su liderazgo por quorum.
// No demuestra unicidad por término: esa propiedad requiere una prueba propia.
func (c *TestCluster) WaitForLeader() string {
	c.t.Helper()
	if !c.started || c.stopped {
		c.t.Fatal("el clúster debe estar iniciado para esperar un líder")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		for _, id := range c.ids {
			c.network.mu.RLock()
			blocked := c.network.blocked[id]
			c.network.mu.RUnlock()
			if blocked {
				continue
			}
			node := c.nodes[id]
			node.mu.Lock()
			leader := node.state == Leader
			node.mu.Unlock()
			if leader {
				round, stop := context.WithTimeout(ctx, time.Second)
				err := node.ConfirmLeadership(round)
				stop()
				if err == nil {
					return id
				}
			}
		}
		select {
		case <-ctx.Done():
			for _, id := range c.ids {
				_, state, term, _, committed := c.nodes[id].Status()
				c.t.Logf("%s: estado=%s, término=%d, índice confirmado=%d", id, state, term, committed)
			}
			c.t.Fatal("no se encontró un líder con quorum dentro del plazo")
			return ""
		case <-ticker.C:
		}
	}
}

func TestClusterLifecycle(t *testing.T) {
	c := NewTestCluster(t, 3)
	c.Start()
	c.Start()
	if id := c.WaitForLeader(); c.nodes[id] == nil {
		t.Fatalf("identificador de líder desconocido: %s", id)
	}
	c.Stop()
	c.Stop()
	if _, err := c.nodes[c.ids[0]].transport.SendRequestVote("http://"+c.ids[1], RequestVoteArgs{}); err == nil {
		t.Fatal("el clúster detenido aceptó una RPC")
	}
}

func TestClusterDisconnectReconnect(t *testing.T) {
	c := NewTestCluster(t, 3)
	check := func(source, target string, allowed bool) {
		t.Helper()
		_, err := c.nodes[source].transport.SendRequestVote("http://"+target, RequestVoteArgs{})
		if (err == nil) != allowed {
			t.Fatalf("enlace %s a %s: permitido=%t, error=%v", source, target, allowed, err)
		}
	}
	a, b, d := c.ids[0], c.ids[1], c.ids[2]
	check(a, b, true)
	c.Disconnect(a)
	check(a, b, false)
	check(b, a, false)
	check(b, d, true)
	c.Reconnect(a)
	check(a, b, true)
	check(b, a, true)
}
