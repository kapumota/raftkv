package experiment

import (
	"math"
	"reflect"
	"testing"
)

func TestMedianPermutationTestSkipsConstantCombinedSample(t *testing.T) {
	result, err := MedianPermutationTest(
		[]float64{0, 0, 0},
		[]float64{0, 0, 0},
		PermutationConfig{Replicas: 100, Seed: 7},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Tested || result.PValue != nil || result.ExtremeCount != 0 || result.Reason == "" {
		t.Fatalf("resultado inesperado: %+v", result)
	}
}

func TestMedianPermutationTestDetectsSeparatedSamples(t *testing.T) {
	result, err := MedianPermutationTest(
		[]float64{0, 1, 2, 3, 4},
		[]float64{10, 11, 12, 13, 14},
		PermutationConfig{Replicas: 5000, Seed: 11},
	)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Tested || result.PValue == nil || *result.PValue <= 0 || *result.PValue >= 0.05 {
		t.Fatalf("valor p inesperado: %+v", result)
	}
	want := float64(result.ExtremeCount+1) / 5001
	if *result.PValue != want {
		t.Fatalf("no se aplicó la corrección Monte Carlo: got=%v want=%v", *result.PValue, want)
	}
}

func TestMedianPermutationTestIsDeterministicAndDoesNotMutateSamples(t *testing.T) {
	baseline := []float64{5, 1, 4, 2, 3}
	scenario := []float64{7, 3, 6, 4, 5}
	baselineOriginal := append([]float64(nil), baseline...)
	scenarioOriginal := append([]float64(nil), scenario...)
	config := PermutationConfig{Replicas: 1000, Seed: 1234}

	first, err := MedianPermutationTest(baseline, scenario, config)
	if err != nil {
		t.Fatal(err)
	}
	second, err := MedianPermutationTest(baseline, scenario, config)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("la permutación no fue determinista: first=%+v second=%+v", first, second)
	}
	if !reflect.DeepEqual(baseline, baselineOriginal) || !reflect.DeepEqual(scenario, scenarioOriginal) {
		t.Fatal("MedianPermutationTest modificó las muestras originales")
	}
}

func TestMedianPermutationTestRejectsInvalidInputs(t *testing.T) {
	valid := PermutationConfig{Replicas: 10, Seed: 1}
	cases := []struct {
		baseline []float64
		scenario []float64
		config   PermutationConfig
	}{
		{nil, []float64{1}, valid},
		{[]float64{1}, nil, valid},
		{[]float64{math.NaN()}, []float64{1}, valid},
		{[]float64{1}, []float64{math.Inf(1)}, valid},
		{[]float64{1}, []float64{2}, PermutationConfig{Replicas: 0}},
	}
	for _, tc := range cases {
		if _, err := MedianPermutationTest(tc.baseline, tc.scenario, tc.config); err == nil {
			t.Fatalf("se aceptó una entrada inválida: %+v", tc)
		}
	}
}

func TestHolmAdjustUsesOnlyTestedComparisons(t *testing.T) {
	p1, p2, p3 := 0.01, 0.04, 0.03
	adjusted, err := HolmAdjust([]*float64{&p1, &p2, &p3, nil})
	if err != nil {
		t.Fatal(err)
	}
	want := []*float64{floatPtr(0.03), floatPtr(0.06), floatPtr(0.06), nil}
	for i := range want {
		if want[i] == nil {
			if adjusted[i] != nil {
				t.Fatalf("posición %d debería permanecer sin prueba", i)
			}
			continue
		}
		if adjusted[i] == nil || math.Abs(*adjusted[i]-*want[i]) > 1e-12 {
			t.Fatalf("ajuste Holm inesperado en %d: got=%v want=%v", i, adjusted[i], *want[i])
		}
	}
}

func TestHolmAdjustRejectsInvalidPValues(t *testing.T) {
	for _, invalid := range []float64{-0.1, 1.1, math.NaN(), math.Inf(1)} {
		value := invalid
		if _, err := HolmAdjust([]*float64{&value}); err == nil {
			t.Fatalf("se aceptó valor p inválido: %v", invalid)
		}
	}
}

func floatPtr(value float64) *float64 {
	return &value
}
