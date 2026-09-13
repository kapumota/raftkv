package raft

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestProgressResumesAfterTransientLossOfQuorum(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	initialLeaderID := c.WaitForLeader()

	initial := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, c.nodes[initialLeaderID], initial)
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		prefix,
		[]Command{initial},
	)

	// Aísla cada nodo. Durante este intervalo no existe una mayoría comunicada,
	// por lo que la fase no exige elección estable ni progreso de operaciones.
	c.Partition(
		[]string{c.ids[0]},
		[]string{c.ids[1]},
		[]string{c.ids[2]},
	)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	err := c.nodes[initialLeaderID].ConfirmLeadership(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrNotLeader) {
		t.Fatalf(
			"un nodo confirmó liderazgo sin quorum: leader=%s, error=%v",
			initialLeaderID,
			err,
		)
	}

	// Al sanar la red vuelve a existir una mayoría estable. El clúster debe
	// recuperar un leader con quorum y volver a confirmar una escritura.
	c.Heal()
	recoveredLeaderID := c.WaitForLeader()

	recovered := Command{Op: "SET", Key: "saldo", Value: "200"}
	prefix = proposeClusterCommand(t, c.nodes[recoveredLeaderID], recovered)
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		prefix,
		[]Command{initial, recovered},
	)

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	readIndex, err := c.nodes[recoveredLeaderID].ReadIndex(readCtx)
	readCancel()
	if err != nil {
		t.Fatalf(
			"el leader recuperado no pudo completar una lectura con quorum: %v",
			err,
		)
	}
	if readIndex < len(prefix) {
		t.Fatalf(
			"ReadIndex quedó detrás del prefijo confirmado: índice=%d, prefijo=%d",
			readIndex,
			len(prefix),
		)
	}
}

func TestStableMajorityMakesProgressWhileMinorityCannotConfirm(t *testing.T) {
	c, recorders := newReplicationCluster(t, 5)
	oldLeaderID := c.WaitForLeader()

	initial := Command{Op: "SET", Key: "saldo", Value: "0"}
	prefix := proposeClusterCommand(t, c.nodes[oldLeaderID], initial)
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		prefix,
		[]Command{initial},
	)

	others := clusterOtherNodes(c, oldLeaderID)
	if len(others) != 4 {
		t.Fatalf("cantidad inesperada de nodos restantes: %d", len(others))
	}

	minority := []string{oldLeaderID, others[0]}
	majority := append([]string(nil), others[1:]...)
	if len(minority) != 2 || len(majority) != 3 {
		t.Fatalf(
			"partición inválida: minoría=%v, mayoría=%v",
			minority,
			majority,
		)
	}

	c.Partition(minority, majority)

	newLeaderID := c.WaitForLeader()
	if !containsNodeID(majority, newLeaderID) {
		t.Fatalf(
			"el leader con quorum no pertenece a la mayoría: leader=%s, mayoría=%v",
			newLeaderID,
			majority,
		)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	err := c.nodes[oldLeaderID].ConfirmLeadership(ctx)
	cancel()
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrNotLeader) {
		t.Fatalf(
			"la minoría confirmó liderazgo sin quorum: leader=%s, error=%v",
			oldLeaderID,
			err,
		)
	}

	commands := []Command{initial}
	for i := 1; i <= 3; i++ {
		cmd := Command{
			Op:    "SET",
			Key:   "saldo",
			Value: fmt.Sprint(i),
		}
		commands = append(commands, cmd)
		prefix = proposeClusterCommand(t, c.nodes[newLeaderID], cmd)
		assertCommittedPrefix(
			t,
			c,
			recorders,
			majority,
			prefix,
			commands,
		)
	}

	for _, id := range minority {
		got := recorders[id].snapshot()
		if !reflect.DeepEqual(got, []Command{initial}) {
			t.Fatalf(
				"%s aplicó comandos nuevos sin pertenecer a la mayoría: %+v",
				id,
				got,
			)
		}
	}

	readCtx, readCancel := context.WithTimeout(context.Background(), 5*time.Second)
	readIndex, err := c.nodes[newLeaderID].ReadIndex(readCtx)
	readCancel()
	if err != nil {
		t.Fatalf("la mayoría estable no pudo servir ReadIndex: %v", err)
	}
	if readIndex < len(prefix) {
		t.Fatalf(
			"ReadIndex de la mayoría quedó detrás del commit: índice=%d, prefijo=%d",
			readIndex,
			len(prefix),
		)
	}

	// Al restaurar los enlaces, la evidencia de liveness incluye catch-up:
	// los nodos de la antigua minoría deben alcanzar el prefijo confirmado.
	c.Heal()
	c.WaitForLeader()
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		prefix,
		commands,
	)
}

func containsNodeID(ids []string, target string) bool {
	for _, id := range ids {
		if id == target {
			return true
		}
	}
	return false
}
