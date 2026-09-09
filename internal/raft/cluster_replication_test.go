package raft

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"
)

type replicationRecorder struct {
	mu       sync.Mutex
	commands []Command
}

func (r *replicationRecorder) apply(cmd Command) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.commands = append(r.commands, cmd)
}

func (r *replicationRecorder) snapshot() []Command {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]Command(nil), r.commands...)
}

func newReplicationCluster(t *testing.T, size int) (*TestCluster, map[string]*replicationRecorder) {
	t.Helper()
	c := NewTestCluster(t, size)
	recorders := make(map[string]*replicationRecorder)
	for _, id := range c.ids {
		recorder := &replicationRecorder{}
		recorders[id] = recorder
		// Los callbacks se instalan antes de iniciar cualquier elección o RPC.
		c.nodes[id].apply = recorder.apply
	}
	c.Start()
	return c, recorders
}

func proposeClusterCommand(t *testing.T, node *Node, cmd Command) []LogEntry {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := node.Propose(ctx, cmd); err != nil {
		t.Fatalf("no se pudo confirmar la propuesta %+v: %v", cmd, err)
	}
	node.mu.Lock()
	defer node.mu.Unlock()
	if node.commitIndex == 0 || node.log[node.commitIndex-1].Command != cmd {
		t.Fatalf("la confirmación no corresponde al comando propuesto: %+v", cmd)
	}
	return append([]LogEntry(nil), node.log[:node.commitIndex]...)
}

func waitForReplication(t *testing.T, description string, ready func() (bool, string)) {
	t.Helper()
	timer := time.NewTimer(15 * time.Second)
	defer timer.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		ok, detail := ready()
		if ok {
			return
		}
		select {
		case <-timer.C:
			t.Fatalf("se agotó el plazo para %s: %s", description, detail)
		case <-ticker.C:
		}
	}
}

// assertCommittedPrefix compara índices, términos y comandos, además de los
// efectos aplicados. Permite barreras NOOP posteriores por nuevas elecciones.
func assertCommittedPrefix(t *testing.T, c *TestCluster, recorders map[string]*replicationRecorder, ids []string, want []LogEntry, commands []Command) {
	t.Helper()
	waitForReplication(t, "replicar y aplicar el prefijo confirmado", func() (bool, string) {
		for _, id := range ids {
			node := c.nodes[id]
			node.mu.Lock()
			committed, applied := node.commitIndex, node.lastApplied
			matches := len(node.log) >= len(want) && reflect.DeepEqual(node.log[:len(want)], want)
			node.mu.Unlock()
			if !matches || committed < len(want) || applied < len(want) {
				return false, fmt.Sprintf("%s: prefijo coincidente=%t, confirmado=%d, aplicado=%d, esperado=%d", id, matches, committed, applied, len(want))
			}
			if got := recorders[id].snapshot(); !reflect.DeepEqual(got, commands) {
				return false, fmt.Sprintf("%s: comandos aplicados=%+v, esperados=%+v", id, got, commands)
			}
		}
		return true, ""
	})
	for _, id := range ids {
		entries, err := c.nodes[id].wal.LoadLog()
		if err != nil {
			t.Fatalf("no se pudo leer el WAL de %s: %v", id, err)
		}
		if len(entries) < len(want) || !reflect.DeepEqual(entries[:len(want)], want) {
			t.Fatalf("el WAL de %s no conserva el prefijo confirmado", id)
		}
	}
}

func TestSingleEntryReplication(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	leader := c.nodes[c.WaitForLeader()]
	cmd := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, leader, cmd)
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{cmd})
}

func TestMultipleEntryReplication(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	leader := c.nodes[c.WaitForLeader()]
	var prefix []LogEntry
	var commands []Command
	for i := 1; i <= 5; i++ {
		cmd := Command{Op: "SET", Key: "saldo", Value: fmt.Sprint(i)}
		commands = append(commands, cmd)
		prefix = proposeClusterCommand(t, leader, cmd)
	}
	// El registro de callbacks detecta pérdidas, duplicados y cambios de orden
	// que quedarían ocultos si solo se comprobara el valor final de SET.
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, commands)
}

func TestMajorityCommit(t *testing.T) {
	c, recorders := newReplicationCluster(t, 5)
	leaderID := c.WaitForLeader()
	leader := c.nodes[leaderID]
	var followers []string
	for _, id := range c.ids {
		if id != leaderID {
			followers = append(followers, id)
		}
	}
	c.Disconnect(followers[2])
	c.Disconnect(followers[3])
	confirmed := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, leader, confirmed)
	assertCommittedPrefix(t, c, recorders, []string{leaderID, followers[0], followers[1]}, prefix, []Command{confirmed})

	// Dos nodos pueden almacenar la propuesta, pero no forman mayoría de cinco.
	c.Disconnect(followers[1])
	unconfirmed := Command{Op: "SET", Key: "saldo", Value: "200"}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := leader.Propose(ctx, unconfirmed); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("se esperaba agotar el plazo sin mayoría, se obtuvo: %v", err)
	}
	for _, id := range []string{leaderID, followers[0]} {
		node := c.nodes[id]
		waitForReplication(t, "observar la entrada sin confirmar en "+id, func() (bool, string) {
			node.mu.Lock()
			defer node.mu.Unlock()
			index := len(prefix)
			found := len(node.log) > index && node.log[index].Command == unconfirmed
			return found, fmt.Sprintf("longitud del log=%d, índice esperado=%d", len(node.log), index+1)
		})
		node.mu.Lock()
		committed, applied := node.commitIndex, node.lastApplied
		node.mu.Unlock()
		if committed != len(prefix) || applied != len(prefix) {
			t.Fatalf("%s avanzó sin mayoría: confirmado=%d, aplicado=%d, esperado=%d", id, committed, applied, len(prefix))
		}
		if got := recorders[id].snapshot(); !reflect.DeepEqual(got, []Command{confirmed}) {
			t.Fatalf("%s aplicó comandos inesperados sin mayoría: %+v", id, got)
		}
	}
}

func TestFollowerCatchUp(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	leaderID := c.WaitForLeader()
	leader := c.nodes[leaderID]
	var followerID string
	for _, id := range c.ids {
		if id != leaderID {
			followerID = id
			break
		}
	}
	c.Disconnect(followerID)
	var prefix []LogEntry
	var commands []Command
	for i := 1; i <= 3; i++ {
		cmd := Command{Op: "SET", Key: "saldo", Value: fmt.Sprint(i)}
		commands = append(commands, cmd)
		prefix = proposeClusterCommand(t, leader, cmd)
	}
	if got := recorders[followerID].snapshot(); len(got) != 0 {
		t.Fatalf("el seguidor aislado recibió comandos nuevos: %+v", got)
	}
	c.Reconnect(followerID)
	// El nodo aislado pudo incrementar su término. Se permite otra elección
	// antes de exigir convergencia, sin fijar quién debe conservar el liderazgo.
	c.WaitForLeader()
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, commands)
}
