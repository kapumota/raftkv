package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"testing"
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
