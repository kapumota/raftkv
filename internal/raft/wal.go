package raft

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
)

// WAL persiste el log replicado y el estado mínimo (currentTerm, votedFor)
// que Raft exige sobrevivir a un reinicio del proceso. Cada nodo tiene su
// propio directorio de datos, montado como volumen Docker independiente.
type WAL struct {
	mu        sync.Mutex
	logPath   string
	statePath string
}

type PersistentState struct {
	CurrentTerm int    `json:"current_term"`
	VotedFor    string `json:"voted_for"`
}

func NewWAL(dir string) (*WAL, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return &WAL{logPath: dir + "/log.jsonl", statePath: dir + "/state.json"}, nil
}

// AppendEntries agrega entradas nuevas al final del log en disco; el caso común ocurre cuando el líder propone.
func (w *WAL) AppendEntries(entries []LogEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.OpenFile(w.logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			return err
		}
	}
	return f.Sync()
}

// RewriteLog reescribe el archivo completo. Se usa cuando un nodo seguidor debe
// descartar entradas en conflicto y revertir una parte del log de Raft.
func (w *WAL) RewriteLog(entries []LogEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	tmp := w.logPath + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, e := range entries {
		if err := enc.Encode(e); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	f.Close()
	return os.Rename(tmp, w.logPath)
}

func (w *WAL) LoadLog() ([]LogEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	f, err := os.Open(w.logPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []LogEntry
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var e LogEntry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, sc.Err()
}

func (w *WAL) SaveState(s PersistentState) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	tmp := w.statePath + ".tmp"
	b, err := json.Marshal(s)
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, w.statePath)
}

func (w *WAL) LoadState() (PersistentState, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	b, err := os.ReadFile(w.statePath)
	if os.IsNotExist(err) {
		return PersistentState{}, nil
	}
	if err != nil {
		return PersistentState{}, err
	}
	var s PersistentState
	err = json.Unmarshal(b, &s)
	return s, err
}
