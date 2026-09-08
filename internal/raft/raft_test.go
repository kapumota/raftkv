package raft

import (
	"context"
	"errors"
	"testing"
	"time"
)

func newTestNode(t *testing.T, apply ApplyFunc) *Node {
	t.Helper()
	wal, err := NewWAL(t.TempDir())
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}
	return NewNode("node-1", nil, wal, NewTransport(), apply)
}

func TestFollowerRejectsProposal(t *testing.T) {
	node := newTestNode(t, nil)

	err := node.Propose(context.Background(), Command{Op: "SET", Key: "a", Value: "1"})
	if !errors.Is(err, ErrNotLeader) {
		t.Fatalf("error inesperado: se obtuvo %v, se esperaba ErrNotLeader", err)
	}
}

func TestNewNodeRestoresCommitIndex(t *testing.T) {
	dir := t.TempDir()
	wal, err := NewWAL(dir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}
	entries := []LogEntry{
		{Term: 1, Index: 1, Command: Command{Op: "SET", Key: "a", Value: "1"}},
		{Term: 2, Index: 2, Command: Command{Op: "SET", Key: "b", Value: "2"}},
	}
	if err := wal.AppendEntries(entries); err != nil {
		t.Fatalf("no se pudo preparar el log: %v", err)
	}
	if err := wal.SaveState(PersistentState{CurrentTerm: 2, VotedFor: "node-2", CommitIndex: 1}); err != nil {
		t.Fatalf("no se pudo guardar el estado: %v", err)
	}

	restarted := NewNode("node-1", nil, wal, NewTransport(), nil)

	restarted.mu.Lock()
	defer restarted.mu.Unlock()
	if restarted.commitIndex != 1 {
		t.Fatalf("índice de commit inesperado después del reinicio: se obtuvo %d, se esperaba 1", restarted.commitIndex)
	}
}

func TestNewNodeLimitsRecoveredCommitIndexToLog(t *testing.T) {
	dir := t.TempDir()
	wal, err := NewWAL(dir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}
	entry := LogEntry{Term: 1, Index: 1, Command: Command{Op: "SET", Key: "a", Value: "1"}}
	if err := wal.AppendEntries([]LogEntry{entry}); err != nil {
		t.Fatalf("no se pudo preparar el log: %v", err)
	}
	if err := wal.SaveState(PersistentState{CurrentTerm: 1, CommitIndex: 4}); err != nil {
		t.Fatalf("no se pudo guardar el estado: %v", err)
	}

	restarted := NewNode("node-1", nil, wal, NewTransport(), nil)

	restarted.mu.Lock()
	defer restarted.mu.Unlock()
	if restarted.commitIndex != 1 {
		t.Fatalf("índice de commit recuperado fuera del log: se obtuvo %d, se esperaba 1", restarted.commitIndex)
	}
}

func TestNewNodeRebuildsStateMachineFromCommittedLog(t *testing.T) {
	dir := t.TempDir()
	wal, err := NewWAL(dir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}
	entries := []LogEntry{
		{Term: 1, Index: 1, Command: Command{Op: "SET", Key: "a", Value: "1"}},
		{Term: 2, Index: 2, Command: Command{Op: "SET", Key: "b", Value: "2"}},
		{Term: 2, Index: 3, Command: Command{Op: "SET", Key: "c", Value: "3"}},
	}
	if err := wal.AppendEntries(entries); err != nil {
		t.Fatalf("no se pudo preparar el log: %v", err)
	}
	if err := wal.SaveState(PersistentState{CurrentTerm: 2, CommitIndex: 2}); err != nil {
		t.Fatalf("no se pudo guardar el estado: %v", err)
	}

	var applied []Command
	restarted := NewNode("node-1", nil, wal, NewTransport(), func(cmd Command) {
		applied = append(applied, cmd)
	})

	if len(applied) != 2 {
		t.Fatalf("cantidad inesperada de comandos recuperados: se obtuvo %d, se esperaba 2", len(applied))
	}
	for i := range applied {
		if applied[i] != entries[i].Command {
			t.Fatalf("comando recuperado %d inesperado: se obtuvo %+v, se esperaba %+v", i, applied[i], entries[i].Command)
		}
	}

	restarted.mu.Lock()
	defer restarted.mu.Unlock()
	if restarted.lastApplied != 2 {
		t.Fatalf("último índice aplicado inesperado: se obtuvo %d, se esperaba 2", restarted.lastApplied)
	}
}

func TestRecoveredEntriesAreNotReapplied(t *testing.T) {
	dir := t.TempDir()
	wal, err := NewWAL(dir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}
	entries := []LogEntry{
		{Term: 1, Index: 1, Command: Command{Op: "SET", Key: "a", Value: "1"}},
		{Term: 2, Index: 2, Command: Command{Op: "SET", Key: "b", Value: "2"}},
		{Term: 2, Index: 3, Command: Command{Op: "SET", Key: "c", Value: "3"}},
	}
	if err := wal.AppendEntries(entries); err != nil {
		t.Fatalf("no se pudo preparar el log: %v", err)
	}
	if err := wal.SaveState(PersistentState{CurrentTerm: 2, CommitIndex: 2}); err != nil {
		t.Fatalf("no se pudo guardar el estado: %v", err)
	}

	var applied []Command
	restarted := NewNode("node-1", nil, wal, NewTransport(), func(cmd Command) {
		applied = append(applied, cmd)
	})
	if len(applied) != 2 {
		t.Fatalf("cantidad inesperada de comandos recuperados: se obtuvo %d, se esperaba 2", len(applied))
	}

	heartbeat := restarted.HandleAppendEntries(AppendEntriesArgs{
		Term:         2,
		LeaderID:     "node-2",
		PrevLogIndex: 3,
		PrevLogTerm:  2,
		LeaderCommit: 2,
	})
	if !heartbeat.Success {
		t.Fatal("se esperaba que el heartbeat fuera aceptado")
	}
	if len(applied) != 2 {
		t.Fatalf("se reaplicaron comandos después del heartbeat: se obtuvo %d aplicaciones, se esperaba 2", len(applied))
	}

	advance := restarted.HandleAppendEntries(AppendEntriesArgs{
		Term:         2,
		LeaderID:     "node-2",
		PrevLogIndex: 3,
		PrevLogTerm:  2,
		LeaderCommit: 3,
	})
	if !advance.Success {
		t.Fatal("se esperaba que el avance de commit fuera aceptado")
	}
	if len(applied) != 3 {
		t.Fatalf("cantidad inesperada de aplicaciones después del avance: se obtuvo %d, se esperaba 3", len(applied))
	}
	if applied[2] != entries[2].Command {
		t.Fatalf("comando nuevo inesperado: se obtuvo %+v, se esperaba %+v", applied[2], entries[2].Command)
	}
}

func TestProposalRespectsCanceledContext(t *testing.T) {
	node := newTestNode(t, nil)
	node.mu.Lock()
	node.state = Leader
	node.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := node.Propose(ctx, Command{Op: "SET", Key: "a", Value: "1"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error inesperado: se obtuvo %v, se esperaba context.Canceled", err)
	}

	node.mu.Lock()
	defer node.mu.Unlock()
	if len(node.log) != 0 {
		t.Fatalf("el log cambió con un contexto cancelado: se obtuvo %d entradas", len(node.log))
	}
}

func TestHandleRequestVoteGrantsVoteToUpToDateCandidate(t *testing.T) {
	node := newTestNode(t, nil)

	reply := node.HandleRequestVote(RequestVoteArgs{
		Term:         1,
		CandidateID:  "node-2",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})

	if !reply.VoteGranted {
		t.Fatal("se esperaba que el follower concediera el voto")
	}
	if reply.Term != 1 {
		t.Fatalf("término inesperado: se obtuvo %d, se esperaba 1", reply.Term)
	}
}

func TestHandleAppendEntriesAppliesCommittedEntry(t *testing.T) {
	var applied []Command
	node := newTestNode(t, func(cmd Command) {
		applied = append(applied, cmd)
	})

	entry := LogEntry{
		Term:    1,
		Index:   1,
		Command: Command{Op: "SET", Key: "saldo", Value: "100"},
	}
	reply := node.HandleAppendEntries(AppendEntriesArgs{
		Term:         1,
		LeaderID:     "node-2",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
		Entries:      []LogEntry{entry},
		LeaderCommit: 1,
	})

	if !reply.Success {
		t.Fatal("se esperaba que AppendEntries fuera aceptado")
	}
	if len(applied) != 1 {
		t.Fatalf("cantidad inesperada de comandos aplicados: se obtuvo %d, se esperaba 1", len(applied))
	}
	if applied[0] != entry.Command {
		t.Fatalf("comando aplicado inesperado: se obtuvo %+v, se esperaba %+v", applied[0], entry.Command)
	}
}

func TestSingleNodeProposalCommitsImmediately(t *testing.T) {
	var applied []Command
	node := newTestNode(t, func(cmd Command) {
		applied = append(applied, cmd)
	})

	node.mu.Lock()
	node.state = Leader
	node.currentTerm = 1
	node.mu.Unlock()

	cmd := Command{Op: "SET", Key: "saldo", Value: "100"}
	if err := node.Propose(context.Background(), cmd); err != nil {
		t.Fatalf("la propuesta falló: %v", err)
	}

	node.mu.Lock()
	defer node.mu.Unlock()
	if node.commitIndex != 1 {
		t.Fatalf("índice de commit inesperado: se obtuvo %d, se esperaba 1", node.commitIndex)
	}
	if len(applied) != 1 || applied[0] != cmd {
		t.Fatalf("comando aplicado inesperado: se obtuvo %+v", applied)
	}

	state, err := node.wal.LoadState()
	if err != nil {
		t.Fatalf("no se pudo cargar el estado persistente: %v", err)
	}
	if state.CommitIndex != 1 {
		t.Fatalf("índice de commit persistente inesperado: se obtuvo %d, se esperaba 1", state.CommitIndex)
	}
}

func TestProposalWaitsForMajorityCommit(t *testing.T) {
	node := newTestNode(t, nil)
	peer1 := "http://127.0.0.1:1"
	peer2 := "http://127.0.0.1:2"

	node.mu.Lock()
	node.peers = []string{peer1, peer2}
	node.state = Leader
	node.currentTerm = 1
	node.nextIndex[peer1] = 1
	node.nextIndex[peer2] = 1
	node.matchIndex[peer1] = 0
	node.matchIndex[peer2] = 0
	node.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- node.Propose(ctx, Command{Op: "SET", Key: "saldo", Value: "100"})
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		node.mu.Lock()
		appended := len(node.log) == 1
		node.mu.Unlock()
		if appended {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("la propuesta no fue agregada al log a tiempo")
		}
		time.Sleep(time.Millisecond)
	}

	select {
	case err := <-done:
		t.Fatalf("la propuesta terminó antes de alcanzar mayoría: %v", err)
	default:
	}

	node.mu.Lock()
	node.matchIndex[peer1] = 1
	node.advanceCommitIndex()
	node.mu.Unlock()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("la propuesta devolvió un error después del commit: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("la propuesta no terminó después de alcanzar mayoría")
	}
}

func TestPendingProposalFailsWhenLeaderStepsDown(t *testing.T) {
	node := newTestNode(t, nil)
	peer1 := "http://127.0.0.1:1"
	peer2 := "http://127.0.0.1:2"

	node.mu.Lock()
	node.peers = []string{peer1, peer2}
	node.state = Leader
	node.currentTerm = 1
	node.nextIndex[peer1] = 1
	node.nextIndex[peer2] = 1
	node.matchIndex[peer1] = 0
	node.matchIndex[peer2] = 0
	node.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- node.Propose(ctx, Command{Op: "SET", Key: "saldo", Value: "100"})
	}()

	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		node.mu.Lock()
		pending := len(node.pendingCommits) == 1
		node.mu.Unlock()
		if pending {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("la propuesta no quedó pendiente a tiempo")
		}
		time.Sleep(time.Millisecond)
	}

	reply := node.HandleAppendEntries(AppendEntriesArgs{
		Term:         2,
		LeaderID:     "node-2",
		PrevLogIndex: 0,
		PrevLogTerm:  0,
	})
	if !reply.Success {
		t.Fatal("se esperaba que el nuevo líder fuera aceptado")
	}

	select {
	case err := <-done:
		if !errors.Is(err, ErrNotLeader) {
			t.Fatalf("error inesperado: se obtuvo %v, se esperaba ErrNotLeader", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("la propuesta no terminó después de perder el liderazgo")
	}

	node.mu.Lock()
	defer node.mu.Unlock()
	if len(node.pendingCommits) != 0 {
		t.Fatalf("quedaron propuestas pendientes: se obtuvo %d, se esperaba 0", len(node.pendingCommits))
	}
}
