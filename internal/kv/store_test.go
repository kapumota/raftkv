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
