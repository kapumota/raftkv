package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/kapumota/raftkv/internal/experiment"
)

const finalRevision = "8ba7a131c4f452aac014a4c628794062fa1a8c9b"

type runFile struct {
	Revision string                      `json:"revision_git"`
	Changes  string                      `json:"cambios_git"`
	Result   experiment.ExperimentResult `json:"experimento"`
}

type scenarioSpec struct {
	Label   string
	Pattern string
	Type    string
	Target  string
}

type runRow struct {
	Scenario string
	Run      int
	Source   string
	Revision string
	Metrics  experiment.RunMetrics
}

type scenarioSummary struct {
	Label   string
	Summary experiment.RepeatedBenchmarkSummary
}

var scenarios = []scenarioSpec{
	{Label: "sin fallas", Pattern: "normal-faults-5-*.json", Type: "ninguna", Target: ""},
	{Label: "follower caído", Pattern: "follower-down-5-*.json", Type: "caida", Target: "seguidor"},
	{Label: "leader caído", Pattern: "leader-down-5-*.json", Type: "caida", Target: "lider"},
	{Label: "partición de leader", Pattern: "leader-partition-5-*.json", Type: "particion", Target: "lider"},
}

func loadRun(path string) (runFile, error) {
	var run runFile
	file, err := os.Open(path)
	if err != nil {
		return run, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&run); err != nil {
		return run, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return run, fmt.Errorf("%s: se requiere un único resultado JSON", path)
	}
	return run, nil
}

func sameWorkload(a, b experiment.ExperimentConfig) bool {
	return a.Nodes == b.Nodes &&
		a.Deployment == b.Deployment &&
		a.Version == b.Version &&
		a.Name == b.Name &&
		a.Seed == b.Seed &&
		a.Duration == b.Duration &&
		a.Clients == b.Clients &&
		a.Rate == b.Rate
}

func collectRuns(dir string, expected int, expectedRevision string) ([]runRow, []scenarioSummary, error) {
	var rows []runRow
	var summaries []scenarioSummary
	var baseline experiment.ExperimentConfig

	for scenarioIndex, spec := range scenarios {
		paths, err := filepath.Glob(filepath.Join(dir, spec.Pattern))
		if err != nil {
			return nil, nil, err
		}
		sort.Strings(paths)
		if len(paths) != expected {
			return nil, nil, fmt.Errorf("%s: se esperaban %d ejecuciones y se encontraron %d", spec.Label, expected, len(paths))
		}

		results := make([]experiment.ExperimentResult, 0, len(paths))
		for runIndex, path := range paths {
			run, err := loadRun(path)
			if err != nil {
				return nil, nil, err
			}
			if run.Revision != expectedRevision || run.Changes != "" {
				return nil, nil, fmt.Errorf("%s: se requiere revisión %s y árbol limpio", path, expectedRevision)
			}

			config := run.Result.Config
			if config.Nodes != 5 || config.Deployment != "benchmark" ||
				config.Name != "benchmark_fallas" ||
				config.Fault.Type != spec.Type || config.Fault.Target != spec.Target {
				return nil, nil, fmt.Errorf("%s: la configuración no corresponde al escenario %s", path, spec.Label)
			}
			if scenarioIndex == 0 && runIndex == 0 {
				baseline = config
			} else if !sameWorkload(baseline, config) {
				return nil, nil, fmt.Errorf("%s: la carga difiere del baseline", path)
			}

			metrics, err := experiment.ExtractRunMetrics(run.Result)
			if err != nil {
				return nil, nil, fmt.Errorf("%s: %w", path, err)
			}
			rows = append(rows, runRow{
				Scenario: spec.Label,
				Run:      runIndex + 1,
				Source:   filepath.Base(path),
				Revision: run.Revision,
				Metrics:  metrics,
			})
			results = append(results, run.Result)
		}

		summary, err := experiment.AggregateBenchmarkRuns(results)
		if err != nil {
			return nil, nil, fmt.Errorf("%s: %w", spec.Label, err)
		}
		summaries = append(summaries, scenarioSummary{Label: spec.Label, Summary: summary})
	}

	return rows, summaries, nil
}

func formatFloat(value *float64) string {
	if value == nil {
		return ""
	}
	return strconv.FormatFloat(*value, 'f', -1, 64)
}

func writeCSV(path string, rows []runRow) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	writeErr := writer.Write([]string{
		"scenario",
		"run",
		"source_file",
		"revision_git",
		"throughput",
		"latency_p50_ms",
		"latency_p95_ms",
		"latency_p99_ms",
		"confirmed_writes",
		"uncertain_writes",
		"rejected_writes",
		"omitted_operations",
		"measurement_window_seconds",
		"restored",
	})
	for _, row := range rows {
		if writeErr != nil {
			break
		}
		writeErr = writer.Write([]string{
			row.Scenario,
			strconv.Itoa(row.Run),
			row.Source,
			row.Revision,
			formatFloat(row.Metrics.Throughput),
			formatFloat(row.Metrics.LatencyP50),
			formatFloat(row.Metrics.LatencyP95),
			formatFloat(row.Metrics.LatencyP99),
			strconv.Itoa(row.Metrics.Confirmed),
			strconv.Itoa(row.Metrics.Uncertain),
			strconv.Itoa(row.Metrics.Rejected),
			strconv.Itoa(row.Metrics.Omitted),
			strconv.FormatFloat(row.Metrics.WindowSeconds, 'f', -1, 64),
			strconv.FormatBool(row.Metrics.Restored),
		})
	}
	writer.Flush()
	if writeErr == nil {
		writeErr = writer.Error()
	}
	closeErr := file.Close()
	if writeErr != nil {
		return writeErr
	}
	return closeErr
}

func summaryValue(value *float64) string {
	if value == nil {
		return "no disponible"
	}
	return fmt.Sprintf("%.3f", *value)
}

func printVerification(summaries []scenarioSummary) {
	fmt.Println("Escenario | Runs | Throughput | p50 ms | p95 ms | p99 ms | Inciertas | Omitidas | Restauradas")
	for _, item := range summaries {
		s := item.Summary
		fmt.Printf("%s | %d | %s | %s | %s | %s | %.3f | %.3f | %d/%d\n",
			item.Label,
			s.Runs,
			summaryValue(s.ThroughputMedian),
			summaryValue(s.LatencyP50Median),
			summaryValue(s.LatencyP95Median),
			summaryValue(s.LatencyP99Median),
			s.UncertainMedian,
			s.OmittedMedian,
			s.RestoredRuns,
			s.Runs,
		)
	}
}

func run() error {
	dir := flag.String("dir", "", "directorio con los JSON crudos de la campaña")
	output := flag.String("output", "", "archivo CSV nuevo para las métricas por run")
	runs := flag.Int("runs", 30, "número esperado de runs válidos por escenario")
	revision := flag.String("revision", finalRevision, "revisión Git experimental esperada")
	flag.Parse()

	if *dir == "" {
		return fmt.Errorf("se requiere -dir")
	}
	if *output == "" {
		return fmt.Errorf("se requiere -output")
	}
	if *runs < 1 {
		return fmt.Errorf("-runs debe ser mayor que cero")
	}
	if *revision == "" {
		return fmt.Errorf("-revision no puede estar vacía")
	}

	rows, summaries, err := collectRuns(*dir, *runs, *revision)
	if err != nil {
		return err
	}
	if err := writeCSV(*output, rows); err != nil {
		return fmt.Errorf("no se pudo escribir %s: %w", *output, err)
	}
	printVerification(summaries)
	fmt.Printf("Runs extraídos: %d\n", len(rows))
	fmt.Printf("CSV: %s\n", *output)
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
