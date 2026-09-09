package raft

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"
)

func clusterOtherNodes(c *TestCluster, excluded string) []string {
	var ids []string
	for _, id := range c.ids {
		if id != excluded {
			ids = append(ids, id)
		}
	}
	return ids
}

func assertPartitionRejectsRead(t *testing.T, node *Node) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	index, err := node.ReadIndex(ctx)
	if index != 0 || (!errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrNotLeader)) {
		t.Fatalf("lectura inesperada sin quorum: índice=%d, error=%v", index, err)
	}
}

func assertPartitionRejectsWrite(t *testing.T, node *Node) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	err := node.Propose(ctx, Command{Op: "SET", Key: "saldo", Value: "sin confirmar"})
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, ErrNotLeader) {
		t.Fatalf("respuesta inesperada para escritura sin quorum: %v", err)
	}
}

func TestClusterPartitionTopology(t *testing.T) {
	c := NewTestCluster(t, 3)
	first := c.ids[:2]
	second := c.ids[2:]
	c.Partition(first, second)
	check := func(partitioned bool) {
		t.Helper()
		for i, source := range c.ids {
			for j, target := range c.ids {
				if source == target {
					continue
				}
				_, err := c.nodes[source].transport.SendRequestVote("http://"+target, RequestVoteArgs{})
				allowed := !partitioned || (i < 2 && j < 2)
				if (err == nil) != allowed {
					t.Fatalf("enlace %s a %s: permitido=%t, error=%v", source, target, allowed, err)
				}
			}
		}
	}
	check(true)
	c.Heal()
	check(false)
}

func TestFollowerPartition(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	leaderID := c.WaitForLeader()
	leader := c.nodes[leaderID]
	initial := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, leader, initial)
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial})
	isolated := clusterOtherNodes(c, leaderID)[0]
	c.Disconnect(isolated)
	updated := Command{Op: "SET", Key: "saldo", Value: "200"}
	prefix = proposeClusterCommand(t, leader, updated)
	assertCommittedPrefix(t, c, recorders, clusterOtherNodes(c, isolated), prefix, []Command{initial, updated})
	if got := recorders[isolated].snapshot(); !reflect.DeepEqual(got, []Command{initial}) {
		t.Fatalf("el seguidor aislado modificó su estado: %+v", got)
	}
	c.Reconnect(isolated)
	c.WaitForLeader()
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial, updated})
}

func TestLeaderPartition(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	oldID := c.WaitForLeader()
	old := c.nodes[oldID]
	initial := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, old, initial)
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial})
	c.Disconnect(oldID)
	newID := c.WaitForLeader()
	if newID == oldID {
		t.Fatal("el líder aislado fue reconocido con quorum")
	}
	updated := Command{Op: "SET", Key: "saldo", Value: "200"}
	prefix = proposeClusterCommand(t, c.nodes[newID], updated)
	assertCommittedPrefix(t, c, recorders, clusterOtherNodes(c, oldID), prefix, []Command{initial, updated})
	assertPartitionRejectsRead(t, old)
	assertPartitionRejectsWrite(t, old)
	if got := recorders[oldID].snapshot(); !reflect.DeepEqual(got, []Command{initial}) {
		t.Fatalf("el líder aislado aplicó una escritura nueva: %+v", got)
	}
}

func TestMinorityPartition(t *testing.T) {
	c, recorders := newReplicationCluster(t, 5)
	oldID := c.WaitForLeader()
	others := clusterOtherNodes(c, oldID)
	minority := []string{oldID, others[0]}
	majority := others[1:]
	initial := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, c.nodes[oldID], initial)
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial})
	// El líder anterior conserva un seguidor accesible, pero solo suma dos votos.
	c.Partition(minority, majority)
	newID := c.WaitForLeader()
	if newID == minority[0] || newID == minority[1] {
		t.Fatal("la minoría fue reconocida con quorum")
	}
	updated := Command{Op: "SET", Key: "saldo", Value: "200"}
	prefix = proposeClusterCommand(t, c.nodes[newID], updated)
	assertCommittedPrefix(t, c, recorders, majority, prefix, []Command{initial, updated})
	assertPartitionRejectsRead(t, c.nodes[oldID])
	assertPartitionRejectsWrite(t, c.nodes[oldID])
	for _, id := range minority {
		if got := recorders[id].snapshot(); !reflect.DeepEqual(got, []Command{initial}) {
			t.Fatalf("%s aplicó una escritura en la minoría: %+v", id, got)
		}
	}
	c.Heal()
	c.WaitForLeader()
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial, updated})
}

func TestOldLeaderRejoins(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	oldID := c.WaitForLeader()
	initial := Command{Op: "SET", Key: "saldo", Value: "100"}
	prefix := proposeClusterCommand(t, c.nodes[oldID], initial)
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial})
	c.Disconnect(oldID)
	newID := c.WaitForLeader()
	if newID == oldID {
		t.Fatal("no se reemplazó al líder aislado")
	}
	updated := Command{Op: "SET", Key: "saldo", Value: "200"}
	prefix = proposeClusterCommand(t, c.nodes[newID], updated)
	assertCommittedPrefix(t, c, recorders, clusterOtherNodes(c, oldID), prefix, []Command{initial, updated})
	c.Reconnect(oldID)
	c.WaitForLeader()
	// La reincorporación debe preservar ambas escrituras en todos los WAL y
	// aplicar cada comando una vez, aunque cambie nuevamente el liderazgo.
	assertCommittedPrefix(t, c, recorders, c.ids, prefix, []Command{initial, updated})
}
