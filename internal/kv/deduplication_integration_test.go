package kv

import (
	"testing"

	"github.com/kapumota/raftkv/internal/raft"
)

func TestLostResponseRetryDoesNotApplyTwice(t *testing.T) {
	for _, restart := range []bool{false, true} {
		name := "sin reinicio"
		if restart {
			name = "con reinicio"
		}
		t.Run(name, func(t *testing.T) {
			dataDir := t.TempDir()
			store := NewStore()
			openNode := func() (*raft.Node, *raft.WAL) {
				t.Helper()
				wal, err := raft.NewWAL(dataDir)
				if err != nil {
					t.Fatalf("no se pudo abrir el WAL: %v", err)
				}
				return raft.NewNode("nodo-1", nil, wal, raft.NewTransport(), store.Apply), wal
			}
			node, wal := openNode()
			appendCommitted := func(index int, cmd raft.Command) {
				t.Helper()
				previousTerm := 1
				if index == 1 {
					previousTerm = 0
				}
				reply := node.HandleAppendEntries(raft.AppendEntriesArgs{
					Term:         1,
					LeaderID:     "líder",
					PrevLogIndex: index - 1,
					PrevLogTerm:  previousTerm,
					Entries:      []raft.LogEntry{{Term: 1, Index: index, Command: cmd}},
					LeaderCommit: index,
				})
				if !reply.Success {
					t.Fatalf("el nodo rechazó la entrada confirmada %d", index)
				}
				_, _, _, _, committed := node.Status()
				if committed != index {
					t.Fatalf("índice confirmado inesperado: se obtuvo %d, se esperaba %d", committed, index)
				}
			}
			assertValue := func(want string) {
				t.Helper()
				if value, ok := store.Get("saldo"); !ok || value != want {
					t.Fatalf("saldo inesperado: se obtuvo %q (presente: %t), se esperaba %q", value, ok, want)
				}
			}

			original := raft.Command{Op: "SET", Key: "saldo", Value: "100", ClientID: "cliente-a", RequestID: 1}
			appendCommitted(1, original)
			assertValue("100")

			// Se simula que la respuesta de esta escritura no llega al cliente.
			// Otro cliente escribe antes del reintento, para hacer observable una
			// reaplicación incluso aunque SET sea idempotente por sí mismo.
			appendCommitted(2, raft.Command{Op: "SET", Key: "saldo", Value: "200", ClientID: "cliente-b", RequestID: 1})
			assertValue("200")
			if restart {
				// No se inician bucles de elecciones: el nodo anterior queda inactivo.
				// Se descartan tanto los datos como las sesiones en memoria.
				store = NewStore()
				node, wal = openNode()
				assertValue("200")
			}

			// El reintento conserva exactamente el comando original y ocupa una
			// entrada nueva del log; solo su efecto debe ser descartado.
			appendCommitted(3, original)
			assertValue("200")
			entries, err := wal.LoadLog()
			if err != nil {
				t.Fatalf("no se pudo leer el log: %v", err)
			}
			if len(entries) != 3 || entries[0].Command != original || entries[2].Command != original {
				t.Fatal("el log debe conservar la solicitud original y su reintento")
			}

			appendCommitted(4, raft.Command{Op: "SET", Key: "saldo", Value: "300", ClientID: "cliente-a", RequestID: 2})
			assertValue("300")
		})
	}
}
