package experiment

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestDockerStatusesQueriesNodesConcurrently(t *testing.T) {
	const nodes = 5

	var mu sync.Mutex
	arrived := 0
	release := make(chan struct{})
	var releaseOnce sync.Once

	ids := make([]string, 0, nodes)
	urls := make(map[string]string, nodes)
	servers := make([]*httptest.Server, 0, nodes)

	for i := 1; i <= nodes; i++ {
		id := fmt.Sprintf("nodo-%d", i)
		ids = append(ids, id)

		nodeID := id
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/status" {
				http.NotFound(w, r)
				return
			}

			mu.Lock()
			arrived++
			if arrived == nodes {
				releaseOnce.Do(func() { close(release) })
			}
			mu.Unlock()

			select {
			case <-release:
			case <-r.Context().Done():
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(NodeStatus{
				ID:          nodeID,
				State:       "seguidor",
				Term:        1,
				CommitIndex: 1,
			})
		}))

		servers = append(servers, server)
		urls[id] = server.URL
	}

	for _, server := range servers {
		defer server.Close()
	}

	backend := &dockerBackend{
		client: &http.Client{Timeout: 2 * time.Second},
		ids:    ids,
		urls:   urls,
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	statuses, err := backend.Statuses(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(statuses) != nodes {
		t.Fatalf("se esperaban %d estados, se obtuvieron %d", nodes, len(statuses))
	}

	mu.Lock()
	gotArrived := arrived
	mu.Unlock()
	if gotArrived != nodes {
		t.Fatalf("no se consultaron todos los nodos concurrentemente: %d/%d", gotArrived, nodes)
	}
}
