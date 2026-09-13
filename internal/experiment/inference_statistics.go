package experiment

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// PermutationConfig fija los parámetros de la prueba Monte Carlo sobre la
// diferencia de medianas entre dos grupos independientes.
type PermutationConfig struct {
	Replicas int
	Seed     int64
}

// PermutationResult conserva evidencia suficiente para auditar el valor p.
// PValue es nil cuando la muestra combinada no contiene variación informativa.
type PermutationResult struct {
	Tested       bool
	PValue       *float64
	ExtremeCount int
	Reason       string
}

// MedianPermutationTest contrasta bilateralmente la diferencia de medianas.
// Bajo H0, las etiquetas baseline/escenario se consideran intercambiables.
func MedianPermutationTest(baseline, scenario []float64, config PermutationConfig) (PermutationResult, error) {
	var result PermutationResult
	if config.Replicas < 1 {
		return result, fmt.Errorf("se requiere al menos una permutación")
	}
	baselineOrdered, err := orderedFiniteCopy(baseline)
	if err != nil {
		return result, fmt.Errorf("baseline: %w", err)
	}
	scenarioOrdered, err := orderedFiniteCopy(scenario)
	if err != nil {
		return result, fmt.Errorf("escenario: %w", err)
	}

	pooled := make([]float64, 0, len(baselineOrdered)+len(scenarioOrdered))
	pooled = append(pooled, baselineOrdered...)
	pooled = append(pooled, scenarioOrdered...)
	minValue, maxValue := pooled[0], pooled[0]
	for _, value := range pooled[1:] {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}
	if minValue == maxValue {
		result.Reason = "la muestra combinada es constante"
		return result, nil
	}

	observed := math.Abs(
		quantileType7(scenarioOrdered, 0.50) - quantileType7(baselineOrdered, 0.50),
	)
	rng := rand.New(rand.NewSource(config.Seed))
	permuted := append([]float64(nil), pooled...)
	baselineSample := make([]float64, len(baselineOrdered))
	scenarioSample := make([]float64, len(scenarioOrdered))
	extreme := 0
	for replica := 0; replica < config.Replicas; replica++ {
		rng.Shuffle(len(permuted), func(i, j int) {
			permuted[i], permuted[j] = permuted[j], permuted[i]
		})
		copy(baselineSample, permuted[:len(baselineSample)])
		copy(scenarioSample, permuted[len(baselineSample):])
		sort.Float64s(baselineSample)
		sort.Float64s(scenarioSample)
		candidate := math.Abs(
			quantileType7(scenarioSample, 0.50) - quantileType7(baselineSample, 0.50),
		)
		if candidate >= observed {
			extreme++
		}
	}
	p := float64(extreme+1) / float64(config.Replicas+1)
	result.Tested = true
	result.PValue = &p
	result.ExtremeCount = extreme
	return result, nil
}

// HolmAdjust aplica el procedimiento step-down de Holm solo a los valores p
// disponibles. Las posiciones nil se conservan como comparaciones no testeadas.
func HolmAdjust(pvalues []*float64) ([]*float64, error) {
	type item struct {
		index int
		value float64
	}
	items := make([]item, 0, len(pvalues))
	for index, value := range pvalues {
		if value == nil {
			continue
		}
		if math.IsNaN(*value) || math.IsInf(*value, 0) || *value < 0 || *value > 1 {
			return nil, fmt.Errorf("valor p inválido en la posición %d", index)
		}
		items = append(items, item{index: index, value: *value})
	}
	sort.SliceStable(items, func(i, j int) bool {
		return items[i].value < items[j].value
	})

	adjusted := make([]*float64, len(pvalues))
	running := 0.0
	familySize := len(items)
	for rank, current := range items {
		candidate := float64(familySize-rank) * current.value
		if candidate > 1 {
			candidate = 1
		}
		if candidate < running {
			candidate = running
		} else {
			running = candidate
		}
		value := candidate
		adjusted[current.index] = &value
	}
	return adjusted, nil
}
