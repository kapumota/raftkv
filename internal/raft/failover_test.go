package raft

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"testing"
	"time"
)

type testStateMachine struct {
	mu     sync.Mutex
	values map[string]string
}

func newTestStateMachine() *testStateMachine {
	return &testStateMachine{values: map[string]string{}}
}

func (s *testStateMachine) apply(cmd Command) {
	if cmd.Op != "SET" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[cmd.Key] = cmd.Value
}

func (s *testStateMachine) get(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	value, ok := s.values[key]
	return value, ok
}

func stopNodeLoop(node *Node) {
	select {
	case <-node.stopCh:
	default:
		close(node.stopCh)
	}
}

func TestCommittedStateSurvivesNodeRestart(t *testing.T) {
	dataDir := t.TempDir()

	initialWAL, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL inicial: %v", err)
	}
	initialMachine := newTestStateMachine()
	initialNode := NewNode("node-1", nil, initialWAL, NewTransport(), initialMachine.apply)
	initialNode.mu.Lock()
	initialNode.currentTerm = 1
	initialNode.state = Leader
	initialNode.mu.Unlock()

	original := Command{Op: "SET", Key: "saldo", Value: "100"}
	if err := initialNode.Propose(context.Background(), original); err != nil {
		t.Fatalf("la escritura inicial no pudo confirmarse: %v", err)
	}
	value, ok := initialMachine.get("saldo")
	if !ok || value != "100" {
		t.Fatalf("valor inicial inesperado: se obtuvo %q, presente=%t", value, ok)
	}

	// Simula un reinicio descartando el estado en memoria y abriendo nuevamente
	// los archivos persistentes desde el mismo directorio de datos.
	recoveredWAL, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo reabrir el WAL: %v", err)
	}
	recoveredMachine := newTestStateMachine()
	recoveredNode := NewNode("node-1", nil, recoveredWAL, NewTransport(), recoveredMachine.apply)

	recoveredValue, recovered := recoveredMachine.get("saldo")
	if !recovered {
		t.Fatal("la escritura confirmada no fue reconstruida después del reinicio")
	}
	if recoveredValue != "100" {
		t.Fatalf("valor recuperado inesperado: se obtuvo %q, se esperaba %q", recoveredValue, "100")
	}

	_, _, term, _, commitIndex := recoveredNode.Status()
	if term != 1 {
		t.Fatalf("término recuperado inesperado: se obtuvo %d, se esperaba 1", term)
	}
	if commitIndex != 1 {
		t.Fatalf("índice de commit recuperado inesperado: se obtuvo %d, se esperaba 1", commitIndex)
	}
}

func TestIsolatedLeaderCannotServeLinearizableRead(t *testing.T) {
	wal, err := NewWAL(t.TempDir())
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}
	leader := NewNode(
		"node-1",
		[]string{"http://127.0.0.1:1", "http://127.0.0.1:2"},
		wal,
		NewTransport(),
		nil,
	)
	t.Cleanup(func() { stopNodeLoop(leader) })
	leader.mu.Lock()
	leader.currentTerm = 1
	leader.becomeLeader()
	leader.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()
	readIndex, err := leader.ReadIndex(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error inesperado para el líder aislado: se obtuvo %v, se esperaba context.DeadlineExceeded", err)
	}
	if readIndex != 0 {
		t.Fatalf("el líder aislado devolvió un índice de lectura: se obtuvo %d, se esperaba 0", readIndex)
	}
}

func TestAcknowledgedWriteSurvivesLeaderFailure(t *testing.T) {
	const nodeCount = 3

	listeners := make([]net.Listener, nodeCount)
	urls := make([]string, nodeCount)
	for i := 0; i < nodeCount; i++ {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("no se pudo crear el listener del nodo %d: %v", i+1, err)
		}
		listeners[i] = listener
		urls[i] = "http://" + listener.Addr().String()
	}

	nodes := make([]*Node, nodeCount)
	servers := make([]*http.Server, nodeCount)
	machines := make([]*testStateMachine, nodeCount)

	for i := 0; i < nodeCount; i++ {
		peers := make([]string, 0, nodeCount-1)
		for j := 0; j < nodeCount; j++ {
			if i != j {
				peers = append(peers, urls[j])
			}
		}

		wal, err := NewWAL(t.TempDir())
		if err != nil {
			t.Fatalf("no se pudo crear el WAL del nodo %d: %v", i+1, err)
		}
		machines[i] = newTestStateMachine()
		transport := NewTransport()
		transport.client.Timeout = time.Second
		nodes[i] = NewNode(
			fmt.Sprintf("node-%d", i+1),
			peers,
			wal,
			transport,
			machines[i].apply,
		)

		mux := http.NewServeMux()
		RegisterRPCHandlers(mux, nodes[i])
		servers[i] = &http.Server{Handler: mux}
		go func(server *http.Server, listener net.Listener) {
			_ = server.Serve(listener)
		}(servers[i], listeners[i])
	}

	t.Cleanup(func() {
		for _, node := range nodes {
			stopNodeLoop(node)
		}
		for _, server := range servers {
			_ = server.Close()
		}
	})

	leader := nodes[0]
	leader.mu.Lock()
	leader.currentTerm = 1
	leader.becomeLeader()
	leader.mu.Unlock()
	barrierCtx, barrierCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer barrierCancel()
	if err := leader.ConfirmLeadership(barrierCtx); err != nil {
		t.Fatalf("el líder inicial no pudo confirmar su barrera: %v", err)
	}

	original := Command{Op: "SET", Key: "saldo", Value: "100"}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := leader.Propose(ctx, original); err != nil {
		t.Fatalf("la escritura confirmada falló: %v", err)
	}

	leader.mu.Lock()
	if leader.commitIndex != 2 {
		leader.mu.Unlock()
		t.Fatalf("índice de commit inesperado antes de la caída: se obtuvo %d, se esperaba 2", leader.commitIndex)
	}
	leader.mu.Unlock()

	var candidate *Node
	var candidateMachine *testStateMachine
	for i := 1; i < nodeCount; i++ {
		nodes[i].mu.Lock()
		hasEntry := len(nodes[i].log) >= 2 && nodes[i].log[1].Command == original
		nodes[i].mu.Unlock()
		if hasEntry {
			candidate = nodes[i]
			candidateMachine = machines[i]
			break
		}
	}
	if candidate == nil {
		t.Fatal("ningún seguidor conservó la escritura confirmada")
	}

	_ = servers[0].Close()
	stopNodeLoop(leader)

	var isLeader bool
	for attempt := 0; attempt < 3; attempt++ {
		candidate.startElection()
		candidate.mu.Lock()
		isLeader = candidate.state == Leader
		candidate.mu.Unlock()
		if isLeader {
			break
		}
	}
	candidate.mu.Lock()
	term := candidate.currentTerm
	entryPreserved := len(candidate.log) >= 2 && candidate.log[1].Command == original
	candidate.mu.Unlock()

	if !isLeader {
		t.Fatal("el seguidor con la escritura confirmada no ganó la nueva elección")
	}
	if term < 2 {
		t.Fatalf("término inesperado después de la nueva elección: se obtuvo %d, se esperaba al menos 2", term)
	}
	if !entryPreserved {
		t.Fatal("la escritura confirmada desapareció después de la caída del líder")
	}

	// La barrera del nuevo líder permite confirmar también las entradas creadas
	// en términos anteriores; la escritura posterior verifica que puede avanzar.
	followUp := Command{Op: "SET", Key: "marcador", Value: "1"}
	followUpCtx, followUpCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer followUpCancel()
	if err := candidate.Propose(followUpCtx, followUp); err != nil {
		t.Fatalf("el nuevo líder no pudo confirmar una escritura posterior: %v", err)
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer readCancel()
	readIndex, err := candidate.ReadIndex(readCtx)
	if err != nil {
		t.Fatalf("el nuevo líder no pudo confirmar una lectura linealizable: %v", err)
	}
	if readIndex < 2 {
		t.Fatalf("índice insuficiente para observar la escritura original: se obtuvo %d, se esperaba al menos 2", readIndex)
	}

	value, ok := candidateMachine.get("saldo")
	if !ok {
		t.Fatal("la escritura confirmada no fue aplicada por el nuevo líder")
	}
	if value != "100" {
		t.Fatalf("valor inesperado después del failover: se obtuvo %q, se esperaba %q", value, "100")
	}
}
