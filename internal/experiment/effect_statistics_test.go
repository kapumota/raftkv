package experiment

import (
	"math"
	"reflect"
	"testing"
)

func TestEstimateBootstrapEffectsDegenerateSamples(t *testing.T) {
	config := BootstrapConfig{Replicas: 1000, Confidence: 0.95, Seed: 7}
	estimate, err := EstimateBootstrapEffects([]float64{1, 1, 1}, []float64{2, 2, 2}, config)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.BaselineN != 3 || estimate.ScenarioN != 3 ||
		estimate.BaselineMedian != 1 || estimate.ScenarioMedian != 2 ||
		estimate.MedianDifference != 1 || estimate.MedianDifferenceCILow != 1 || estimate.MedianDifferenceCIHigh != 1 ||
		estimate.CliffsDelta != 1 || estimate.CliffsDeltaCILow != 1 || estimate.CliffsDeltaCIHigh != 1 {
		t.Fatalf("estimación inesperada: %+v", estimate)
	}
	if estimate.RelativeChangePercent == nil || *estimate.RelativeChangePercent != 100 {
		t.Fatalf("cambio relativo inesperado: %+v", estimate.RelativeChangePercent)
	}
}

func TestEstimateBootstrapEffectsHandlesZeroBaselineAndTies(t *testing.T) {
	config := BootstrapConfig{Replicas: 100, Confidence: 0.95, Seed: 11}
	estimate, err := EstimateBootstrapEffects([]float64{0, 0, 0}, []float64{0, 0, 0}, config)
	if err != nil {
		t.Fatal(err)
	}
	if estimate.RelativeChangePercent != nil || estimate.MedianDifference != 0 ||
		estimate.MedianDifferenceCILow != 0 || estimate.MedianDifferenceCIHigh != 0 ||
		estimate.CliffsDelta != 0 || estimate.CliffsDeltaCILow != 0 || estimate.CliffsDeltaCIHigh != 0 {
		t.Fatalf("estimación inesperada con baseline cero: %+v", estimate)
	}

	estimate, err = EstimateBootstrapEffects([]float64{1, 2, 3}, []float64{2, 3, 4}, config)
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(estimate.CliffsDelta-5.0/9.0) > 1e-12 {
		t.Fatalf("Cliff's delta inesperado: %v", estimate.CliffsDelta)
	}
}

func TestEstimateBootstrapEffectsIsDeterministicAndDoesNotMutateSamples(t *testing.T) {
	baseline := []float64{5, 1, 4, 2, 3}
	scenario := []float64{6, 2, 5, 3, 4}
	baselineOriginal := append([]float64(nil), baseline...)
	scenarioOriginal := append([]float64(nil), scenario...)
	config := BootstrapConfig{Replicas: 500, Confidence: 0.95, Seed: 1234}

	first, err := EstimateBootstrapEffects(baseline, scenario, config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := EstimateBootstrapEffects(baseline, scenario, config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("el bootstrap no fue determinista: first=%+v second=%+v", first, second)
	}
	if !reflect.DeepEqual(baseline, baselineOriginal) || !reflect.DeepEqual(scenario, scenarioOriginal) {
		t.Fatal("EstimateBootstrapEffects modificó las muestras originales")
	}
}

func TestEstimateBootstrapEffectsRejectsInvalidInputs(t *testing.T) {
	valid := BootstrapConfig{Replicas: 10, Confidence: 0.95, Seed: 1}
	cases := []struct {
		baseline []float64
		scenario []float64
		config   BootstrapConfig
	}{
		{nil, []float64{1}, valid},
		{[]float64{1}, nil, valid},
		{[]float64{math.NaN()}, []float64{1}, valid},
		{[]float64{1}, []float64{math.Inf(1)}, valid},
		{[]float64{1}, []float64{2}, BootstrapConfig{Replicas: 0, Confidence: 0.95}},
		{[]float64{1}, []float64{2}, BootstrapConfig{Replicas: 10, Confidence: 0}},
		{[]float64{1}, []float64{2}, BootstrapConfig{Replicas: 10, Confidence: 1}},
	}
	for _, tc := range cases {
		if _, err := EstimateBootstrapEffects(tc.baseline, tc.scenario, tc.config); err == nil {
			t.Fatalf("se aceptó una entrada inválida: %+v", tc)
		}
	}
}

func TestDeriveAnalysisSeedIsStablePerComparison(t *testing.T) {
	first := DeriveAnalysisSeed(20260912, "leader caído", "throughput")
	second := DeriveAnalysisSeed(20260912, "leader caído", "throughput")
	other := DeriveAnalysisSeed(20260912, "leader caído", "latency_p99_ms")
	if first != second || first == other || first < 0 || other < 0 {
		t.Fatalf("semillas derivadas inesperadas: %d %d %d", first, second, other)
	}
}
