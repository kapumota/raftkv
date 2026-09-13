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

func validateLogMatching(logs map[string][]LogEntry) error {
	ids := make([]string, 0, len(logs))
	for id, log := range logs {
		ids = append(ids, id)
		for position, entry := range log {
			expected := position + 1
			if entry.Index != expected {
				return fmt.Errorf(
					"%s tiene índice no contiguo: posición=%d, entry.Index=%d",
					id,
					expected,
					entry.Index,
				)
			}
		}
	}
	sort.Strings(ids)

	for i := 0; i < len(ids); i++ {
		for j := i + 1; j < len(ids); j++ {
			leftID, rightID := ids[i], ids[j]
			left, right := logs[leftID], logs[rightID]
			limit := len(left)
			if len(right) < limit {
				limit = len(right)
			}

			for position := 0; position < limit; position++ {
				if left[position].Term != right[position].Term {
					continue
				}
				if reflect.DeepEqual(left[:position+1], right[:position+1]) {
					continue
				}
				return fmt.Errorf(
					"Log Matching violado entre %s y %s en índice=%d, término=%d",
					leftID,
					rightID,
					position+1,
					left[position].Term,
				)
			}
		}
	}

	return nil
}

func snapshotClusterLogs(c *TestCluster) map[string][]LogEntry {
	logs := make(map[string][]LogEntry, len(c.ids))
	for _, id := range c.ids {
		node := c.nodes[id]
		node.mu.Lock()
		logs[id] = append([]LogEntry(nil), node.log...)
		node.mu.Unlock()
	}
	return logs
}

func assertLogMatchingInvariant(t *testing.T, c *TestCluster) {
	t.Helper()
	if err := validateLogMatching(snapshotClusterLogs(c)); err != nil {
		t.Fatal(err)
	}
}

func TestLogMatchingCheckerDetectsDivergentPrefixWithSharedIndexAndTerm(t *testing.T) {
	logs := map[string][]LogEntry{
		"node-a": {
			{
				Index:   1,
				Term:    1,
				Command: Command{Op: "SET", Key: "saldo", Value: "100"},
			},
			{
				Index:   2,
				Term:    3,
				Command: Command{Op: "SET", Key: "marcador", Value: "1"},
			},
		},
		"node-b": {
			{
				Index:   1,
				Term:    2,
				Command: Command{Op: "SET", Key: "saldo", Value: "200"},
			},
			{
				Index:   2,
				Term:    3,
				Command: Command{Op: "SET", Key: "marcador", Value: "1"},
			},
		},
	}

	if err := validateLogMatching(logs); err == nil {
		t.Fatal("el checker aceptó logs con el mismo índice y término, pero prefijos distintos")
	}
}

func TestLogMatchingHoldsAcrossConflictingSuffixRepair(t *testing.T) {
	c, recorders := newReplicationCluster(t, 3)
	oldID := c.WaitForLeader()
	oldLeader := c.nodes[oldID]

	initial := Command{Op: "SET", Key: "saldo", Value: "100"}
	committedPrefix := proposeClusterCommand(t, oldLeader, initial)
	assertCommittedPrefix(t, c, recorders, c.ids, committedPrefix, []Command{initial})
	assertLogMatchingInvariant(t, c)

	c.Disconnect(oldID)

	for _, value := range []string{"pendiente-1", "pendiente-2"} {
		ctx, cancel := context.WithTimeout(context.Background(), 400*time.Millisecond)
		err := oldLeader.Propose(ctx, Command{Op: "SET", Key: "saldo", Value: value})
		cancel()
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("se esperaba una propuesta pendiente sin quorum, se obtuvo: %v", err)
		}
	}

	oldLeader.mu.Lock()
	divergent := append([]LogEntry(nil), oldLeader.log...)
	oldLeader.mu.Unlock()

	if len(divergent) != len(committedPrefix)+2 {
		t.Fatalf(
			"no se creó el sufijo divergente esperado: longitud=%d, prefijo=%d",
			len(divergent),
			len(committedPrefix),
		)
	}
	assertLogMatchingInvariant(t, c)

	newID := c.WaitForLeader()
	if newID == oldID {
		t.Fatal("el leader aislado conservó quorum")
	}

	updated := Command{Op: "SET", Key: "saldo", Value: "200"}
	repairedPrefix := proposeClusterCommand(t, c.nodes[newID], updated)
	majority := clusterOtherNodes(c, oldID)
	assertCommittedPrefix(
		t,
		c,
		recorders,
		majority,
		repairedPrefix,
		[]Command{initial, updated},
	)

	if len(repairedPrefix) <= len(committedPrefix) {
		t.Fatal("el nuevo leader no produjo un sufijo posterior al prefijo confirmado")
	}
	if divergent[len(committedPrefix)].Term == repairedPrefix[len(committedPrefix)].Term {
		t.Fatal("el escenario no produjo términos conflictivos en el primer índice divergente")
	}
	assertLogMatchingInvariant(t, c)

	c.Reconnect(oldID)
	c.WaitForLeader()
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		repairedPrefix,
		[]Command{initial, updated},
	)
	assertLogMatchingInvariant(t, c)

	oldLeader.mu.Lock()
	repairedOldLog := append([]LogEntry(nil), oldLeader.log...)
	oldLeader.mu.Unlock()
	if len(repairedOldLog) < len(repairedPrefix) ||
		!reflect.DeepEqual(repairedOldLog[:len(repairedPrefix)], repairedPrefix) {
		t.Fatalf("el leader antiguo no reparó su prefijo: log=%+v", repairedOldLog)
	}

	persisted, err := oldLeader.wal.LoadLog()
	if err != nil {
		t.Fatalf("no se pudo leer el WAL del leader antiguo: %v", err)
	}
	if len(persisted) < len(repairedPrefix) ||
		!reflect.DeepEqual(persisted[:len(repairedPrefix)], repairedPrefix) {
		t.Fatalf("el WAL no conserva el prefijo reparado: %+v", persisted)
	}
}

func TestCommittedPrefixRejectsConflictAndSurvivesPendingSuffixRepair(t *testing.T) {
	var applied []Command
	node := newTestNode(t, func(cmd Command) {
		applied = append(applied, cmd)
	})

	entries := []LogEntry{
		{
			Index:   1,
			Term:    1,
			Command: Command{Op: "SET", Key: "a", Value: "1"},
		},
		{
			Index:   2,
			Term:    1,
			Command: Command{Op: "SET", Key: "b", Value: "2"},
		},
		{
			Index:   3,
			Term:    1,
			Command: Command{Op: "SET", Key: "c", Value: "pendiente"},
		},
	}

	reply := node.HandleAppendEntries(AppendEntriesArgs{
		Term:         1,
		LeaderID:     "leader-1",
		Entries:      entries,
		LeaderCommit: 2,
	})
	if !reply.Success {
		t.Fatalf("se rechazó el log inicial: %+v", reply)
	}

	committedPrefix := append([]LogEntry(nil), entries[:2]...)
	wantApplied := []Command{entries[0].Command, entries[1].Command}
	if node.commitIndex != 2 || node.lastApplied != 2 ||
		!reflect.DeepEqual(applied, wantApplied) {
		t.Fatalf(
			"estado inicial inesperado: commitIndex=%d, lastApplied=%d, applied=%+v",
			node.commitIndex,
			node.lastApplied,
			applied,
		)
	}

	conflict := LogEntry{
		Index:   2,
		Term:    2,
		Command: Command{Op: "SET", Key: "b", Value: "conflicto"},
	}
	reply = node.HandleAppendEntries(AppendEntriesArgs{
		Term:         2,
		LeaderID:     "leader-2",
		PrevLogIndex: 1,
		PrevLogTerm:  1,
		Entries:      []LogEntry{conflict},
		LeaderCommit: 2,
	})
	if reply.Success {
		t.Fatalf("se aceptó un reemplazo dentro del prefijo confirmado: %+v", reply)
	}

	node.mu.Lock()
	afterReject := append([]LogEntry(nil), node.log...)
	commitAfterReject := node.commitIndex
	appliedAfterReject := node.lastApplied
	node.mu.Unlock()

	if !reflect.DeepEqual(afterReject[:2], committedPrefix) ||
		commitAfterReject != 2 ||
		appliedAfterReject != 2 ||
		!reflect.DeepEqual(applied, wantApplied) {
		t.Fatalf(
			"el rechazo alteró el prefijo confirmado: log=%+v, commit=%d, applied=%d, efectos=%+v",
			afterReject,
			commitAfterReject,
			appliedAfterReject,
			applied,
		)
	}

	persistedAfterReject, err := node.wal.LoadLog()
	if err != nil {
		t.Fatalf("no se pudo leer el WAL después del rechazo: %v", err)
	}
	if !reflect.DeepEqual(persistedAfterReject[:2], committedPrefix) {
		t.Fatalf("el WAL alteró el prefijo confirmado: %+v", persistedAfterReject)
	}

	replacement := LogEntry{
		Index:   3,
		Term:    2,
		Command: Command{Op: "SET", Key: "c", Value: "3"},
	}
	reply = node.HandleAppendEntries(AppendEntriesArgs{
		Term:         2,
		LeaderID:     "leader-2",
		PrevLogIndex: 2,
		PrevLogTerm:  1,
		Entries:      []LogEntry{replacement},
		LeaderCommit: 3,
	})
	if !reply.Success {
		t.Fatalf("se rechazó la reparación del sufijo pendiente: %+v", reply)
	}

	node.mu.Lock()
	finalLog := append([]LogEntry(nil), node.log...)
	finalCommit := node.commitIndex
	finalApplied := node.lastApplied
	node.mu.Unlock()

	wantLog := append(append([]LogEntry(nil), committedPrefix...), replacement)
	wantApplied = append(wantApplied, replacement.Command)
	if !reflect.DeepEqual(finalLog, wantLog) ||
		finalCommit != 3 ||
		finalApplied != 3 ||
		!reflect.DeepEqual(applied, wantApplied) {
		t.Fatalf(
			"la reparación no preservó el prefijo: log=%+v, commit=%d, applied=%d, efectos=%+v",
			finalLog,
			finalCommit,
			finalApplied,
			applied,
		)
	}

	persistedFinal, err := node.wal.LoadLog()
	if err != nil {
		t.Fatalf("no se pudo leer el WAL final: %v", err)
	}
	if !reflect.DeepEqual(persistedFinal, wantLog) {
		t.Fatalf("el WAL final no coincide con el log reparado: %+v", persistedFinal)
	}
}
