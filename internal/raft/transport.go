package raft

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"time"
)

// Transport implementa las RPC de Raft sobre HTTP plano. En un clúster
// Docker esto viaja por la red interna de tipo puente (raftnet), lo que permite
// que el script de partición simule fallas de red reales desconectando contenedores.
type Transport struct {
	client *http.Client
}

func NewTransport() *Transport {
	return &Transport{client: &http.Client{Timeout: 300 * time.Millisecond}}
}

func (t *Transport) post(ctx context.Context, url string, body interface{}, out interface{}) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func (t *Transport) SendRequestVote(peerBaseURL string, args RequestVoteArgs) (RequestVoteReply, error) {
	var reply RequestVoteReply
	err := t.post(context.Background(), peerBaseURL+"/raft/request-vote", args, &reply)
	return reply, err
}

func (t *Transport) SendAppendEntries(peerBaseURL string, args AppendEntriesArgs) (AppendEntriesReply, error) {
	return t.SendAppendEntriesContext(context.Background(), peerBaseURL, args)
}

func (t *Transport) SendAppendEntriesContext(ctx context.Context, peerBaseURL string, args AppendEntriesArgs) (AppendEntriesReply, error) {
	var reply AppendEntriesReply
	err := t.post(ctx, peerBaseURL+"/raft/append-entries", args, &reply)
	return reply, err
}

// RegisterRPCHandlers conecta las rutas internas de Raft al multiplexor HTTP del nodo.
func RegisterRPCHandlers(mux *http.ServeMux, node *Node) {
	mux.HandleFunc("/raft/request-vote", func(w http.ResponseWriter, r *http.Request) {
		var args RequestVoteArgs
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		reply := node.HandleRequestVote(args)
		json.NewEncoder(w).Encode(reply)
	})
	mux.HandleFunc("/raft/append-entries", func(w http.ResponseWriter, r *http.Request) {
		var args AppendEntriesArgs
		if err := json.NewDecoder(r.Body).Decode(&args); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		reply := node.HandleAppendEntries(args)
		json.NewEncoder(w).Encode(reply)
	})
}
