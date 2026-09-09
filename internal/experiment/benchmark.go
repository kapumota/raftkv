package experiment

import "fmt"

// CompareNormalRuns exige la misma carga y revisa la integridad temporal con
// CalculateMetrics. Una pareja de ejecuciones no determina significancia estadística.
func CompareNormalRuns(three, five ExperimentResult) ([2]Metrics, error) {
	var metrics [2]Metrics
	a, b := three.Config, five.Config
	if a.Nodes != 3 || b.Nodes != 5 {
		return metrics, fmt.Errorf("se requiere primero un resultado de tres nodos y luego uno de cinco")
	}
	for _, config := range []ExperimentConfig{a, b} {
		if err := config.Validate(); err != nil {
			return metrics, err
		}
		if config.Fault.Type != "ninguna" || config.Deployment != "benchmark" {
			return metrics, fmt.Errorf("se requieren benchmarks sin fallas")
		}
	}
	if a.Name != b.Name || a.Seed != b.Seed || a.Duration != b.Duration || a.Clients != b.Clients || a.Rate != b.Rate {
		return metrics, fmt.Errorf("las cargas no son comparables: difieren nombre, semilla, duración, clientes o tasa")
	}
	for i, r := range []ExperimentResult{three, five} {
		if r.Error != "" {
			return metrics, fmt.Errorf("no se comparan ejecuciones incompletas")
		}
		if len(r.Operations) != r.Config.Duration*r.Config.Rate {
			return metrics, fmt.Errorf("el resultado no registra todas las operaciones programadas")
		}
		for _, op := range r.Operations {
			if op.Sequence < 0 || op.Sequence >= len(r.Operations) || op.Client != op.Sequence%r.Config.Clients {
				return metrics, fmt.Errorf("secuencia o cliente fuera del plan")
			}
		}
		m, err := CalculateMetrics(r)
		if err != nil {
			return metrics, err
		}
		metrics[i] = m
	}
	return metrics, nil
}
