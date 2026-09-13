package experiment

import (
	"fmt"
	"math"
	"sort"
)

// DescriptiveStatistics resume una métrica observada a nivel de run.
// StdDev es nil cuando una única observación no permite calcular la desviación
// estándar muestral.
type DescriptiveStatistics struct {
	N      int
	Min    float64
	Q1     float64
	Median float64
	Q3     float64
	Max    float64
	IQR    float64
	MAD    float64
	Mean   float64
	StdDev *float64
}

// SummarizeDescriptive calcula estadísticos robustos sin modificar la muestra.
// Q1, mediana y Q3 usan cuantiles Hyndman-Fan type 7. MAD es la mediana de
// |xi - mediana| sin factor de escala. StdDev usa el denominador n-1.
func SummarizeDescriptive(values []float64) (DescriptiveStatistics, error) {
	var summary DescriptiveStatistics
	if len(values) == 0 {
		return summary, fmt.Errorf("se requiere al menos una observación")
	}

	ordered := append([]float64(nil), values...)
	for _, value := range ordered {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return summary, fmt.Errorf("la muestra contiene un valor no finito")
		}
	}
	sort.Float64s(ordered)

	q1 := quantileType7(ordered, 0.25)
	medianValue := quantileType7(ordered, 0.50)
	q3 := quantileType7(ordered, 0.75)

	deviations := make([]float64, len(ordered))
	for i, value := range ordered {
		deviations[i] = math.Abs(value - medianValue)
	}
	sort.Float64s(deviations)

	mean := 0.0
	m2 := 0.0
	for i, value := range ordered {
		delta := value - mean
		mean += delta / float64(i+1)
		m2 += delta * (value - mean)
	}

	summary = DescriptiveStatistics{
		N:      len(ordered),
		Min:    ordered[0],
		Q1:     q1,
		Median: medianValue,
		Q3:     q3,
		Max:    ordered[len(ordered)-1],
		IQR:    q3 - q1,
		MAD:    quantileType7(deviations, 0.50),
		Mean:   mean,
	}
	if len(ordered) > 1 {
		value := math.Sqrt(m2 / float64(len(ordered)-1))
		summary.StdDev = &value
	}
	return summary, nil
}

func quantileType7(ordered []float64, probability float64) float64 {
	if len(ordered) == 1 {
		return ordered[0]
	}
	position := float64(len(ordered)-1) * probability
	lower := int(math.Floor(position))
	upper := int(math.Ceil(position))
	if lower == upper {
		return ordered[lower]
	}
	fraction := position - float64(lower)
	return ordered[lower] + fraction*(ordered[upper]-ordered[lower])
}
