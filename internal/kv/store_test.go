package kv

import (
	"testing"

	"github.com/kapumota/raftkv/internal/raft"
)

func TestStoreApplySet(t *testing.T) {
	store := NewStore()
	store.Apply(raft.Command{Op: "SET", Key: "saldo", Value: "100"})

	value, ok := store.Get("saldo")
	if !ok {
		t.Fatal("se esperaba encontrar la clave aplicada")
	}
	if value != "100" {
		t.Fatalf("valor inesperado: se obtuvo %q, se esperaba %q", value, "100")
	}
}

func TestStoreIgnoresUnsupportedCommand(t *testing.T) {
	store := NewStore()
	store.Apply(raft.Command{Op: "DELETE", Key: "saldo"})

	if _, ok := store.Get("saldo"); ok {
		t.Fatal("un comando no soportado no debe modificar el almacén")
	}
}

func TestStoreGetMissingKey(t *testing.T) {
	store := NewStore()

	if _, ok := store.Get("inexistente"); ok {
		t.Fatal("una clave inexistente no debe reportarse como encontrada")
	}
}

func TestStoreDeduplicatesRequests(t *testing.T) {
	tests := []struct {
		name     string
		commands []raft.Command
		want     string
	}{
		{
			name: "reintento idéntico después de otro cliente",
			commands: []raft.Command{
				{Op: "SET", Key: "saldo", Value: "100", ClientID: "cliente-a", RequestID: 1},
				{Op: "SET", Key: "saldo", Value: "200", ClientID: "cliente-b", RequestID: 1},
				{Op: "SET", Key: "saldo", Value: "100", ClientID: "cliente-a", RequestID: 1},
			},
			want: "200",
		},
		{
			name: "solicitud antigua",
			commands: []raft.Command{
				{Op: "SET", Key: "saldo", Value: "100", ClientID: "cliente-a", RequestID: 1},
				{Op: "SET", Key: "saldo", Value: "200", ClientID: "cliente-a", RequestID: 2},
				{Op: "SET", Key: "saldo", Value: "100", ClientID: "cliente-a", RequestID: 1},
			},
			want: "200",
		},
		{
			name: "primera solicitud con identificador cero",
			commands: []raft.Command{
				{Op: "SET", Key: "saldo", Value: "100", ClientID: "cliente-a", RequestID: 0},
				{Op: "SET", Key: "saldo", Value: "200", ClientID: "cliente-a", RequestID: 0},
			},
			want: "100",
		},
		{
			name: "compatibilidad sin identidad",
			commands: []raft.Command{
				{Op: "SET", Key: "saldo", Value: "100"},
				{Op: "SET", Key: "saldo", Value: "200"},
			},
			want: "200",
		},
		{
			name: "operación no soportada no consume la solicitud",
			commands: []raft.Command{
				{Op: "NOOP", ClientID: "cliente-a", RequestID: 1},
				{Op: "SET", Key: "saldo", Value: "100", ClientID: "cliente-a", RequestID: 1},
			},
			want: "100",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := NewStore()
			for _, cmd := range tt.commands {
				store.Apply(cmd)
			}
			if value, ok := store.Get("saldo"); !ok || value != tt.want {
				t.Fatalf("valor inesperado: se obtuvo %q (presente: %t), se esperaba %q", value, ok, tt.want)
			}
		})
	}
}
