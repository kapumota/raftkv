package raft

import (
	"context"
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestConflictingSuffixIsRepaired(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	oldID := c.WaitForLeader()
	old := c.nodes[oldID]
	initial := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, old, initial)
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial})
	c.Disconnect(oldID)
	for _, value := range []string{"pendiente-1", "pendiente-2"} {
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		err := old.Propose(ctx, Command{Op: "SET", Key: "saldo", Value: value})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("se esperaba una propuesta pendiente sin mayoría, se obtuvo: %v", err)
		}
	}
	old.mu.Lock()
	divergent := append([]LogEntry(nil), old.log...)
	committed := old.commitIndex
	old.mu.Unlock()
	if len(divergent) != len(prefix)+2 || committed != len(prefix) {
		t.Fatalf("no se construyó el sufijo pendiente: longitud=%d, confirmado=%d", len(divergent), committed)
	}
	newID := c.WaitForLeader()
	if newID == oldID {
		t.Fatal("el líder aislado conservó el quorum")
	}
	updated := Command{Op: "SET", Key: "saldo", Value: "200"}
	want := proposeClusterCommand(t, c.nodes[newID], updated)
	assertCommittedPrefix(t, c, recorders, clusterOtherNodes(c, oldID), want, []Command{initial, updated})
	// Acredita un conflicto real en el mismo índice, antes de reparar el log.
	if len(want) <= len(prefix) || divergent[len(prefix)].Term == want[len(prefix)].Term {
		t.Fatal("el escenario no produjo entradas con términos conflictivos")
	}
	c.Reconnect(oldID)
	c.WaitForLeader()
	assertCommittedPrefix(t, c, recorders, c.ids, want, []Command{initial, updated})
	c.Stop()
	for _, id := range c.ids {
		entries, err := c.nodes[id].wal.LoadLog()
		if err != nil {
			t.Fatalf("no se pudo leer el WAL de %s: %v", id, err)
		}
		for _, entry := range entries {
			if entry.Command.Value == "pendiente-1" || entry.Command.Value == "pendiente-2" {
				t.Fatalf("%s conservó una entrada del sufijo descartado: %+v", id, entry)
			}
		}
	}
	// Otra máquina de estados reconstruye el WAL reparado, sin reutilizar datos.
	recoveredWAL, err := NewWAL(filepath.Dir(old.wal.logPath))
	if err != nil {
		t.Fatalf("no se pudo reabrir el WAL reparado: %v", err)
	}
	recovered := &replicationRecorder{}
	NewNode(oldID, nil, recoveredWAL, NewTransport(), recovered.apply)
	if got := recovered.snapshot(); !reflect.DeepEqual(got, []Command{initial, updated}) {
		t.Fatalf("el reinicio reconstruyó comandos inesperados: %+v", got)
	}
}

func TestStaleTermIsRejected(t *testing.T) {
	var applied []Command
	node := newTestNode(t, func(cmd Command) { applied = append(applied, cmd) })
	entry := LogEntry{Index: 1, Term: 5, Command: Command{Op: "SET", Key: "saldo", Value: "100"}}
	if reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 5, Entries: []LogEntry{entry}, LeaderCommit: 1}); !reply.Success {
		t.Fatal("se rechazó la entrada inicial")
	}
	if reply := node.HandleRequestVote(RequestVoteArgs{Term: 5, CandidateID: "candidato-actual", LastLogIndex: 1, LastLogTerm: 5}); !reply.VoteGranted {
		t.Fatal("no se pudo registrar el voto del término actual")
	}
	before, err := node.wal.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el estado inicial: %v", err)
	}
	electionReset := node.electionResetAt
	appendReply := node.HandleAppendEntries(AppendEntriesArgs{
		Term: 4, LeaderID: "líder-antiguo", LeaderCommit: 2,
		Entries: []LogEntry{{Index: 1, Term: 4, Command: Command{Op: "SET", Key: "saldo", Value: "obsoleto"}}},
	})
	if appendReply.Success || appendReply.Term != 5 {
		t.Fatalf("respuesta inesperada a AppendEntries antiguo: %+v", appendReply)
	}
	// Incluso un log anunciado como más reciente no permite votar en un término antiguo.
	voteReply := node.HandleRequestVote(RequestVoteArgs{Term: 4, CandidateID: "candidato-antiguo", LastLogIndex: 100, LastLogTerm: 100})
	if voteReply.VoteGranted || voteReply.Term != 5 {
		t.Fatalf("respuesta inesperada a RequestVote antiguo: %+v", voteReply)
	}
	after, err := node.wal.LoadState()
	if err != nil || before != after {
		t.Fatalf("las peticiones antiguas alteraron el estado persistente: antes=%+v, después=%+v, error=%v", before, after, err)
	}
	entries, err := node.wal.LoadLog()
	if err != nil || !reflect.DeepEqual(entries, []LogEntry{entry}) || !reflect.DeepEqual(node.log, entries) {
		t.Fatalf("las peticiones antiguas alteraron el log: %+v, error=%v", entries, err)
	}
	if node.currentTerm != 5 || node.votedFor != before.VotedFor || node.commitIndex != 1 || node.lastApplied != 1 || !node.electionResetAt.Equal(electionReset) {
		t.Fatal("las peticiones antiguas alteraron el estado en memoria o el temporizador")
	}
	if !reflect.DeepEqual(applied, []Command{entry.Command}) {
		t.Fatalf("las peticiones antiguas produjeron efectos: %+v", applied)
	}
}

func TestCommittedEntryIsPreserved(t *testing.T) {
	var applied []Command
	node := newTestNode(t, func(cmd Command) { applied = append(applied, cmd) })
	entries := []LogEntry{
		{Index: 1, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "100"}},
		{Index: 2, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "200"}},
		{Index: 3, Term: 2, Command: Command{Op: "SET", Key: "saldo", Value: "pendiente"}},
	}
	if reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 2, Entries: entries, LeaderCommit: 2}); !reply.Success {
		t.Fatal("se rechazó el log inicial")
	}
	// Inyecta una petición inválida que intenta reemplazar una entrada confirmada.
	conflict := LogEntry{Index: 2, Term: 3, Command: Command{Op: "SET", Key: "saldo", Value: "inválido"}}
	reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 3, PrevLogIndex: 1, PrevLogTerm: 1, Entries: []LogEntry{conflict}, LeaderCommit: 2})
	if reply.Success || reply.Term != 3 {
		t.Fatalf("no se rechazó el conflicto con el prefijo confirmado: %+v", reply)
	}
	persisted, err := node.wal.LoadLog()
	if err != nil || !reflect.DeepEqual(persisted, entries) || !reflect.DeepEqual(node.log, entries) {
		t.Fatalf("el rechazo alteró el log: %+v, error=%v", persisted, err)
	}
	wantCommands := []Command{entries[0].Command, entries[1].Command}
	if node.commitIndex != 2 || node.lastApplied != 2 || !reflect.DeepEqual(applied, wantCommands) {
		t.Fatal("el rechazo alteró el estado confirmado o sus efectos")
	}
	// El mismo nodo aún debe poder reparar únicamente la parte pendiente.
	replacement := LogEntry{Index: 3, Term: 3, Command: Command{Op: "SET", Key: "saldo", Value: "300"}}
	reply = node.HandleAppendEntries(AppendEntriesArgs{Term: 3, PrevLogIndex: 2, PrevLogTerm: 1, Entries: []LogEntry{replacement}, LeaderCommit: 3})
	if !reply.Success {
		t.Fatal("se rechazó la reparación del sufijo pendiente")
	}
	wantLog := []LogEntry{entries[0], entries[1], replacement}
	wantCommands = append(wantCommands, replacement.Command)
	if !reflect.DeepEqual(applied, wantCommands) {
		t.Fatalf("se reaplicó el prefijo o se perdió el comando nuevo: %+v", applied)
	}
	wal, err := NewWAL(filepath.Dir(node.wal.logPath))
	if err != nil {
		t.Fatalf("no se pudo reabrir el WAL: %v", err)
	}
	var recovered []Command
	restarted := NewNode("nodo-recuperado", nil, wal, NewTransport(), func(cmd Command) { recovered = append(recovered, cmd) })
	if !reflect.DeepEqual(restarted.log, wantLog) || !reflect.DeepEqual(recovered, wantCommands) || restarted.commitIndex != 3 || restarted.lastApplied != 3 || restarted.currentTerm != 3 {
		t.Fatalf("el reinicio no conservó el estado esperado: log=%+v, comandos=%+v", restarted.log, recovered)
	}
}
