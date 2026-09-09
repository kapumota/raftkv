package experiment

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func benchmarkResult(t *testing.T, config ExperimentConfig, confirmed int) ExperimentResult {
	t.Helper()
	start := time.Unix(1000, 0)
	result := ExperimentResult{
		Config:   config,
		Started:  start,
		Finished: start.Add(time.Duration(config.Duration) * time.Second),
	}
	for i := 0; i < config.Duration*config.Rate; i++ {
		outcome, status := "incierta", 0
		if i < confirmed {
			outcome, status = "confirmada", 200
		}
		result.Operations = append(result.Operations, OperationResult{
			Sequence: i,
			Client:   i % config.Clients,
			Outcome:  outcome,
			Status:   status,
			Elapsed:  time.Duration(i+1) * time.Millisecond,
		})
	}
	return result
}

func TestG3ScenarioFiles(t *testing.T) {
	for _, name := range []string{
		"normal-faults-5.yaml",
		"follower-down-5.yaml",
		"leader-down-5.yaml",
		"leader-partition-5.yaml",
	} {
		data, err := os.ReadFile("../../experiments/configs/" + name)
		if err != nil {
			t.Fatal(err)
		}
		config, err := LoadConfig(strings.NewReader(string(data)))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if config.Nodes != 5 || config.Deployment != "benchmark" || config.Name != "benchmark_fallas" {
			t.Fatalf("%s no pertenece al benchmark G3", name)
		}
	}
}

func TestAggregateBenchmarkRuns(t *testing.T) {
	config, err := LoadConfig(strings.NewReader(`version: 1
nombre: benchmark_fallas
semilla: 42
nodos: 5
despliegue: benchmark
duracion_segundos: 3
clientes: 1
operaciones_por_segundo: 1
falla:
  tipo: ninguna
  objetivo: ""
  instante_segundos: 0
  duracion_segundos: 0
`))
	if err != nil {
		t.Fatal(err)
	}

	results := []ExperimentResult{
		benchmarkResult(t, config, 1),
		benchmarkResult(t, config, 2),
		benchmarkResult(t, config, 3),
	}
	summary, err := AggregateBenchmarkRuns(results)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Runs != 3 || summary.ThroughputMedian == nil || summary.LatencyP95Median == nil || summary.RestoredRuns != 0 {
		t.Fatalf("resumen inesperado: %+v", summary)
	}

	results[2].Config.Seed++
	if _, err := AggregateBenchmarkRuns(results); err == nil {
		t.Fatal("se mezclaron configuraciones diferentes")
	}
}

func TestAggregateBenchmarkRunsRequiresRecovery(t *testing.T) {
	config, err := LoadConfig(strings.NewReader(`version: 1
nombre: benchmark_fallas
semilla: 42
nodos: 5
despliegue: benchmark
duracion_segundos: 3
clientes: 1
operaciones_por_segundo: 1
falla:
  tipo: caida
  objetivo: lider
  instante_segundos: 1
  duracion_segundos: 1
`))
	if err != nil {
		t.Fatal(err)
	}

	result := benchmarkResult(t, config, 2)
	if _, err := AggregateBenchmarkRuns([]ExperimentResult{result}); err == nil {
		t.Fatal("se aceptó una ejecución bajo falla sin restauración")
	}

	result.Events = append(result.Events, Event{
		Name: "restauracion_completada",
		At:   result.Started.Add(2 * time.Second),
	})
	if _, err := AggregateBenchmarkRuns([]ExperimentResult{result}); err != nil {
		t.Fatalf("se rechazó una ejecución restaurada: %v", err)
	}
}

type transientStatusBackend struct {
	fakeBackend
	calls int
}

func (b *transientStatusBackend) Statuses(ctx context.Context) ([]NodeStatus, error) {
	b.calls++
	nodes, err := b.fakeBackend.Statuses(ctx)
	if err != nil {
		return nil, err
	}
	if b.calls == 1 {
		return nodes[:2], nil
	}
	return nodes[:3], nil
}

func TestResolveFaultTargetRetriesWithoutQuorum(t *testing.T) {
	config, _ := LoadConfig(strings.NewReader(validScenario))
	config.Nodes = 5
	config.Fault.Target = "lider"
	b := &transientStatusBackend{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	target, term, err := resolveFaultTarget(ctx, config, b)
	if err != nil {
		t.Fatal(err)
	}
	if target != "nodo-2" || term != 1 || b.calls < 2 {
		t.Fatalf("objetivo inesperado: target=%s term=%d calls=%d", target, term, b.calls)
	}
}
