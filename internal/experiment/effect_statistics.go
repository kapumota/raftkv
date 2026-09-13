package experiment

import (
	"fmt"
	"hash/fnv"
	"math"
	"math/rand"
	"sort"
	"strconv"
)

// BootstrapConfig fija los parámetros de remuestreo usados para estimar
// incertidumbre en comparaciones entre runs independientes.
type BootstrapConfig struct {
	Replicas   int
	Confidence float64
	Seed       int64
}

// EffectEstimate resume una comparación orientada como escenario - baseline.
type EffectEstimate struct {
	BaselineN              int
	ScenarioN              int
	BaselineMedian         float64
	ScenarioMedian         float64
	MedianDifference       float64
	MedianDifferenceCILow  float64
	MedianDifferenceCIHigh float64
	RelativeChangePercent  *float64
	CliffsDelta            float64
	CliffsDeltaCILow       float64
	CliffsDeltaCIHigh      float64
}

// DeriveAnalysisSeed genera una semilla estable para una comparación concreta.
// Esto evita que agregar o reordenar métricas altere las corrientes aleatorias
// de comparaciones ya existentes.
func DeriveAnalysisSeed(master int64, labels ...string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte(strconv.FormatInt(master, 10)))
	_, _ = h.Write([]byte{0})
	for _, label := range labels {
		_, _ = h.Write([]byte(label))
		_, _ = h.Write([]byte{0})
	}
	return int64(h.Sum64() & math.MaxInt64)
}

// EstimateBootstrapEffects calcula diferencia de medianas, cambio relativo y
// Cliff's delta. Los intervalos usan bootstrap percentil y remuestreo
// independiente dentro de cada grupo.
func EstimateBootstrapEffects(baseline, scenario []float64, config BootstrapConfig) (EffectEstimate, error) {
	var estimate EffectEstimate
	if config.Replicas < 1 {
		return estimate, fmt.Errorf("se requiere al menos una réplica bootstrap")
	}
	if config.Confidence <= 0 || config.Confidence >= 1 || math.IsNaN(config.Confidence) {
		return estimate, fmt.Errorf("el nivel de confianza debe estar entre cero y uno")
	}

	baselineOrdered, err := orderedFiniteCopy(baseline)
	if err != nil {
		return estimate, fmt.Errorf("baseline: %w", err)
	}
	scenarioOrdered, err := orderedFiniteCopy(scenario)
	if err != nil {
		return estimate, fmt.Errorf("escenario: %w", err)
	}

	baselineMedian := quantileType7(baselineOrdered, 0.50)
	scenarioMedian := quantileType7(scenarioOrdered, 0.50)
	estimate = EffectEstimate{
		BaselineN:        len(baselineOrdered),
		ScenarioN:        len(scenarioOrdered),
		BaselineMedian:   baselineMedian,
		ScenarioMedian:   scenarioMedian,
		MedianDifference: scenarioMedian - baselineMedian,
		CliffsDelta:      cliffsDeltaSorted(baselineOrdered, scenarioOrdered),
	}
	if baselineMedian != 0 {
		value := 100 * (scenarioMedian/baselineMedian - 1)
		estimate.RelativeChangePercent = &value
	}

	rng := rand.New(rand.NewSource(config.Seed))
	baselineSample := make([]float64, len(baselineOrdered))
	scenarioSample := make([]float64, len(scenarioOrdered))
	differences := make([]float64, config.Replicas)
	deltas := make([]float64, config.Replicas)
	for replica := 0; replica < config.Replicas; replica++ {
		for i := range baselineSample {
			baselineSample[i] = baselineOrdered[rng.Intn(len(baselineOrdered))]
		}
		for i := range scenarioSample {
			scenarioSample[i] = scenarioOrdered[rng.Intn(len(scenarioOrdered))]
		}
		sort.Float64s(baselineSample)
		sort.Float64s(scenarioSample)
		differences[replica] = quantileType7(scenarioSample, 0.50) - quantileType7(baselineSample, 0.50)
		deltas[replica] = cliffsDeltaSorted(baselineSample, scenarioSample)
	}
	sort.Float64s(differences)
	sort.Float64s(deltas)
	alpha := 1 - config.Confidence
	estimate.MedianDifferenceCILow = quantileType7(differences, alpha/2)
	estimate.MedianDifferenceCIHigh = quantileType7(differences, 1-alpha/2)
	estimate.CliffsDeltaCILow = quantileType7(deltas, alpha/2)
	estimate.CliffsDeltaCIHigh = quantileType7(deltas, 1-alpha/2)
	return estimate, nil
}

func orderedFiniteCopy(values []float64) ([]float64, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("se requiere al menos una observación")
	}
	ordered := append([]float64(nil), values...)
	for _, value := range ordered {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return nil, fmt.Errorf("la muestra contiene un valor no finito")
		}
	}
	sort.Float64s(ordered)
	return ordered, nil
}

// cliffsDeltaSorted calcula P(escenario > baseline) - P(escenario < baseline).
// Los empates no se cuentan como victorias ni derrotas.
func cliffsDeltaSorted(baseline, scenario []float64) float64 {
	var wins, losses int64
	for _, value := range scenario {
		lower := sort.Search(len(baseline), func(i int) bool { return baseline[i] >= value })
		upper := sort.Search(len(baseline), func(i int) bool { return baseline[i] > value })
		wins += int64(lower)
		losses += int64(len(baseline) - upper)
	}
	return float64(wins-losses) / float64(len(baseline)*len(scenario))
}
