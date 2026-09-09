package experiment

import (
	"errors"
	"sync/atomic"
	"testing"
)

func TestUpdateLeaderHintPreservesLastKnownLeader(t *testing.T) {
	var hint atomic.Value
	hint.Store("nodo-2")

	updateLeaderHint(&hint, nil, errors.New("timeout"))
	if got := hint.Load().(string); got != "nodo-2" {
		t.Fatalf("se perdió el último líder conocido: %s", got)
	}

	updateLeaderHint(&hint, []NodeStatus{
		{ID: "nodo-1", State: "seguidor"},
		{ID: "nodo-2", State: "seguidor"},
	}, nil)
	if got := hint.Load().(string); got != "nodo-2" {
		t.Fatalf("una vista sin líder borró el hint: %s", got)
	}

	updateLeaderHint(&hint, []NodeStatus{
		{ID: "nodo-1", State: "seguidor"},
		{ID: "nodo-3", State: "lider"},
		{ID: "nodo-4", State: "seguidor"},
	}, nil)
	if got := hint.Load().(string); got != "nodo-3" {
		t.Fatalf("no se actualizó el nuevo líder: %s", got)
	}
}
