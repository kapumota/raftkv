package experiment

import (
	"fmt"
	"math"
	"sort"
)

// Metrics distingue ausencia de evidencia (null) de una medición igual a cero.
// Las latencias se expresan en milisegundos y throughput en escrituras/s.
type Metrics struct {
	Throughput            *float64          `json:"throughput"`
	LatencyP50            *float64          `json:"latency_p50"`
	LatencyP95            *float64          `json:"latency_p95"`
	LatencyP99            *float64          `json:"latency_p99"`
	ElectionDurationMS    *float64          `json:"election_duration_ms"`
	FailoverDurationMS    *float64          `json:"failover_duration_ms"`
	RecoveryDurationMS    *float64          `json:"recovery_duration_ms"`
	CommittedWritesLost   *int              `json:"committed_writes_lost"`
	DuplicateApplications *int              `json:"duplicate_applications"`
	Confirmed             int               `json:"confirmed_writes"`
	Uncertain             int               `json:"uncertain_writes"`
	Rejected              int               `json:"rejected_writes"`
	Omitted               int               `json:"omitted_operations"`
	WindowSeconds         float64           `json:"measurement_window_seconds"`
	Partial               bool              `json:"partial_run"`
	Unavailable           map[string]string `json:"no_disponible"`
}

// CalculateMetrics procesa resultados F2 sin inferir propiedades de seguridad
// a partir de errores HTTP, valores finales de SET o contadores del cliente.
func CalculateMetrics(result ExperimentResult) (Metrics, error) {
	m := Metrics{Partial: result.Error != "", Unavailable: map[string]string{
		"election_duration_ms":   "Falta observar el inicio y el final de la elección.",
		"failover_duration_ms":   "Faltan marcas temporales de las confirmaciones posteriores a la falla.",
		"recovery_duration_ms":   "Restaurar la red no demuestra que el nodo haya alcanzado el estado confirmado.",
		"committed_writes_lost":  "Falta verificar las escrituras confirmadas contra el estado recuperado.",
		"duplicate_applications": "Falta observar las aplicaciones dentro de la máquina de estados.",
	}}
	if result.Started.IsZero() || !result.Finished.After(result.Started) {
		return m, fmt.Errorf("el resultado no contiene una ventana temporal válida")
	}
	m.WindowSeconds = result.Finished.Sub(result.Started).Seconds()
	var latencies []float64
	seen := make(map[int]bool)
	for _, op := range result.Operations {
		if op.Sequence < 0 || seen[op.Sequence] || op.Elapsed < 0 {
			return m, fmt.Errorf("registro de operación inválido o repetido: %d", op.Sequence)
		}
		seen[op.Sequence] = true
		switch op.Outcome {
		case "confirmada":
			if op.Status != 200 {
				return m, fmt.Errorf("confirmación sin HTTP 200 en la operación %d", op.Sequence)
			}
			m.Confirmed++
			latencies = append(latencies, float64(op.Elapsed)/1e6)
		case "incierta":
			m.Uncertain++
		case "rechazada":
			m.Rejected++
		case "omitida_saturacion", "omitida_sin_lider":
			m.Omitted++
		default:
			return m, fmt.Errorf("resultado de operación desconocido: %q", op.Outcome)
		}
	}
	// La ventana F2 incluye el drenaje y cualquier limpieza posterior a la carga.
	if !m.Partial {
		value := float64(m.Confirmed) / m.WindowSeconds
		m.Throughput = &value
	} else {
		m.Unavailable["throughput"] = "Ejecución interrumpida o fallida; no se agrega como benchmark completo."
	}
	sort.Float64s(latencies)
	m.LatencyP50 = nearestRank(latencies, .50)
	m.LatencyP95 = nearestRank(latencies, .95)
	m.LatencyP99 = nearestRank(latencies, .99)
	if len(latencies) == 0 {
		for _, key := range []string{"latency_p50", "latency_p95", "latency_p99"} {
			m.Unavailable[key] = "No hubo escrituras confirmadas."
		}
	}
	return m, nil
}

func nearestRank(sorted []float64, quantile float64) *float64 {
	if len(sorted) == 0 {
		return nil
	}
	index := int(math.Ceil(quantile*float64(len(sorted)))) - 1
	value := sorted[index]
	return &value
}
