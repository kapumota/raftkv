package raft

import (
	"errors"
	"testing"
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

	err := node.Propose(Command{Op: "SET", Key: "a", Value: "1"})
	if !errors.Is(err, ErrNotLeader) {
		t.Fatalf("error inesperado: se obtuvo %v, se esperaba ErrNotLeader", err)
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
