package experiment

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func statisticalConfig(t *testing.T, fault string) ExperimentConfig {
	t.Helper()
	target, at, duration := "", 0, 0
	if fault != "ninguna" {
		target, at, duration = "lider", 1, 1
	}
	scenario := fmt.Sprintf(`version: 1
nombre: benchmark_fallas
semilla: 42
nodos: 5
despliegue: benchmark
duracion_segundos: 3
clientes: 2
operaciones_por_segundo: 1
falla:
  tipo: %s
  objetivo: %q
  instante_segundos: %d
  duracion_segundos: %d
`, fault, target, at, duration)
	config, err := LoadConfig(strings.NewReader(scenario))
	if err != nil {
		t.Fatal(err)
	}
	return config
}

func TestExtractRunMetricsUsesCanonicalMetrics(t *testing.T) {
	config := statisticalConfig(t, "ninguna")
	result := benchmarkResult(t, config, 2)

	metrics, err := ExtractRunMetrics(result)
	if err != nil {
		t.Fatal(err)
	}
	if metrics.Throughput == nil || *metrics.Throughput != 2.0/3.0 {
		t.Fatalf("throughput inesperado: %+v", metrics.Throughput)
	}
	if metrics.LatencyP50 == nil || *metrics.LatencyP50 != 1 ||
		metrics.LatencyP95 == nil || *metrics.LatencyP95 != 2 {
		t.Fatalf("latencias inesperadas: %+v", metrics)
	}
	if metrics.Confirmed != 2 || metrics.Uncertain != 1 || metrics.Restored {
		t.Fatalf("contadores inesperados: %+v", metrics)
	}
}

func TestExtractRunMetricsRequiresCompleteRun(t *testing.T) {
	config := statisticalConfig(t, "ninguna")
	result := benchmarkResult(t, config, 2)
	result.Operations = result.Operations[:2]
	if _, err := ExtractRunMetrics(result); err == nil {
		t.Fatal("se aceptó un run con operaciones faltantes")
	}

	result = benchmarkResult(t, config, 2)
	result.Error = "interrumpido"
	if _, err := ExtractRunMetrics(result); err == nil {
		t.Fatal("se aceptó un run incompleto")
	}
}

func TestExtractRunMetricsRequiresRecovery(t *testing.T) {
	config := statisticalConfig(t, "caida")
	result := benchmarkResult(t, config, 2)
	if _, err := ExtractRunMetrics(result); err == nil {
		t.Fatal("se aceptó un run bajo falla sin restauración")
	}

	result.Events = append(result.Events, Event{
		Name: "restauracion_completada",
		At:   result.Started.Add(2 * time.Second),
	})
	metrics, err := ExtractRunMetrics(result)
	if err != nil || !metrics.Restored {
		t.Fatalf("se rechazó un run restaurado: %+v, %v", metrics, err)
	}
}
