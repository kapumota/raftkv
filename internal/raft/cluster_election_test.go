package raft

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

func TestInitialLeaderElection(t *testing.T) {
	for _, size := range []int{1, 3, 5} {
		t.Run(fmt.Sprintf("nodos-%d", size), func(t *testing.T) {
			c := NewTestCluster(t, size)
			c.Start()
			id := c.WaitForLeader()
			node := c.nodes[id]
			node.mu.Lock()
			state, term, vote := node.state, node.currentTerm, node.votedFor
			barrier, committed := node.leaderBarrierIndex, node.commitIndex
			node.mu.Unlock()
			if state != Leader || term < 1 || vote != id {
				t.Fatalf("elección inicial inválida: nodo=%s, estado=%s, término=%d, voto=%s", id, state, term, vote)
			}
			if barrier < 1 || committed < barrier {
				t.Fatalf("barrera sin confirmar: índice=%d, confirmado=%d", barrier, committed)
			}
		})
	}
}

func TestLeaderReplacementAfterFailure(t *testing.T) {
	c := NewTestCluster(t, 3)
	c.Start()
	original := c.WaitForLeader()
	_, _, originalTerm, _, _ := c.nodes[original].Status()

	// La falla es de comunicación: el líder aislado conserva su proceso y WAL.
	c.Disconnect(original)
	replacement := c.WaitForLeader()
	_, _, replacementTerm, _, _ := c.nodes[replacement].Status()
	if replacement == original || replacementTerm <= originalTerm {
		t.Fatalf("reemplazo inválido: líder=%s, término=%d; anterior=%s, término=%d", replacement, replacementTerm, original, originalTerm)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	if err := c.nodes[original].ConfirmLeadership(ctx); err == nil {
		t.Fatal("el líder aislado confirmó liderazgo sin quorum")
	}
}

// TestSingleLeaderPerTerm conserva las observaciones por término, incluso
// después de cambiar de líder. Un líder antiguo aislado puede seguir creyendo
// que es líder: solo es un conflicto si otro nodo lidera en el mismo término.
// El muestreo verifica esta ejecución; no demuestra todos los intercalados.
func TestSingleLeaderPerTerm(t *testing.T) {
	c := NewTestCluster(t, 5)
	var mu sync.Mutex
	leaders := make(map[int]string)
	var conflicts []string
	record := func() {
		mu.Lock()
		defer mu.Unlock()
		for _, id := range c.ids {
			node := c.nodes[id]
			node.mu.Lock()
			state, term := node.state, node.currentTerm
			node.mu.Unlock()
			if state != Leader {
				continue
			}
			if previous, ok := leaders[term]; ok && previous != id {
				conflicts = append(conflicts, fmt.Sprintf("término %d: líderes %s y %s", term, previous, id))
			} else {
				leaders[term] = id
			}
		}
	}
	stop, done := make(chan struct{}), make(chan struct{})
	var once sync.Once
	stopObserver := func() {
		once.Do(func() { close(stop) })
		<-done
	}
	go func() {
		defer close(done)
		ticker := time.NewTicker(5 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				record()
			}
		}
	}()
	t.Cleanup(stopObserver)
	c.Start()
	for round := 0; round < 3; round++ {
		id := c.WaitForLeader()
		record()
		if round < 2 {
			// Cinco nodos permiten aislar dos líderes sucesivos y conservar mayoría.
			c.Disconnect(id)
		}
	}
	stopObserver()
	if len(conflicts) != 0 {
		t.Fatalf("se observaron líderes distintos en un mismo término: %v", conflicts)
	}
	if len(leaders) < 3 {
		t.Fatalf("se esperaban al menos tres términos con líder, se observaron %v", leaders)
	}
}
