package raft

import (
	"reflect"
	"testing"
)

func TestDelayedAppendEntriesPreservesSuffix(t *testing.T) {
	for _, committed := range []bool{false, true} {
		name := "sufijo pendiente"
		if committed {
			name = "sufijo confirmado"
		}
		t.Run(name, func(t *testing.T) {
			var applied []Command
			node := newTestNode(t, func(cmd Command) { applied = append(applied, cmd) })
			entries := []LogEntry{
				{Index: 1, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "100"}},
				{Index: 2, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "200"}},
				{Index: 3, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "300"}},
			}
			commitIndex := 1
			if committed {
				commitIndex = len(entries)
			}
			if reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 1, Entries: entries, LeaderCommit: commitIndex}); !reply.Success {
				t.Fatal("se rechazó la petición inicial")
			}
			// La petición más corta llega después de la larga, sin temporizadores.
			if reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 1, Entries: entries[:1], LeaderCommit: 1}); !reply.Success {
				t.Fatal("se rechazó el prefijo repetido")
			}
			if !reflect.DeepEqual(node.log, entries) || node.commitIndex != commitIndex || node.lastApplied != commitIndex {
				t.Fatalf("la petición retrasada alteró el estado: log=%+v, confirmado=%d, aplicado=%d", node.log, node.commitIndex, node.lastApplied)
			}
			persisted, err := node.wal.LoadLog()
			if err != nil || !reflect.DeepEqual(persisted, entries) {
				t.Fatalf("el WAL perdió el sufijo: log=%+v, error=%v", persisted, err)
			}
			if reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 1, PrevLogIndex: 3, PrevLogTerm: 1, LeaderCommit: 3}); !reply.Success {
				t.Fatal("se rechazó la confirmación del sufijo conservado")
			}
			want := []Command{entries[0].Command, entries[1].Command, entries[2].Command}
			if !reflect.DeepEqual(applied, want) {
				t.Fatalf("se perdieron o reaplicaron comandos: %+v", applied)
			}
		})
	}
}

func TestAppendEntriesReplacesOnlyConflictingSuffix(t *testing.T) {
	node := newTestNode(t, nil)
	first := LogEntry{Index: 1, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "100"}}
	old := LogEntry{Index: 2, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "200"}}
	if reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 1, Entries: []LogEntry{first, old}, LeaderCommit: 1}); !reply.Success {
		t.Fatal("se rechazó el log inicial")
	}
	replacement := LogEntry{Index: 2, Term: 2, Command: Command{Op: "SET", Key: "saldo", Value: "300"}}
	reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 2, PrevLogIndex: 1, PrevLogTerm: 1, Entries: []LogEntry{replacement}, LeaderCommit: 2})
	if !reply.Success || !reflect.DeepEqual(node.log, []LogEntry{first, replacement}) || node.commitIndex != 2 {
		t.Fatalf("no se reparó el sufijo pendiente: respuesta=%+v, log=%+v", reply, node.log)
	}
	conflict := LogEntry{Index: 1, Term: 3, Command: old.Command}
	reply = node.HandleAppendEntries(AppendEntriesArgs{Term: 3, Entries: []LogEntry{conflict}})
	if reply.Success || reply.Term != 3 || !reflect.DeepEqual(node.log, []LogEntry{first, replacement}) {
		t.Fatalf("se alteró el prefijo confirmado o se devolvió un término incorrecto: respuesta=%+v, log=%+v", reply, node.log)
	}
}

func TestAppendEntriesCommitsOnlyMatchedPrefix(t *testing.T) {
	node := newTestNode(t, nil)
	entries := []LogEntry{
		{Index: 1, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "100"}},
		{Index: 2, Term: 1, Command: Command{Op: "SET", Key: "saldo", Value: "200"}},
	}
	if reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 1, Entries: entries}); !reply.Success {
		t.Fatal("se rechazó el log inicial")
	}
	reply := node.HandleAppendEntries(AppendEntriesArgs{Term: 2, PrevLogIndex: 1, PrevLogTerm: 1, LeaderCommit: 2})
	if !reply.Success || node.commitIndex != 1 || node.lastApplied != 1 {
		t.Fatalf("se confirmó un sufijo no verificado: respuesta=%+v, confirmado=%d, aplicado=%d", reply, node.commitIndex, node.lastApplied)
	}
}
