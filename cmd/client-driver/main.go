package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"time"
)

// client-driver genera carga contra el clúster sin conocer de antemano
// quién es el líder: intenta un nodo al azar y, si la escritura es rechazada,
// rota a otro nodo hasta encontrar al líder vigente. Esto imita cómo se
// comportaría un cliente real frente a un clúster Raft.

func main() {
	nodesEnv := os.Getenv("NODES")
	if nodesEnv == "" {
		log.Fatal("se requiere la variable de entorno NODES con URLs base separadas por coma")
	}
	nodes := strings.Split(nodesEnv, ",")

	client := &http.Client{Timeout: 1 * time.Second}
	var latencies []time.Duration
	var writes, failures int

	writeTicker := time.NewTicker(300 * time.Millisecond)
	reportTicker := time.NewTicker(10 * time.Second)
	defer writeTicker.Stop()
	defer reportTicker.Stop()

	current := nodes[rand.Intn(len(nodes))]
	i := 0

	for {
		select {
		case <-writeTicker.C:
			i++
			key := fmt.Sprintf("k%d", i)
			val := fmt.Sprintf("v%d", i)
			start := time.Now()
			ok := trySet(client, current, key, val)
			if !ok {
				// Probablemente hablamos con un nodo seguidor o el nodo está
				// inalcanzable por una partición; probamos otro nodo del clúster.
				current = nodes[rand.Intn(len(nodes))]
				ok = trySet(client, current, key, val)
			}
			if ok {
				writes++
				latencies = append(latencies, time.Since(start))
			} else {
				failures++
			}
		case <-reportTicker.C:
			printReport(writes, failures, latencies, current)
			writes, failures = 0, 0
			latencies = nil
		}
	}
}

func trySet(client *http.Client, base, key, val string) bool {
	b, _ := json.Marshal(map[string]string{"Key": key, "Value": val})
	resp, err := client.Post(base+"/kv/set", "application/json", bytes.NewReader(b))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func printReport(writes, failures int, latencies []time.Duration, leaderGuess string) {
	if len(latencies) == 0 {
		fmt.Printf("[client-driver] escrituras=0 fallos=%d (sin escrituras exitosas en esta ventana, líder estimado: %s)\n", failures, leaderGuess)
		return
	}
	var total time.Duration
	for _, l := range latencies {
		total += l
	}
	avg := total / time.Duration(len(latencies))
	fmt.Printf("[client-driver] escrituras=%d fallos=%d latencia_promedio=%s lider_estimado=%s\n", writes, failures, avg, leaderGuess)
}
