package experiment

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestCalculateMetrics(t *testing.T) {
	start := time.Unix(1000, 0)
	r := ExperimentResult{Started: start, Finished: start.Add(10 * time.Second)}
	for i := 100; i >= 1; i-- {
		r.Operations = append(r.Operations, OperationResult{Sequence: i, Outcome: "confirmada", Status: 200, Elapsed: time.Duration(i) * time.Millisecond})
	}
	r.Operations = append(r.Operations, OperationResult{Sequence: 101, Outcome: "incierta", Elapsed: 7 * time.Second}, OperationResult{Sequence: 102, Outcome: "omitida_saturacion"})
	m, err := CalculateMetrics(r)
	if err != nil {
		t.Fatal(err)
	}
	if m.Throughput == nil || *m.Throughput != 10 || *m.LatencyP50 != 50 || *m.LatencyP95 != 95 || *m.LatencyP99 != 99 || m.Uncertain != 1 || m.Omitted != 1 {
		t.Fatalf("métricas inesperadas: %+v", m)
	}
	if m.CommittedWritesLost != nil || m.DuplicateApplications != nil || m.RecoveryDurationMS != nil {
		t.Fatal("se inventaron mediciones sin evidencia")
	}
}

func TestMetricsWithoutConfirmations(t *testing.T) {
	start := time.Unix(1000, 0)
	r := ExperimentResult{Started: start, Finished: start.Add(time.Second)}
	m, err := CalculateMetrics(r)
	if err != nil || m.Throughput == nil || *m.Throughput != 0 || m.LatencyP50 != nil {
		t.Fatalf("métricas vacías incorrectas: %+v, %v", m, err)
	}
	data, _ := json.Marshal(m)
	if !strings.Contains(string(data), `"committed_writes_lost":null`) {
		t.Fatal("una medición ausente debe serializarse como null")
	}
	r.Error = "cancelado"
	m, err = CalculateMetrics(r)
	if err != nil || !m.Partial || m.Throughput != nil {
		t.Fatal("una ejecución incompleta no debe producir throughput de benchmark")
	}
}

func TestMetricsRejectInvalidRecords(t *testing.T) {
	start := time.Unix(1000, 0)
	for _, ops := range [][]OperationResult{
		{{Sequence: 1, Outcome: "confirmada", Status: 503}},
		{{Sequence: 1, Outcome: "incierta", Elapsed: -1}},
		{{Sequence: 1, Outcome: "desconocido"}},
		{{Sequence: 1, Outcome: "incierta"}, {Sequence: 1, Outcome: "incierta"}},
	} {
		if _, err := CalculateMetrics(ExperimentResult{Started: start, Finished: start.Add(time.Second), Operations: ops}); err == nil {
			t.Fatal("se aceptaron registros inválidos")
		}
	}
	if _, err := CalculateMetrics(ExperimentResult{}); err == nil {
		t.Fatal("se aceptó una ventana temporal inválida")
	}
}
