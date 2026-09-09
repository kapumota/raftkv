package kv

import (
	"sync"

	"github.com/kapumota/raftkv/internal/raft"
)

// ClientSession registra la última solicitud aplicada de un cliente.
// Los identificadores deben crecer y cada cliente debe esperar la respuesta
// de una solicitud antes de enviar la siguiente.
type ClientSession struct {
	LastRequestID uint64
}

// Store es la máquina de estados replicada. Cada nodo mantiene su propia
// copia; Raft garantiza que todos aplican los mismos comandos en el mismo
// orden, así que todas las copias convergen.
type Store struct {
	mu       sync.RWMutex
	data     map[string]string
	sessions map[string]ClientSession
}

func NewStore() *Store {
	return &Store{
		data:     map[string]string{},
		sessions: map[string]ClientSession{},
	}
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
	if cmd.ClientID != "" {
		session, ok := s.sessions[cmd.ClientID]
		if ok && cmd.RequestID <= session.LastRequestID {
			return
		}
	}

	// El efecto y la sesión se actualizan bajo el mismo bloqueo. La reproducción
	// del log confirmado reconstruye ambos al reiniciar.
	s.data[cmd.Key] = cmd.Value
	if cmd.ClientID != "" {
		s.sessions[cmd.ClientID] = ClientSession{LastRequestID: cmd.RequestID}
	}
}

func (s *Store) Get(key string) (string, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	return v, ok
}
