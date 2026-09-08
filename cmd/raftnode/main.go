package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/kapumota/raftkv/internal/kv"
	"github.com/kapumota/raftkv/internal/raft"
)

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

	mux.HandleFunc("/kv/set", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Key, Value string }
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if err := node.Propose(raft.Command{Op: "SET", Key: body.Key, Value: body.Value}); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMisdirectedRequest)
			json.NewEncoder(w).Encode(map[string]string{"error": "no_es_lider"})
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"status": "aceptado"})
	})

	mux.HandleFunc("/kv/get", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		v, ok := store.Get(key)
		w.Header().Set("Content-Type", "application/json")
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]string{"error": "no_encontrado"})
			return
		}
		json.NewEncoder(w).Encode(map[string]string{"key": key, "value": v})
	})

	log.Printf("nodo Raft %s escuchando en :%s (peers=%v)", id, port, peers)
	log.Fatal(http.ListenAndServe(":"+port, mux))
}
