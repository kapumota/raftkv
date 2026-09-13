package raft

import (
	"fmt"
	"reflect"
	"testing"
)

func nodeContainsEntry(node *Node, expected LogEntry) bool {
	node.mu.Lock()
	defer node.mu.Unlock()

	if expected.Index < 1 || expected.Index > len(node.log) {
		return false
	}
	return node.log[expected.Index-1] == expected
}

func assertObservedLeaderContainsCommittedEntry(
	t *testing.T,
	c *TestCluster,
	leaderID string,
	expected LogEntry,
) {
	t.Helper()

	node := c.nodes[leaderID]
	node.mu.Lock()
	state := node.state
	term := node.currentTerm
	logCopy := append([]LogEntry(nil), node.log...)
	node.mu.Unlock()

	if expected.Index < 1 || expected.Index > len(logCopy) {
		t.Fatalf(
			"leader observado %s no contiene el índice committed %d: estado=%s, término=%d, log=%+v",
			leaderID,
			expected.Index,
			state,
			term,
			logCopy,
		)
	}
	if logCopy[expected.Index-1] != expected {
		t.Fatalf(
			"leader observado %s alteró la entrada committed en índice %d: estado=%s, término=%d, got=%+v, want=%+v",
			leaderID,
			expected.Index,
			state,
			term,
			logCopy[expected.Index-1],
			expected,
		)
	}
}

func assertNodeDoesNotContainEntry(
	t *testing.T,
	nodeID string,
	node *Node,
	expected LogEntry,
) {
	t.Helper()

	node.mu.Lock()
	defer node.mu.Unlock()

	if expected.Index <= len(node.log) && node.log[expected.Index-1] == expected {
		t.Fatalf(
			"%s ya contiene la entrada que debía permanecer ausente: índice=%d, término=%d",
			nodeID,
			expected.Index,
			expected.Term,
		)
	}
}

func TestCommittedEntryAppearsInSuccessiveLeaders(t *testing.T) {
	c, recorders := newReplicationCluster(t, 5)
	initialLeaderID := c.WaitForLeader()
	initialLeader := c.nodes[initialLeaderID]

	var followers []string
	for _, id := range c.ids {
		if id != initialLeaderID {
			followers = append(followers, id)
		}
	}
	if len(followers) != 4 {
		t.Fatalf("cantidad inesperada de followers: %d", len(followers))
	}

	quorumFollowers := followers[:2]
	staleFollowers := followers[2:]

	for _, id := range staleFollowers {
		c.Disconnect(id)
	}

	committedCommand := Command{
		Op:    "SET",
		Key:   "saldo",
		Value: "100",
	}
	committedPrefix := proposeClusterCommand(t, initialLeader, committedCommand)
	if len(committedPrefix) == 0 {
		t.Fatal("la propuesta confirmada no produjo una entrada en el log")
	}
	committedEntry := committedPrefix[len(committedPrefix)-1]
	if committedEntry.Command != committedCommand {
		t.Fatalf(
			"la última entrada confirmada no corresponde al comando: %+v",
			committedEntry,
		)
	}

	initialQuorum := []string{
		initialLeaderID,
		quorumFollowers[0],
		quorumFollowers[1],
	}
	assertCommittedPrefix(
		t,
		c,
		recorders,
		initialQuorum,
		committedPrefix,
		[]Command{committedCommand},
	)
	assertObservedLeaderContainsCommittedEntry(t, c, initialLeaderID, committedEntry)

	for _, id := range staleFollowers {
		assertNodeDoesNotContainEntry(t, id, c.nodes[id], committedEntry)
	}

	// El leader inicial deja de participar. Dos nodos que sí contienen la
	// entrada permanecen conectados, pero todavía no constituyen mayoría de 5.
	c.Disconnect(initialLeaderID)

	// Se incorpora un follower atrasado para volver a tener tres nodos
	// comunicados. Mientras carezca de la entrada committed no puede obtener los
	// votos de los dos nodos cuyos logs están más actualizados.
	firstStaleID := staleFollowers[0]
	c.Reconnect(firstStaleID)

	firstReplacementID := c.WaitForLeader()
	if firstReplacementID == initialLeaderID {
		t.Fatal("el leader aislado volvió a ser reconocido con quorum")
	}
	if firstReplacementID == firstStaleID &&
		!nodeContainsEntry(c.nodes[firstStaleID], committedEntry) {
		t.Fatal("un nodo atrasado se convirtió en leader sin la entrada committed")
	}
	assertObservedLeaderContainsCommittedEntry(t, c, firstReplacementID, committedEntry)

	// WaitForLeader confirma el liderazgo mediante una ronda de AppendEntries.
	// Después de esa ronda, el follower que estaba atrasado debe poder ponerse al
	// día antes de ser candidato válido en una elección posterior.
	waitForReplication(t, "poner al día al primer follower atrasado", func() (bool, string) {
		if nodeContainsEntry(c.nodes[firstStaleID], committedEntry) {
			return true, ""
		}
		return false, fmt.Sprintf(
			"%s aún no contiene índice=%d, término=%d",
			firstStaleID,
			committedEntry.Index,
			committedEntry.Term,
		)
	})

	// Fuerza un segundo cambio de leader. El segundo follower sigue aislado y
	// debe continuar sin la entrada committed hasta este punto.
	c.Disconnect(firstReplacementID)

	secondStaleID := staleFollowers[1]
	assertNodeDoesNotContainEntry(t, secondStaleID, c.nodes[secondStaleID], committedEntry)
	c.Reconnect(secondStaleID)

	// Al volver a tener tres nodos comunicados, cualquier leader posterior debe
	// conservar la entrada ya committed antes de ejecutar una operación nueva.
	secondReplacementID := c.WaitForLeader()
	if secondReplacementID == initialLeaderID ||
		secondReplacementID == firstReplacementID {
		t.Fatalf(
			"se reconoció un leader que debía seguir aislado: %s",
			secondReplacementID,
		)
	}
	if secondReplacementID == secondStaleID &&
		!nodeContainsEntry(c.nodes[secondStaleID], committedEntry) {
		t.Fatal("el segundo nodo atrasado lideró sin la entrada committed")
	}
	assertObservedLeaderContainsCommittedEntry(t, c, secondReplacementID, committedEntry)

	// Hasta este punto no se propuso ningún comando nuevo después del original.
	// La evidencia de Leader Completeness se obtiene antes de depender de una
	// escritura posterior para reparar o volver observable la entrada.
	secondLeader := c.nodes[secondReplacementID]
	secondLeader.mu.Lock()
	observedEntry := secondLeader.log[committedEntry.Index-1]
	secondLeader.mu.Unlock()
	if observedEntry != committedEntry {
		t.Fatalf(
			"el segundo leader no conserva la entrada original antes de nuevas escrituras: got=%+v, want=%+v",
			observedEntry,
			committedEntry,
		)
	}

	// Restablece todos los enlaces y exige convergencia del prefijo confirmado.
	c.Heal()
	c.WaitForLeader()
	assertCommittedPrefix(
		t,
		c,
		recorders,
		c.ids,
		committedPrefix,
		[]Command{committedCommand},
	)

	for _, id := range c.ids {
		node := c.nodes[id]
		node.mu.Lock()
		if len(node.log) < len(committedPrefix) {
			gotLen := len(node.log)
			node.mu.Unlock()
			t.Fatalf(
				"%s perdió el prefijo committed: longitud=%d, esperada al menos=%d",
				id,
				gotLen,
				len(committedPrefix),
			)
		}
		prefix := append([]LogEntry(nil), node.log[:len(committedPrefix)]...)
		node.mu.Unlock()

		if !reflect.DeepEqual(prefix, committedPrefix) {
			t.Fatalf(
				"%s no convergió al prefijo committed: got=%+v, want=%+v",
				id,
				prefix,
				committedPrefix,
			)
		}
	}
}
