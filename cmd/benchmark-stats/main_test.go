package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kapumota/raftkv/internal/experiment"
)

func TestFinalCampaignExtractionReproducesG4(t *testing.T) {
	dir := filepath.Join("..", "..", "experiments", "raw", finalRevision, "final")
	rows, summaries, err := collectRuns(dir, 30, finalRevision)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 120 {
		t.Fatalf("se esperaban 120 runs y se extrajeron %d", len(rows))
	}

	expected := map[string][]string{
		"sin fallas":          {"1.983", "418.087", "1903.720", "2692.064", "0.000", "0.000", "0/30"},
		"follower caído":      {"1.972", "423.315", "1872.698", "2564.284", "0.000", "0.000", "30/30"},
		"leader caído":        {"1.808", "410.819", "2697.890", "4246.179", "11.000", "0.000", "30/30"},
		"partición de leader": {"1.817", "391.942", "1951.367", "3026.359", "10.000", "0.000", "30/30"},
	}
	if len(summaries) != len(expected) {
		t.Fatalf("se esperaban %d escenarios y se obtuvieron %d", len(expected), len(summaries))
	}
	for _, item := range summaries {
		s := item.Summary
		got := []string{
			summaryValue(s.ThroughputMedian),
			summaryValue(s.LatencyP50Median),
			summaryValue(s.LatencyP95Median),
			summaryValue(s.LatencyP99Median),
			fmt.Sprintf("%.3f", s.UncertainMedian),
			fmt.Sprintf("%.3f", s.OmittedMedian),
			fmt.Sprintf("%d/%d", s.RestoredRuns, s.Runs),
		}
		want, ok := expected[item.Label]
		if !ok {
			t.Fatalf("escenario inesperado: %s", item.Label)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: valor %d inesperado: got=%s want=%s", item.Label, i, got[i], want[i])
			}
		}
	}
}

func TestWriteCSVProducesOneRowPerRun(t *testing.T) {
	dir := filepath.Join("..", "..", "experiments", "raw", finalRevision, "final")
	rows, _, err := collectRuns(dir, 30, finalRevision)
	if err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "per-run.csv")
	if err := writeCSV(output, rows); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 121 {
		t.Fatalf("se esperaban cabecera + 120 filas y se obtuvieron %d registros", len(records))
	}
	if records[0][0] != "scenario" || records[0][4] != "throughput" || records[0][13] != "restored" {
		t.Fatalf("cabecera CSV inesperada: %v", records[0])
	}
	if records[1][0] != "sin fallas" || records[1][2] != "normal-faults-5-001.json" {
		t.Fatalf("primera fila inesperada: %v", records[1])
	}
	if err := writeCSV(output, rows); err == nil {
		t.Fatal("se sobrescribió un CSV existente")
	}
}
func TestBuildDescriptiveRowsReproducesG4Medians(t *testing.T) {
	dir := filepath.Join("..", "..", "experiments", "raw", finalRevision, "final")
	runs, _, err := collectRuns(dir, 30, finalRevision)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := buildDescriptiveRows(runs)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 24 {
		t.Fatalf("se esperaban 24 filas descriptivas y se obtuvieron %d", len(rows))
	}

	expected := map[string]map[string]string{
		"sin fallas": {
			"throughput": "1.983", "latency_p50_ms": "418.087", "latency_p95_ms": "1903.720",
			"latency_p99_ms": "2692.064", "uncertain_writes": "0.000", "omitted_operations": "0.000",
		},
		"follower caído": {
			"throughput": "1.972", "latency_p50_ms": "423.315", "latency_p95_ms": "1872.698",
			"latency_p99_ms": "2564.284", "uncertain_writes": "0.000", "omitted_operations": "0.000",
		},
		"leader caído": {
			"throughput": "1.808", "latency_p50_ms": "410.819", "latency_p95_ms": "2697.890",
			"latency_p99_ms": "4246.179", "uncertain_writes": "11.000", "omitted_operations": "0.000",
		},
		"partición de leader": {
			"throughput": "1.817", "latency_p50_ms": "391.942", "latency_p95_ms": "1951.367",
			"latency_p99_ms": "3026.359", "uncertain_writes": "10.000", "omitted_operations": "0.000",
		},
	}
	for _, row := range rows {
		if row.Summary.N != 30 {
			t.Fatalf("%s/%s: n inesperado: %d", row.Scenario, row.Metric, row.Summary.N)
		}
		want, ok := expected[row.Scenario][row.Metric]
		if !ok {
			t.Fatalf("fila descriptiva inesperada: %s/%s", row.Scenario, row.Metric)
		}
		got := fmt.Sprintf("%.3f", row.Summary.Median)
		if got != want {
			t.Fatalf("%s/%s: mediana inesperada: got=%s want=%s", row.Scenario, row.Metric, got, want)
		}
	}
}

func TestWriteDescriptiveCSVProducesStableSchema(t *testing.T) {
	dir := filepath.Join("..", "..", "experiments", "raw", finalRevision, "final")
	runs, _, err := collectRuns(dir, 30, finalRevision)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := buildDescriptiveRows(runs)
	if err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "descriptive.csv")
	if err := writeDescriptiveCSV(output, rows); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 25 {
		t.Fatalf("se esperaban cabecera + 24 filas y se obtuvieron %d registros", len(records))
	}
	wantHeader := []string{"scenario", "metric", "unit", "n", "min", "q1", "median", "q3", "max", "iqr", "mad", "mean", "stddev"}
	for i := range wantHeader {
		if records[0][i] != wantHeader[i] {
			t.Fatalf("cabecera descriptiva inesperada: %v", records[0])
		}
	}
	if records[1][0] != "sin fallas" || records[1][1] != "throughput" || records[1][3] != "30" {
		t.Fatalf("primera fila descriptiva inesperada: %v", records[1])
	}
	if err := writeDescriptiveCSV(output, rows); err == nil {
		t.Fatal("se sobrescribió un CSV descriptivo existente")
	}
}

func TestBuildEffectRowsUsesBaselineAndStablePointEstimates(t *testing.T) {
	dir := filepath.Join("..", "..", "experiments", "raw", finalRevision, "final")
	runs, _, err := collectRuns(dir, 30, finalRevision)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := buildEffectRows(runs, 200, 0.95, 20260912)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 18 {
		t.Fatalf("se esperaban 18 comparaciones y se obtuvieron %d", len(rows))
	}

	expectedThroughputMedian := map[string]float64{
		"follower caído":      1.971589773172238,
		"leader caído":        1.8083056893217422,
		"partición de leader": 1.816650967595013,
	}
	const baselineMedian = 1.9833165015409095
	seenThroughput := 0
	for _, row := range rows {
		if row.Baseline != "sin fallas" || row.Estimate.BaselineN != 30 || row.Estimate.ScenarioN != 30 {
			t.Fatalf("comparación inesperada: %+v", row)
		}
		if row.Metric != "throughput" {
			continue
		}
		seenThroughput++
		want, ok := expectedThroughputMedian[row.Scenario]
		if !ok {
			t.Fatalf("escenario inesperado para throughput: %s", row.Scenario)
		}
		if row.Estimate.BaselineMedian != baselineMedian || row.Estimate.ScenarioMedian != want {
			t.Fatalf("medianas inesperadas para %s: %+v", row.Scenario, row.Estimate)
		}
		if row.Estimate.RelativeChangePercent == nil {
			t.Fatalf("%s: falta cambio relativo de throughput", row.Scenario)
		}
		if row.BootstrapSeed != experiment.DeriveAnalysisSeed(20260912, row.Scenario, row.Metric) {
			t.Fatalf("%s: semilla bootstrap inestable", row.Scenario)
		}
	}
	if seenThroughput != 3 {
		t.Fatalf("se esperaban tres comparaciones de throughput y se observaron %d", seenThroughput)
	}
}

func TestWriteEffectsCSVProducesStableSchema(t *testing.T) {
	dir := filepath.Join("..", "..", "experiments", "raw", finalRevision, "final")
	runs, _, err := collectRuns(dir, 30, finalRevision)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := buildEffectRows(runs, 50, 0.95, 20260912)
	if err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(t.TempDir(), "effects.csv")
	if err := writeEffectsCSV(output, rows); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(output)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	records, err := csv.NewReader(file).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 19 {
		t.Fatalf("se esperaban cabecera + 18 filas y se obtuvieron %d registros", len(records))
	}
	wantHeader := []string{
		"baseline", "scenario", "metric", "unit", "n_baseline", "n_scenario",
		"baseline_median", "scenario_median", "median_difference",
		"median_difference_ci_low", "median_difference_ci_high", "relative_change_percent",
		"cliffs_delta", "cliffs_delta_ci_low", "cliffs_delta_ci_high",
		"bootstrap_replicas", "confidence_level", "analysis_seed", "bootstrap_seed",
	}
	for i := range wantHeader {
		if records[0][i] != wantHeader[i] {
			t.Fatalf("cabecera de efectos inesperada: %v", records[0])
		}
	}
	if records[1][0] != "sin fallas" || records[1][1] != "follower caído" || records[1][2] != "throughput" || records[1][15] != "50" {
		t.Fatalf("primera fila de efectos inesperada: %v", records[1])
	}
	if err := writeEffectsCSV(output, rows); err == nil {
		t.Fatal("se sobrescribió un CSV de efectos existente")
	}
}
