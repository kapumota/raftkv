package main

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestGenerateFiguresProducesDeterministicArtifactSet(t *testing.T) {
	dir := t.TempDir()
	descriptive := filepath.Join(dir, "descriptive.csv")
	comparisons := filepath.Join(dir, "comparisons.csv")
	output := filepath.Join(dir, "figures")

	if err := os.WriteFile(descriptive, []byte(testDescriptiveCSV()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(comparisons, []byte(testComparisonsCSV()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generateFigures(descriptive, comparisons, output, defaultRevision); err != nil {
		t.Fatal(err)
	}

	names := []string{"throughput.svg", "latency-p95.svg", "latency-p99.svg", "uncertain-writes.svg", "cliffs-delta.svg", "figures-manifest.sha256"}
	first := map[string][]byte{}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if len(data) == 0 {
			t.Fatalf("%s está vacío", name)
		}
		first[name] = append([]byte(nil), data...)
	}
	if err := generateFigures(descriptive, comparisons, output, defaultRevision); err != nil {
		t.Fatal(err)
	}
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(output, name))
		if err != nil {
			t.Fatal(err)
		}
		if string(data) != string(first[name]) {
			t.Fatalf("%s no es determinista", name)
		}
	}
	if !strings.Contains(string(first["cliffs-delta.svg"]), "Holm ajustado") {
		t.Fatal("falta leyenda Holm")
	}
}

func TestGenerateFiguresRejectsIncompleteDescriptiveData(t *testing.T) {
	dir := t.TempDir()
	descriptive := filepath.Join(dir, "descriptive.csv")
	comparisons := filepath.Join(dir, "comparisons.csv")
	if err := os.WriteFile(descriptive, []byte("scenario,metric,unit,n,min,q1,median,q3,max,iqr,mad,mean,stddev\nsin fallas,throughput,writes/s,30,0,1,2,3,4,2,1,2,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(comparisons, []byte(testComparisonsCSV()), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := generateFigures(descriptive, comparisons, filepath.Join(dir, "out"), defaultRevision); err == nil {
		t.Fatal("se esperaba error")
	}
}

func testDescriptiveCSV() string {
	var b strings.Builder
	b.WriteString("scenario,metric,unit,n,min,q1,median,q3,max,iqr,mad,mean,stddev\n")
	metrics := []struct{ name, unit string }{{"throughput", "writes/s"}, {"latency_p50_ms", "ms"}, {"latency_p95_ms", "ms"}, {"latency_p99_ms", "ms"}, {"uncertain_writes", "count"}, {"omitted_operations", "count"}}
	for si, scenario := range scenarioOrder {
		for mi, metric := range metrics {
			base := float64((si+1)*10 + mi)
			b.WriteString(scenario + "," + metric.name + "," + metric.unit + ",30,0," +
				fmtFloat(base) + "," + fmtFloat(base+1) + "," + fmtFloat(base+2) + ",100,2,1,10,1\n")
		}
	}
	return b.String()
}

func testComparisonsCSV() string {
	var b strings.Builder
	b.WriteString("baseline,scenario,metric,unit,n_baseline,n_scenario,baseline_median,scenario_median,median_difference,median_difference_ci_low,median_difference_ci_high,relative_change_percent,cliffs_delta,cliffs_delta_ci_low,cliffs_delta_ci_high,bootstrap_replicas,confidence_level,analysis_seed,bootstrap_seed,tested,not_tested_reason,permutation_replicas,permutation_seed,permutation_extreme_count,permutation_p,holm_family_size,holm_adjusted_p,alpha\n")
	for si, scenario := range scenarioOrder[1:] {
		for mi, metric := range metricOrder {
			d := -0.6 + float64(si*6+mi)*0.06
			p := 1.0
			if mi == 0 {
				p = 0.01
			}
			b.WriteString("sin fallas," + scenario + "," + metric + ",x,30,30,0,0,0,-1,1,," + fmtFloat(d) + "," + fmtFloat(d-0.1) + "," + fmtFloat(d+0.1) + ",100,0.95,1,1,true,,100,1,1,0.1,18," + fmtFloat(p) + ",0.05\n")
		}
	}
	return b.String()
}

func fmtFloat(v float64) string { return strconv.FormatFloat(v, 'f', 6, 64) }
