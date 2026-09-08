package kv

import (
	"sync"

	"github.com/kapumota/raftkv/internal/raft"
)

// Store es la máquina de estados replicada. Cada nodo mantiene su propia
// copia; Raft garantiza que todos aplican los mismos comandos en el mismo
// orden, así que todas las copias convergen.
type Store struct {
	mu   sync.RWMutex
	data map[string]string
}

func NewStore() *Store {
	return &Store{data: map[string]string{}}
}

// Apply se registra como el ApplyFunc del nodo Raft: solo se invoca para
// comandos ya comprometidos (commitIndex avanzado), nunca para propuestas
// que aún no alcanzaron mayoría.
func (s *Store) Apply(cmd raft.Command) {
	if cmd.Op != "SET" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[cmd.Key] = cmd.Value
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}
