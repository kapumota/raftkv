package experiment

import (
	"math"
	"testing"
)

func TestSummarizeDescriptiveUsesType7AndRawMAD(t *testing.T) {
	values := []float64{8, 1, 7, 2, 6, 3, 5, 4}
	original := append([]float64(nil), values...)

	summary, err := SummarizeDescriptive(values)
	if err != nil {
		t.Fatal(err)
	}
	if summary.N != 8 || summary.Min != 1 || summary.Q1 != 2.75 ||
		summary.Median != 4.5 || summary.Q3 != 6.25 || summary.Max != 8 ||
		summary.IQR != 3.5 || summary.MAD != 2 || summary.Mean != 4.5 {
		t.Fatalf("resumen inesperado: %+v", summary)
	}
	if summary.StdDev == nil || math.Abs(*summary.StdDev-math.Sqrt(6)) > 1e-12 {
		t.Fatalf("desviación estándar muestral inesperada: %+v", summary.StdDev)
	}
	for i := range values {
		if values[i] != original[i] {
			t.Fatal("SummarizeDescriptive modificó la muestra original")
		}
	}
}

func TestSummarizeDescriptiveSingleObservation(t *testing.T) {
	summary, err := SummarizeDescriptive([]float64{7.5})
	if err != nil {
		t.Fatal(err)
	}
	if summary.N != 1 || summary.Min != 7.5 || summary.Q1 != 7.5 ||
		summary.Median != 7.5 || summary.Q3 != 7.5 || summary.Max != 7.5 ||
		summary.IQR != 0 || summary.MAD != 0 || summary.Mean != 7.5 {
		t.Fatalf("resumen inesperado: %+v", summary)
	}
	if summary.StdDev != nil {
		t.Fatal("una sola observación no debe producir desviación estándar muestral")
	}
}

func TestSummarizeDescriptiveRejectsInvalidSamples(t *testing.T) {
	for _, values := range [][]float64{
		nil,
		{},
		{1, math.NaN()},
		{1, math.Inf(1)},
		{1, math.Inf(-1)},
	} {
		if _, err := SummarizeDescriptive(values); err == nil {
			t.Fatalf("se aceptó una muestra inválida: %v", values)
		}
	}
}
