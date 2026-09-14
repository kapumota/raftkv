package raft

import (
	"testing"
	"time"
)

func TestVotePersistsAcrossRestartAndRejectsDifferentCandidateSameTerm(t *testing.T) {
	dataDir := t.TempDir()

	wal, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL inicial: %v", err)
	}
	node := NewNode("node-1", nil, wal, NewTransport(), nil)

	first := node.HandleRequestVote(RequestVoteArgs{
		Term:         7,
		CandidateID:  "candidate-a",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if !first.VoteGranted || first.Term != 7 {
		t.Fatalf("el primer voto no fue concedido: respuesta=%+v", first)
	}

	persisted, err := wal.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el voto persistido: %v", err)
	}
	if persisted.CurrentTerm != 7 || persisted.VotedFor != "candidate-a" {
		t.Fatalf("estado persistido inesperado: %+v", persisted)
	}

	recoveredWAL, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo reabrir el WAL: %v", err)
	}
	recovered := NewNode("node-1", nil, recoveredWAL, NewTransport(), nil)

	_, _, term, votedFor, _ := recovered.Status()
	if term != 7 || votedFor != "candidate-a" {
		t.Fatalf("el reinicio perdió el voto: término=%d, votedFor=%q", term, votedFor)
	}

	second := recovered.HandleRequestVote(RequestVoteArgs{
		Term:         7,
		CandidateID:  "candidate-b",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if second.VoteGranted || second.Term != 7 {
		t.Fatalf("se concedió un segundo voto en el mismo término: respuesta=%+v", second)
	}

	afterReject, err := recoveredWAL.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el estado después del rechazo: %v", err)
	}
	if afterReject.CurrentTerm != 7 || afterReject.VotedFor != "candidate-a" {
		t.Fatalf("el rechazo alteró el voto persistido: %+v", afterReject)
	}

	repeated := recovered.HandleRequestVote(RequestVoteArgs{
		Term:         7,
		CandidateID:  "candidate-a",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if !repeated.VoteGranted || repeated.Term != 7 {
		t.Fatalf("el mismo candidato no pudo renovar su voto: respuesta=%+v", repeated)
	}

	afterRepeat, err := recoveredWAL.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el estado después del voto repetido: %v", err)
	}
	if afterRepeat.CurrentTerm != 7 || afterRepeat.VotedFor != "candidate-a" {
		t.Fatalf("el voto repetido alteró el estado persistido: %+v", afterRepeat)
	}
}

func TestNewTermCanReplacePersistedVoteAfterRestart(t *testing.T) {
	dataDir := t.TempDir()

	wal, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL inicial: %v", err)
	}
	node := NewNode("node-1", nil, wal, NewTransport(), nil)

	first := node.HandleRequestVote(RequestVoteArgs{
		Term:         4,
		CandidateID:  "candidate-a",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if !first.VoteGranted {
		t.Fatalf("no se concedió el voto del término 4: %+v", first)
	}

	recoveredWAL, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo reabrir el WAL: %v", err)
	}
	recovered := NewNode("node-1", nil, recoveredWAL, NewTransport(), nil)

	next := recovered.HandleRequestVote(RequestVoteArgs{
		Term:         5,
		CandidateID:  "candidate-b",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if !next.VoteGranted || next.Term != 5 {
		t.Fatalf("no se concedió el voto del término nuevo: respuesta=%+v", next)
	}

	persisted, err := recoveredWAL.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el nuevo voto persistido: %v", err)
	}
	if persisted.CurrentTerm != 5 || persisted.VotedFor != "candidate-b" {
		t.Fatalf("estado persistido inesperado en el término nuevo: %+v", persisted)
	}

	secondRestartWAL, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo reabrir el WAL por segunda vez: %v", err)
	}
	secondRestart := NewNode("node-1", nil, secondRestartWAL, NewTransport(), nil)

	conflict := secondRestart.HandleRequestVote(RequestVoteArgs{
		Term:         5,
		CandidateID:  "candidate-c",
		LastLogIndex: 0,
		LastLogTerm:  0,
	})
	if conflict.VoteGranted || conflict.Term != 5 {
		t.Fatalf("se concedió un segundo voto en el término 5: respuesta=%+v", conflict)
	}

	finalState, err := secondRestartWAL.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el estado final: %v", err)
	}
	if finalState.CurrentTerm != 5 || finalState.VotedFor != "candidate-b" {
		t.Fatalf("el segundo reinicio alteró el voto persistido: %+v", finalState)
	}
}

func TestRejectedStaleLogDoesNotConsumeVote(t *testing.T) {
	dataDir := t.TempDir()

	wal, err := NewWAL(dataDir)
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}
	entry := LogEntry{
		Term:    3,
		Index:   1,
		Command: Command{Op: "SET", Key: "saldo", Value: "100"},
	}
	if err := wal.AppendEntries([]LogEntry{entry}); err != nil {
		t.Fatalf("no se pudo preparar el log: %v", err)
	}
	if err := wal.SaveState(PersistentState{CurrentTerm: 3}); err != nil {
		t.Fatalf("no se pudo preparar el estado persistente: %v", err)
	}

	node := NewNode("node-1", nil, wal, NewTransport(), nil)

	// Un candidato con log obsoleto y término mayor debe actualizar currentTerm,
	// pero no reiniciar el election timer si el voto finalmente se rechaza.
	node.mu.Lock()
	node.electionResetAt = time.Unix(0, 0)
	observedResetAt := node.electionResetAt
	node.mu.Unlock()

	stale := node.HandleRequestVote(RequestVoteArgs{
		Term:         4,
		CandidateID:  "candidate-stale",
		LastLogIndex: 99,
		LastLogTerm:  2,
	})
	if stale.VoteGranted || stale.Term != 4 {
		t.Fatalf("se concedió voto a un log obsoleto: respuesta=%+v", stale)
	}

	afterStale, err := wal.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el estado después del rechazo: %v", err)
	}
	if afterStale.CurrentTerm != 4 || afterStale.VotedFor != "" {
		t.Fatalf("un candidato obsoleto consumió el voto: %+v", afterStale)
	}

	node.mu.Lock()
	resetAfterReject := node.electionResetAt
	node.mu.Unlock()
	if !resetAfterReject.Equal(observedResetAt) {
		t.Fatalf(
			"un RequestVote rechazado reinició el election timer: antes=%v, después=%v",
			observedResetAt,
			resetAfterReject,
		)
	}

	fresh := node.HandleRequestVote(RequestVoteArgs{
		Term:         4,
		CandidateID:  "candidate-fresh",
		LastLogIndex: 1,
		LastLogTerm:  3,
	})
	if !fresh.VoteGranted || fresh.Term != 4 {
		t.Fatalf("no se concedió voto al candidato actualizado: respuesta=%+v", fresh)
	}

	node.mu.Lock()
	resetAfterGrant := node.electionResetAt
	node.mu.Unlock()
	if !resetAfterGrant.After(observedResetAt) {
		t.Fatalf(
			"un voto concedido no reinició el election timer: antes=%v, después=%v",
			observedResetAt,
			resetAfterGrant,
		)
	}

	afterFresh, err := wal.LoadState()
	if err != nil {
		t.Fatalf("no se pudo leer el voto final: %v", err)
	}
	if afterFresh.CurrentTerm != 4 || afterFresh.VotedFor != "candidate-fresh" {
		t.Fatalf("el voto válido no quedó persistido: %+v", afterFresh)
	}
}
