package raft

import "testing"

func TestWALSaveAndLoadState(t *testing.T) {
	wal, err := NewWAL(t.TempDir())
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}

	expected := PersistentState{CurrentTerm: 7, VotedFor: "node-2", CommitIndex: 3}
	if err := wal.SaveState(expected); err != nil {
		t.Fatalf("no se pudo guardar el estado: %v", err)
	}

	got, err := wal.LoadState()
	if err != nil {
		t.Fatalf("no se pudo cargar el estado: %v", err)
	}
	if got != expected {
		t.Fatalf("estado inesperado: se obtuvo %+v, se esperaba %+v", got, expected)
	}
}

func TestWALAppendAndLoadLog(t *testing.T) {
	wal, err := NewWAL(t.TempDir())
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}

	entries := []LogEntry{
		{Term: 1, Index: 1, Command: Command{Op: "SET", Key: "a", Value: "1"}},
		{Term: 1, Index: 2, Command: Command{Op: "SET", Key: "b", Value: "2"}},
	}
	if err := wal.AppendEntries(entries); err != nil {
		t.Fatalf("no se pudieron agregar entradas: %v", err)
	}

	got, err := wal.LoadLog()
	if err != nil {
		t.Fatalf("no se pudo cargar el log: %v", err)
	}
	if len(got) != len(entries) {
		t.Fatalf("cantidad inesperada de entradas: se obtuvo %d, se esperaba %d", len(got), len(entries))
	}
	for i := range entries {
		if got[i] != entries[i] {
			t.Fatalf("entrada %d inesperada: se obtuvo %+v, se esperaba %+v", i, got[i], entries[i])
		}
	}
}

func TestWALRewriteLog(t *testing.T) {
	wal, err := NewWAL(t.TempDir())
	if err != nil {
		t.Fatalf("no se pudo crear el WAL: %v", err)
	}

	initial := []LogEntry{
		{Term: 1, Index: 1, Command: Command{Op: "SET", Key: "a", Value: "1"}},
		{Term: 1, Index: 2, Command: Command{Op: "SET", Key: "b", Value: "2"}},
	}
	if err := wal.AppendEntries(initial); err != nil {
		t.Fatalf("no se pudo preparar el log inicial: %v", err)
	}

	rewritten := []LogEntry{
		{Term: 1, Index: 1, Command: Command{Op: "SET", Key: "a", Value: "1"}},
		{Term: 2, Index: 2, Command: Command{Op: "SET", Key: "b", Value: "3"}},
	}
	if err := wal.RewriteLog(rewritten); err != nil {
		t.Fatalf("no se pudo reescribir el log: %v", err)
	}

	got, err := wal.LoadLog()
	if err != nil {
		t.Fatalf("no se pudo cargar el log reescrito: %v", err)
	}
	if len(got) != len(rewritten) {
		t.Fatalf("cantidad inesperada de entradas: se obtuvo %d, se esperaba %d", len(got), len(rewritten))
	}
	for i := range rewritten {
		if got[i] != rewritten[i] {
			t.Fatalf("entrada %d inesperada: se obtuvo %+v, se esperaba %+v", i, got[i], rewritten[i])
		}
	}
}
