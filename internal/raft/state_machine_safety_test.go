package raft

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"
)

type appliedStateSnapshot struct {
	entries     []LogEntry
	commitIndex int
	lastApplied int
	logLength   int
}

func snapshotAppliedState(node *Node) appliedStateSnapshot {
	node.mu.Lock()
	defer node.mu.Unlock()

	applied := node.lastApplied
	if applied < 0 {
		applied = 0
	}
	if applied > len(node.log) {
		applied = len(node.log)
	}

	return appliedStateSnapshot{
		entries:     append([]LogEntry(nil), node.log[:applied]...),
		commitIndex: node.commitIndex,
		lastApplied: node.lastApplied,
		logLength:   len(node.log),
	}
}

func validateStateMachineSafety(
	snapshots map[string]appliedStateSnapshot,
) error {
	ids := make([]string, 0, len(snapshots))
	for id, snapshot := range snapshots {
		ids = append(ids, id)

		if snapshot.lastApplied < 0 {
			return fmt.Errorf("%s tiene lastApplied negativo: %d", id, snapshot.lastApplied)
		}
		if snapshot.commitIndex < snapshot.lastApplied {
			return fmt.Errorf(
				"%s aplicó más allá de commitIndex: lastApplied=%d, commitIndex=%d",
				id,
				snapshot.lastApplied,
				snapshot.commitIndex,
			)
		}
		if snapshot.commitIndex > snapshot.logLength {
			return fmt.Errorf(
				"%s tiene commitIndex fuera del log: commitIndex=%d, longitud=%d",
				id,
				snapshot.commitIndex,
				snapshot.logLength,
			)
		}
		if len(snapshot.entries) != snapshot.lastApplied {
			return fmt.Errorf(
				"%s tiene snapshot aplicado inconsistente: entradas=%d, lastApplied=%d",
				id,
				len(snapshot.entries),
				snapshot.lastApplied,
			)
		}

		for position, entry := range snapshot.entries {
			expectedIndex := position + 1
			if entry.Index != expectedIndex {
				return fmt.Errorf(
					"%s aplicó índice no contiguo: posición=%d, entry.Index=%d",
					id,
					expectedIndex,
					entry.Index,
				)
			}
		}
	}
	sort.Strings(ids)

	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			leftID, rightID := ids[i], ids[j]
			left, right := snapshots[leftID], snapshots[rightID]

			limit := len(left.entries)
			if len(right.entries) < limit {
				limit = len(right.entries)
			}
			for position := 0; position < limit; position++ {
				if left.entries[position] == right.entries[position] {
					continue
				}
				return fmt.Errorf(
					"State Machine Safety violado en índice=%d entre %s y %s: left=%+v, right=%+v",
					position+1,
					leftID,
					rightID,
					left.entries[position],
					right.entries[position],
				)
			}
		}
	}

	return nil
}

func snapshotClusterAppliedState(
	c *TestCluster,
) map[string]appliedStateSnapshot {
	snapshots := make(map[string]appliedStateSnapshot, len(c.ids))
	for _, id := range c.ids {
		snapshots[id] = snapshotAppliedState(c.nodes[id])
	}
	return snapshots
}

func assertStateMachineSafetyInvariant(t *testing.T, c *TestCluster) {
	t.Helper()
	if err := validateStateMachineSafety(snapshotClusterAppliedState(c)); err != nil {
		t.Fatal(err)
	}
}

func commandsFromAppliedEntries(entries []LogEntry) []Command {
	var commands []Command
	for _, entry := range entries {
		if entry.Command.Op == noOpOperation {
			continue
		}
		commands = append(commands, entry.Command)
	}
	return commands
}

func assertAppliedCallbacksMatchLog(
	t *testing.T,
	c *TestCluster,
	recorders map[string]*replicationRecorder,
) {
	t.Helper()

	for _, id := range c.ids {
		snapshot := snapshotAppliedState(c.nodes[id])
		want := commandsFromAppliedEntries(snapshot.entries)
		got := recorders[id].snapshot()

		if !reflect.DeepEqual(got, want) {
			t.Fatalf(
				"%s aplicó efectos distintos de su prefijo aplicado: got=%+v, want=%+v",
				id,
				got,
				want,
			)
		}
	}
}

func TestStateMachineSafetyCheckerDetectsDifferentCommandsAtSameIndex(t *testing.T) {
	first := LogEntry{
		Index:   1,
		Term:    1,
		Command: Command{Op: "SET", Key: "saldo", Value: "100"},
	}
	different := LogEntry{
		Index:   1,
		Term:    2,
		Command: Command{Op: "SET", Key: "saldo", Value: "200"},
	}

	snapshots := map[string]appliedStateSnapshot{
		"node-a": {
			entries:     []LogEntry{first},
			commitIndex: 1,
			lastApplied: 1,
			logLength:   1,
		},
		"node-b": {
			entries:     []LogEntry{different},
			commitIndex: 1,
			lastApplied: 1,
			logLength:   1,
		},
	}

	if err := validateStateMachineSafety(snapshots); err == nil {
		t.Fatal("el checker aceptó comandos diferentes aplicados en el mismo índice")
	}
}

func TestStateMachineSafetyAcrossLeaderChangeAndConflictRepair(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	oldID := c.WaitForLeader()
	oldLeader := c.nodes[oldID]

	initial := Command{
		Op:    "SET",
		Key:   "saldo",
		Value: "100",
	}
	initialPrefix := proposeClusterCommand(t, oldLeader, initial)
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		initialPrefix,
		[]Command{initial},
	)
	assertStateMachineSafetyInvariant(t, c)
	assertAppliedCallbacksMatchLog(t, c, recorders)

	c.Disconnect(oldID)

	for _, value := range []string{"pendiente-1", "pendiente-2"} {
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		err := oldLeader.Propose(ctx, Command{
			Op:    "SET",
			Key:   "saldo",
			Value: value,
		})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf(
				"se esperaba una propuesta pendiente sin quorum, se obtuvo: %v",
				err,
			)
		}
	}

	if got := recorders[oldID].snapshot(); !reflect.DeepEqual(got, []Command{initial}) {
		t.Fatalf(
			"el leader aislado aplicó entradas no confirmadas: %+v",
			got,
		)
	}
	assertStateMachineSafetyInvariant(t, c)
	assertAppliedCallbacksMatchLog(t, c, recorders)

	newID := c.WaitForLeader()
	if newID == oldID {
		t.Fatal("el leader aislado conservó quorum")
	}

	updated := Command{
		Op:    "SET",
		Key:   "saldo",
		Value: "200",
	}
	newPrefix := proposeClusterCommand(t, c.nodes[newID], updated)
	majority := clusterOtherNodes(c, oldID)
	assertCommittedPrefix(
		t,
		c,
		recorders,
		majority,
		newPrefix,
		[]Command{initial, updated},
	)

	// El nodo aislado puede conservar un sufijo conflictivo no confirmado, pero
	// su prefijo aplicado debe seguir siendo compatible con el de la mayoría.
	assertStateMachineSafetyInvariant(t, c)
	assertAppliedCallbacksMatchLog(t, c, recorders)

	oldSnapshot := snapshotAppliedState(oldLeader)
	if !reflect.DeepEqual(
		commandsFromAppliedEntries(oldSnapshot.entries),
		[]Command{initial},
	) {
		t.Fatalf(
			"el nodo aislado alteró su historial aplicado: %+v",
			oldSnapshot.entries,
		)
	}

	c.Reconnect(oldID)
	c.WaitForLeader()
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		newPrefix,
		[]Command{initial, updated},
	)

	assertStateMachineSafetyInvariant(t, c)
	assertAppliedCallbacksMatchLog(t, c, recorders)

	snapshots := snapshotClusterAppliedState(c)
	reference := snapshots[c.ids[0]].entries
	for _, id := range c.ids[1:] {
		if !reflect.DeepEqual(snapshots[id].entries, reference) {
			t.Fatalf(
				"%s no convergió al mismo historial aplicado: got=%+v, want=%+v",
				id,
				snapshots[id].entries,
				reference,
			)
		}
	}
}

func TestUncommittedConflictIsNeverApplied(t *testing.T) {
	var applied []Command
	node := newTestNode(t, func(cmd Command) {
		applied = append(applied, cmd)
	})

	committed := LogEntry{
		Index:   1,
		Term:    1,
		Command: Command{Op: "SET", Key: "saldo", Value: "100"},
	}
	pending := LogEntry{
		Index:   2,
		Term:    1,
		Command: Command{Op: "SET", Key: "saldo", Value: "pendiente"},
	}

	reply := node.HandleAppendEntries(AppendEntriesArgs{
		Term:         1,
		LeaderID:     "leader-1",
		Entries:      []LogEntry{committed, pending},
		LeaderCommit: 1,
	})
	if !reply.Success {
		t.Fatalf("se rechazó el log inicial: %+v", reply)
	}
	if !reflect.DeepEqual(applied, []Command{committed.Command}) {
		t.Fatalf("se aplicó una entrada pendiente: %+v", applied)
	}

	replacement := LogEntry{
		Index:   2,
		Term:    2,
		Command: Command{Op: "SET", Key: "saldo", Value: "200"},
	}
	reply = node.HandleAppendEntries(AppendEntriesArgs{
		Term:         2,
		LeaderID:     "leader-2",
		PrevLogIndex: 1,
		PrevLogTerm:  1,
		Entries:      []LogEntry{replacement},
		LeaderCommit: 2,
	})
	if !reply.Success {
		t.Fatalf("se rechazó el reemplazo del sufijo pendiente: %+v", reply)
	}

	if !reflect.DeepEqual(
		applied,
		[]Command{committed.Command, replacement.Command},
	) {
		t.Fatalf(
			"la máquina de estados aplicó un historial inesperado: %+v",
			applied,
		)
	}

	snapshot := snapshotAppliedState(node)
	want := []LogEntry{committed, replacement}
	if !reflect.DeepEqual(snapshot.entries, want) {
		t.Fatalf(
			"el historial aplicado no coincide con el log confirmado: got=%+v, want=%+v",
			snapshot.entries,
			want,
		)
	}

	if err := validateStateMachineSafety(map[string]appliedStateSnapshot{
		"node-1": snapshot,
	}); err != nil {
		t.Fatal(err)
	}
}
