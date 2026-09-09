package experiment

import (
	"fmt"
	"sort"
)

type RepeatedBenchmarkSummary struct {
	Runs             int      `json:"ejecuciones"`
	ThroughputMedian *float64 `json:"throughput_mediana,omitempty"`
	LatencyP50Median *float64 `json:"latency_p50_mediana_ms,omitempty"`
	LatencyP95Median *float64 `json:"latency_p95_mediana_ms,omitempty"`
	LatencyP99Median *float64 `json:"latency_p99_mediana_ms,omitempty"`
	UncertainMedian  float64  `json:"inciertas_mediana"`
	OmittedMedian    float64  `json:"omitidas_mediana"`
	RestoredRuns     int      `json:"ejecuciones_restauradas"`
}

// AggregateBenchmarkRuns resume ejecuciones repetidas de una misma configuración.
// Las medianas se calculan sobre métricas por ejecución, no sobre operaciones mezcladas.
func AggregateBenchmarkRuns(results []ExperimentResult) (RepeatedBenchmarkSummary, error) {
	var summary RepeatedBenchmarkSummary
	if len(results) == 0 {
		return summary, fmt.Errorf("se requiere al menos una ejecución")
	}

	base := results[0].Config
	if err := base.Validate(); err != nil {
		return summary, err
	}

	var throughput, p50, p95, p99, uncertain, omitted []float64
	for i, result := range results {
		config := result.Config
		if err := config.Validate(); err != nil {
			return summary, fmt.Errorf("ejecución %d: %w", i+1, err)
		}
		if !sameBenchmarkConfig(base, config) {
			return summary, fmt.Errorf("las ejecuciones no pertenecen a la misma configuración")
		}
		if result.Error != "" {
			return summary, fmt.Errorf("la ejecución %d está incompleta: %s", i+1, result.Error)
		}
		if len(result.Operations) != config.Duration*config.Rate {
			return summary, fmt.Errorf("la ejecución %d no registra todas las operaciones programadas", i+1)
		}

		metrics, err := CalculateMetrics(result)
		if err != nil {
			return summary, fmt.Errorf("ejecución %d: %w", i+1, err)
		}
		appendMetric := func(values *[]float64, value *float64) {
			if value != nil {
				*values = append(*values, *value)
			}
		}
		appendMetric(&throughput, metrics.Throughput)
		appendMetric(&p50, metrics.LatencyP50)
		appendMetric(&p95, metrics.LatencyP95)
		appendMetric(&p99, metrics.LatencyP99)
		uncertain = append(uncertain, float64(metrics.Uncertain))
		omitted = append(omitted, float64(metrics.Omitted))

		for _, event := range result.Events {
			if event.Name == "restauracion_completada" {
				summary.RestoredRuns++
				break
			}
		}
	}

	if base.Fault.Type == "ninguna" && summary.RestoredRuns != 0 {
		return summary, fmt.Errorf("un benchmark sin fallas no debe registrar restauraciones")
	}
	if base.Fault.Type != "ninguna" && summary.RestoredRuns != len(results) {
		return summary, fmt.Errorf("no todas las ejecuciones bajo falla completaron la restauración")
	}

	summary.Runs = len(results)
	summary.ThroughputMedian = medianPtr(throughput)
	summary.LatencyP50Median = medianPtr(p50)
	summary.LatencyP95Median = medianPtr(p95)
	summary.LatencyP99Median = medianPtr(p99)
	summary.UncertainMedian = median(uncertain)
	summary.OmittedMedian = median(omitted)
	return summary, nil
}

func sameBenchmarkConfig(a, b ExperimentConfig) bool {
	return a.Nodes == b.Nodes &&
		a.Deployment == b.Deployment &&
		a.Version == b.Version &&
		a.Name == b.Name &&
		a.Seed == b.Seed &&
		a.Duration == b.Duration &&
		a.Clients == b.Clients &&
		a.Rate == b.Rate &&
		a.Fault == b.Fault
}

func medianPtr(values []float64) *float64 {
	if len(values) == 0 {
		return nil
	}
	value := median(values)
	return &value
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	middle := len(ordered) / 2
	if len(ordered)%2 == 1 {
		return ordered[middle]
	}
	return (ordered[middle-1] + ordered[middle]) / 2
}
