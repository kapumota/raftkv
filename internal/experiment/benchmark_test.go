package experiment

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

type normalBackend struct {
	fakeBackend
	size int
}

func (b *normalBackend) Statuses(ctx context.Context) ([]NodeStatus, error) {
	nodes, err := b.fakeBackend.Statuses(ctx)
	return nodes[:b.size], err
}

func TestNormalBenchmarks(t *testing.T) {
	for _, size := range []int{3, 5} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			data, err := os.ReadFile(fmt.Sprintf("../../experiments/configs/normal-%d.yaml", size))
			if err != nil {
				t.Fatal(err)
			}
			config, err := LoadConfig(strings.NewReader(string(data)))
			if err != nil {
				t.Fatal(err)
			}
			config.Duration, config.Rate = 3, 1
			b := &normalBackend{size: size}
			result, err := runExperiment(context.Background(), config, b)
			if err != nil || len(result.Operations) != 3 || len(result.Events) != 0 || b.applyNode != "" || b.restoreNode != "" {
				t.Fatalf("se inyectó una falla o no terminó el benchmark: %+v, %v", result, err)
			}
			d := newDockerBackend(config)
			if len(d.ids) != size || d.ids[0] != fmt.Sprintf("raft-bench-%d-node-1", size) || d.urls[d.ids[0]] != fmt.Sprintf("http://127.0.0.1:%d", 18081+size*100) {
				t.Fatal("el backend no coincide con el despliegue")
			}
		})
	}
}

func TestBenchmarkConfigValidation(t *testing.T) {
	legacy, err := LoadConfig(strings.NewReader(validScenario))
	if err != nil || legacy.Nodes != 5 || legacy.Deployment != "local" {
		t.Fatalf("se rompió la configuración F1: %v", err)
	}
	for _, suffix := range []string{"nodos: 0\n", "nodos: 4\n", "nodos: 3\n", "nodos: 5\nnodos: 5\n", "despliegue: desconocido\n"} {
		if _, err := LoadConfig(strings.NewReader(validScenario + suffix)); err == nil {
			t.Fatalf("se aceptó una topología inválida: %s", suffix)
		}
	}
	legacy.Fault.Type = "ninguna"
	if err := legacy.Validate(); err == nil {
		t.Fatal("se aceptaron tiempos de falla en un benchmark normal")
	}
}

func TestCompareNormalRuns(t *testing.T) {
	config, _ := LoadConfig(strings.NewReader(validScenario))
	config.Nodes, config.Deployment, config.Fault = 3, "benchmark", FaultConfig{Type: "ninguna"}
	start := time.Unix(1000, 0)
	a := ExperimentResult{Config: config, Started: start, Finished: start.Add(3 * time.Second)}
	for i := 0; i < 3; i++ {
		a.Operations = append(a.Operations, OperationResult{Sequence: i, Client: 0, Outcome: "confirmada", Status: 200, Elapsed: time.Millisecond})
	}
	b := a
	b.Config.Nodes = 5
	m, err := CompareNormalRuns(a, b)
	if err != nil || *m[0].Throughput != 1 || *m[1].Throughput != 1 {
		t.Fatalf("comparación incorrecta: %v", err)
	}
	b.Config.Seed++
	if _, err := CompareNormalRuns(a, b); err == nil {
		t.Fatal("se compararon semillas distintas")
	}
	b.Config.Seed = a.Config.Seed
	b.Error = "interrumpido"
	if _, err := CompareNormalRuns(a, b); err == nil {
		t.Fatal("se comparó una ejecución incompleta")
	}
}
