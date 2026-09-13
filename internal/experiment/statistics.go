package experiment

import "fmt"

// RunMetrics contiene las métricas observadas de un único run válido.
// Los punteros conservan la distinción entre cero y una métrica no disponible.
type RunMetrics struct {
	Throughput    *float64
	LatencyP50    *float64
	LatencyP95    *float64
	LatencyP99    *float64
	Confirmed     int
	Uncertain     int
	Rejected      int
	Omitted       int
	WindowSeconds float64
	Restored      bool
}

// ExtractRunMetrics valida un run completo y reutiliza CalculateMetrics como
// fuente única para throughput, latencias y contadores experimentales.
func ExtractRunMetrics(result ExperimentResult) (RunMetrics, error) {
	var extracted RunMetrics
	if err := result.Config.Validate(); err != nil {
		return extracted, err
	}
	if result.Error != "" {
		return extracted, fmt.Errorf("la ejecución está incompleta: %s", result.Error)
	}
	if len(result.Operations) != result.Config.Duration*result.Config.Rate {
		return extracted, fmt.Errorf("la ejecución no registra todas las operaciones programadas")
	}

	metrics, err := CalculateMetrics(result)
	if err != nil {
		return extracted, err
	}
	for _, event := range result.Events {
		if event.Name == "restauracion_completada" {
			extracted.Restored = true
			break
		}
	}
	if result.Config.Fault.Type == "ninguna" && extracted.Restored {
		return RunMetrics{}, fmt.Errorf("un benchmark sin fallas no debe registrar restauraciones")
	}
	if result.Config.Fault.Type != "ninguna" && !extracted.Restored {
		return RunMetrics{}, fmt.Errorf("la ejecución bajo falla no completó la restauración")
	}

	extracted.Throughput = metrics.Throughput
	extracted.LatencyP50 = metrics.LatencyP50
	extracted.LatencyP95 = metrics.LatencyP95
	extracted.LatencyP99 = metrics.LatencyP99
	extracted.Confirmed = metrics.Confirmed
	extracted.Uncertain = metrics.Uncertain
	extracted.Rejected = metrics.Rejected
	extracted.Omitted = metrics.Omitted
	extracted.WindowSeconds = metrics.WindowSeconds
	return extracted, nil
}
