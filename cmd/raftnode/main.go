package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kapumota/raftkv/internal/kv"
	"github.com/kapumota/raftkv/internal/raft"
)

const (
	kvProposalTimeout = 5 * time.Second
	kvReadTimeout     = 5 * time.Second
)

type proposalNode interface {
	Propose(ctx context.Context, cmd raft.Command) error
}

type readNode interface {
	ReadIndex(ctx context.Context) (int, error)
}

type keyValueReader interface {
	Get(key string) (string, bool)
}

func writeJSON(w http.ResponseWriter, status int, payload map[string]string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

// newKVSetHandler crea el handler de escritura y limita el tiempo de espera
// para evitar que una petición quede abierta indefinidamente sin mayoría.
func newKVSetHandler(node proposalNode, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", http.MethodPost)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "metodo_no_permitido"})
			return
		}

		var body struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "solicitud_invalida"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		err := node.Propose(ctx, raft.Command{Op: "SET", Key: body.Key, Value: body.Value})
		switch {
		case err == nil:
			writeJSON(w, http.StatusOK, map[string]string{"estado": "confirmado"})
		case errors.Is(err, raft.ErrNotLeader):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no_es_lider"})
		case errors.Is(err, context.DeadlineExceeded):
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "tiempo_de_espera_agotado"})
		case errors.Is(err, context.Canceled):
			writeJSON(w, http.StatusRequestTimeout, map[string]string{"error": "solicitud_cancelada"})
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "error_interno"})
		}
	}
}

// newKVGetHandler confirma el liderazgo antes de consultar la máquina de
// estados y limita el tiempo durante el cual se intenta alcanzar mayoría.
func newKVGetHandler(node readNode, store keyValueReader, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "metodo_no_permitido"})
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()

		_, err := node.ReadIndex(ctx)
		switch {
		case err == nil:
		case errors.Is(err, raft.ErrNotLeader):
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "no_es_lider"})
			return
		case errors.Is(err, context.DeadlineExceeded):
			writeJSON(w, http.StatusGatewayTimeout, map[string]string{"error": "tiempo_de_espera_agotado"})
			return
		case errors.Is(err, context.Canceled):
			writeJSON(w, http.StatusRequestTimeout, map[string]string{"error": "solicitud_cancelada"})
			return
		default:
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "error_interno"})
			return
		}

		key := r.URL.Query().Get("key")
		value, ok := store.Get(key)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "no_encontrado"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"key": key, "value": value})
	}
}

func main() {
	id := os.Getenv("NODE_ID")
	if id == "" {
		log.Fatal("se requiere la variable de entorno NODE_ID")
	}
	var peers []string
	if p := os.Getenv("PEERS"); p != "" {
		peers = strings.Split(p, ",")
	}
	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		dataDir = "/data"
	}
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	store := kv.NewStore()
	wal, err := raft.NewWAL(dataDir)
	if err != nil {
		log.Fatalf("no se pudo inicializar el WAL: %v", err)
	}
	transport := raft.NewTransport()
	node := raft.NewNode(id, peers, wal, transport, store.Apply)
	node.Run()

	mux := http.NewServeMux()
	raft.RegisterRPCHandlers(mux, node)

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		nid, state, term, votedFor, commitIndex := node.Status()
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":           nid,
			"state":        state,
			"term":         term,
			"voted_for":    votedFor,
			"commit_index": commitIndex,
		})
	})

	mux.HandleFunc("/kv/set", newKVSetHandler(node, kvProposalTimeout))
	mux.HandleFunc("/kv/get", newKVGetHandler(node, store, kvReadTimeout))

	log.Printf("nodo Raft %s escuchando en :%s (peers=%v)", id, port, peers)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
