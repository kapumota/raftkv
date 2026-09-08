package raft

import (
	"context"
	"errors"
	"math/rand"
	"sync"
	"time"
)

// ApplyFunc aplica un comando ya comprometido (commitIndex avanzado) a la
// máquina de estados local (el KV store).
type ApplyFunc func(cmd Command)

var ErrNotLeader = errors.New("el nodo no es líder")

// Node implementa el algoritmo Raft descrito en el artículo original
// (Ongaro & Ousterhout, 2014), sin instantáneas ni cambios dinámicos de
// membresía. Está pensado para estudiar el algoritmo, no para producción.
type Node struct {
	mu sync.Mutex

	id    string
	peers []string // URLs HTTP base de los otros nodos

	state       State
	currentTerm int
	votedFor    string

	log         []LogEntry // log[i] = entrada de índice i+1
	commitIndex int
	lastApplied int

	nextIndex      map[string]int
	matchIndex     map[string]int
	pendingCommits map[int]chan error

	wal       *WAL
	transport *Transport
	apply     ApplyFunc

	electionResetAt time.Time
	electionTimeout time.Duration

	stopCh chan struct{}
}

func NewNode(id string, peers []string, wal *WAL, transport *Transport, apply ApplyFunc) *Node {
	st, _ := wal.LoadState()
	log, _ := wal.LoadLog()
	n := &Node{
		id:             id,
		peers:          peers,
		state:          Follower,
		currentTerm:    st.CurrentTerm,
		votedFor:       st.VotedFor,
		log:            log,
		commitIndex:    validCommitIndex(st.CommitIndex, len(log)),
		nextIndex:      map[string]int{},
		matchIndex:     map[string]int{},
		pendingCommits: map[int]chan error{},
		wal:            wal,
		transport:      transport,
		apply:          apply,
		stopCh:         make(chan struct{}),
	}
	// Reconstruye la máquina de estados solo con las entradas confirmadas antes
	// del reinicio. Las entradas posteriores a commitIndex permanecen sin aplicar.
	n.applyCommitted()
	n.resetElectionTimer()
	return n
}

func validCommitIndex(commitIndex, logLength int) int {
	if commitIndex < 0 {
		return 0
	}
	if commitIndex > logLength {
		return logLength
	}
	return commitIndex
}

// saveState asume que el llamador ya mantiene bloqueado n.mu.
func (n *Node) saveState() error {
	return n.wal.SaveState(PersistentState{
		CurrentTerm: n.currentTerm,
		VotedFor:    n.votedFor,
		CommitIndex: n.commitIndex,
	})
}

// resetElectionTimer usa un tiempo de espera aleatorio amplio (1.5s-3s) porque en un
// clúster Docker real la latencia entre contenedores es más variable que en
// procesos locales simulados; tiempos de espera cortos generan elecciones espurias.
func (n *Node) resetElectionTimer() {
	n.electionResetAt = time.Now()
	n.electionTimeout = time.Duration(1500+rand.Intn(1500)) * time.Millisecond
}

func (n *Node) Run() {
	go n.electionTicker()
}

func (n *Node) electionTicker() {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-n.stopCh:
			return
		case <-ticker.C:
			n.mu.Lock()
			state := n.state
			elapsed := time.Since(n.electionResetAt)
			timeout := n.electionTimeout
			n.mu.Unlock()
			if state != Leader && elapsed >= timeout {
				n.startElection()
			}
		}
	}
}

func (n *Node) startElection() {
	n.mu.Lock()
	n.state = Candidate
	n.currentTerm++
	n.votedFor = n.id
	term := n.currentTerm
	lastLogIndex := len(n.log)
	lastLogTerm := 0
	if lastLogIndex > 0 {
		lastLogTerm = n.log[lastLogIndex-1].Term
	}
	_ = n.saveState()
	n.resetElectionTimer()
	peers := append([]string{}, n.peers...)
	n.mu.Unlock()

	votes := 1
	var voteMu sync.Mutex
	majority := make(chan struct{}, 1)

	for _, p := range peers {
		go func(peer string) {
			reply, err := n.transport.SendRequestVote(peer, RequestVoteArgs{
				Term: term, CandidateID: n.id, LastLogIndex: lastLogIndex, LastLogTerm: lastLogTerm,
			})
			if err != nil {
				return
			}
			n.mu.Lock()
			if reply.Term > n.currentTerm {
				n.becomeFollower(reply.Term)
				n.mu.Unlock()
				return
			}
			n.mu.Unlock()
			if reply.VoteGranted {
				voteMu.Lock()
				votes++
				v := votes
				voteMu.Unlock()
				if v > (len(peers)+1)/2 {
					select {
					case majority <- struct{}{}:
					default:
					}
				}
			}
		}(p)
	}

	select {
	case <-majority:
		n.mu.Lock()
		if n.state == Candidate && n.currentTerm == term {
			n.becomeLeader()
		}
		n.mu.Unlock()
	case <-time.After(2 * time.Second):
		// No se alcanzó mayoría a tiempo; el temporizador de elección reintentará.
	}
}

// becomeFollower asume que el llamador ya mantiene bloqueado n.mu.
func (n *Node) becomeFollower(term int) {
	wasLeader := n.state == Leader
	n.state = Follower
	n.currentTerm = term
	n.votedFor = ""
	_ = n.saveState()
	n.resetElectionTimer()
	if wasLeader {
		n.failPendingCommits(ErrNotLeader)
	}
}

// becomeLeader asume que el llamador ya mantiene bloqueado n.mu.
func (n *Node) becomeLeader() {
	n.state = Leader
	for _, p := range n.peers {
		n.nextIndex[p] = len(n.log) + 1
		n.matchIndex[p] = 0
	}
	go n.leaderLoop()
}

func (n *Node) leaderLoop() {
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	for {
		n.mu.Lock()
		isLeader := n.state == Leader
		n.mu.Unlock()
		if !isLeader {
			return
		}
		n.broadcastAppendEntries()
		select {
		case <-ticker.C:
		case <-n.stopCh:
			return
		}
	}
}

func (n *Node) broadcastAppendEntries() {
	n.mu.Lock()
	term := n.currentTerm
	leaderCommit := n.commitIndex
	peers := append([]string{}, n.peers...)
	logCopy := append([]LogEntry{}, n.log...)
	nextIdx := map[string]int{}
	for k, v := range n.nextIndex {
		nextIdx[k] = v
	}
	n.mu.Unlock()

	for _, p := range peers {
		go func(peer string) {
			ni := nextIdx[peer]
			if ni < 1 {
				ni = 1
			}
			prevLogIndex := ni - 1
			prevLogTerm := 0
			if prevLogIndex > 0 && prevLogIndex <= len(logCopy) {
				prevLogTerm = logCopy[prevLogIndex-1].Term
			}
			var entries []LogEntry
			if ni <= len(logCopy) {
				entries = logCopy[ni-1:]
			}
			reply, err := n.transport.SendAppendEntries(peer, AppendEntriesArgs{
				Term: term, LeaderID: n.id, PrevLogIndex: prevLogIndex, PrevLogTerm: prevLogTerm,
				Entries: entries, LeaderCommit: leaderCommit,
			})
			if err != nil {
				return // El nodo par está inalcanzable; se reintentará en el próximo latido.
			}
			n.mu.Lock()
			defer n.mu.Unlock()
			if reply.Term > n.currentTerm {
				n.becomeFollower(reply.Term)
				return
			}
			if n.state != Leader || n.currentTerm != term {
				return
			}
			if reply.Success {
				n.matchIndex[peer] = prevLogIndex + len(entries)
				n.nextIndex[peer] = n.matchIndex[peer] + 1
				n.advanceCommitIndex()
			} else if n.nextIndex[peer] > 1 {
				n.nextIndex[peer]--
			}
		}(p)
	}
}

// advanceCommitIndex asume que el llamador ya mantiene bloqueado n.mu.
func (n *Node) advanceCommitIndex() {
	for idx := len(n.log); idx > n.commitIndex; idx-- {
		if n.log[idx-1].Term != n.currentTerm {
			continue // Raft solo puede comprometer directamente entradas del término actual
		}
		count := 1 // el líder se cuenta a sí mismo
		for _, m := range n.matchIndex {
			if m >= idx {
				count++
			}
		}
		if count > (len(n.peers)+1)/2 {
			previousCommitIndex := n.commitIndex
			n.commitIndex = idx
			if err := n.saveState(); err != nil {
				n.commitIndex = previousCommitIndex
				n.failPendingCommits(err)
				return
			}
			n.applyCommitted()
			n.notifyCommitted()
			break
		}
	}
}

// notifyCommitted asume que el llamador ya mantiene bloqueado n.mu.
// Despierta las propuestas cuyo índice ya fue confirmado por mayoría.
func (n *Node) notifyCommitted() {
	for idx, done := range n.pendingCommits {
		if idx <= n.commitIndex {
			done <- nil
			close(done)
			delete(n.pendingCommits, idx)
		}
	}
}

// failPendingCommits asume que el llamador ya mantiene bloqueado n.mu.
// Finaliza las propuestas pendientes cuando el nodo pierde el liderazgo.
func (n *Node) failPendingCommits(err error) {
	for idx, done := range n.pendingCommits {
		done <- err
		close(done)
		delete(n.pendingCommits, idx)
	}
}

// applyCommitted asume que el llamador ya mantiene bloqueado n.mu.
func (n *Node) applyCommitted() {
	for n.lastApplied < n.commitIndex {
		n.lastApplied++
		entry := n.log[n.lastApplied-1]
		if n.apply != nil {
			n.apply(entry.Command)
		}
	}
}

func (n *Node) HandleRequestVote(args RequestVoteArgs) RequestVoteReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	if args.Term > n.currentTerm {
		n.becomeFollower(args.Term)
	}

	reply := RequestVoteReply{Term: n.currentTerm, VoteGranted: false}
	if args.Term < n.currentTerm {
		return reply
	}

	lastLogIndex := len(n.log)
	lastLogTerm := 0
	if lastLogIndex > 0 {
		lastLogTerm = n.log[lastLogIndex-1].Term
	}
	logOk := args.LastLogTerm > lastLogTerm ||
		(args.LastLogTerm == lastLogTerm && args.LastLogIndex >= lastLogIndex)

	if (n.votedFor == "" || n.votedFor == args.CandidateID) && logOk {
		n.votedFor = args.CandidateID
		_ = n.saveState()
		n.resetElectionTimer()
		reply.VoteGranted = true
	}
	return reply
}

func (n *Node) HandleAppendEntries(args AppendEntriesArgs) AppendEntriesReply {
	n.mu.Lock()
	defer n.mu.Unlock()

	reply := AppendEntriesReply{Term: n.currentTerm, Success: false}
	if args.Term < n.currentTerm {
		return reply
	}
	if args.Term > n.currentTerm || n.state != Follower {
		n.becomeFollower(args.Term)
	}
	n.resetElectionTimer()

	if args.PrevLogIndex > 0 {
		if args.PrevLogIndex > len(n.log) {
			return reply
		}
		if n.log[args.PrevLogIndex-1].Term != args.PrevLogTerm {
			return reply
		}
	}

	if len(args.Entries) > 0 {
		newLog := append([]LogEntry{}, n.log[:args.PrevLogIndex]...)
		newLog = append(newLog, args.Entries...)
		n.log = newLog
		_ = n.wal.RewriteLog(n.log)
	}

	if args.LeaderCommit > n.commitIndex {
		previousCommitIndex := n.commitIndex
		if args.LeaderCommit < len(n.log) {
			n.commitIndex = args.LeaderCommit
		} else {
			n.commitIndex = len(n.log)
		}
		if err := n.saveState(); err != nil {
			n.commitIndex = previousCommitIndex
			return reply
		}
		n.applyCommitted()
	}

	reply.Term = n.currentTerm
	reply.Success = true
	return reply
}

// Propose agrega un comando al log si este nodo es el líder actual y espera
// hasta que la entrada sea confirmada por una mayoría o el contexto termine.
// Devuelve ErrNotLeader si el nodo no es el líder actual.
func (n *Node) Propose(ctx context.Context, cmd Command) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	n.mu.Lock()
	if n.state != Leader {
		n.mu.Unlock()
		return ErrNotLeader
	}

	entry := LogEntry{Term: n.currentTerm, Index: len(n.log) + 1, Command: cmd}
	done := make(chan error, 1)
	n.log = append(n.log, entry)
	n.pendingCommits[entry.Index] = done
	_ = n.wal.AppendEntries([]LogEntry{entry})

	// Permite que un clúster de un solo nodo confirme inmediatamente.
	n.advanceCommitIndex()
	n.mu.Unlock()

	n.broadcastAppendEntries()

	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		n.mu.Lock()
		committed := entry.Index <= n.commitIndex
		if current, ok := n.pendingCommits[entry.Index]; ok && current == done {
			delete(n.pendingCommits, entry.Index)
		}
		n.mu.Unlock()
		if committed {
			return nil
		}
		return ctx.Err()
	}
}

func (n *Node) Status() (id string, state string, term int, votedFor string, commitIndex int) {
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.id, n.state.String(), n.currentTerm, n.votedFor, n.commitIndex
}
