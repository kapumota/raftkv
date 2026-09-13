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

type descriptiveRow struct {
	Scenario string
	Metric   string
	Unit     string
	Summary  experiment.DescriptiveStatistics
}

type descriptiveMetric struct {
	Name  string
	Unit  string
	Value func(runRow) (float64, bool)
}

type effectRow struct {
	Baseline      string
	Scenario      string
	Metric        string
	Unit          string
	AnalysisSeed  int64
	BootstrapSeed int64
	Replicas      int
	Confidence    float64
	Estimate      experiment.EffectEstimate
}

var scenarios = []scenarioSpec{
	{Label: "sin fallas", Pattern: "normal-faults-5-*.json", Type: "ninguna", Target: ""},
	{Label: "follower caído", Pattern: "follower-down-5-*.json", Type: "caida", Target: "seguidor"},
	{Label: "leader caído", Pattern: "leader-down-5-*.json", Type: "caida", Target: "lider"},
	{Label: "partición de leader", Pattern: "leader-partition-5-*.json", Type: "particion", Target: "lider"},
}

var descriptiveMetrics = []descriptiveMetric{
	{Name: "throughput", Unit: "writes/s", Value: func(row runRow) (float64, bool) {
		if row.Metrics.Throughput == nil {
			return 0, false
		}
		return *row.Metrics.Throughput, true
	}},
	{Name: "latency_p50_ms", Unit: "ms", Value: func(row runRow) (float64, bool) {
		if row.Metrics.LatencyP50 == nil {
			return 0, false
		}
		return *row.Metrics.LatencyP50, true
	}},
	{Name: "latency_p95_ms", Unit: "ms", Value: func(row runRow) (float64, bool) {
		if row.Metrics.LatencyP95 == nil {
			return 0, false
		}
		return *row.Metrics.LatencyP95, true
	}},
	{Name: "latency_p99_ms", Unit: "ms", Value: func(row runRow) (float64, bool) {
		if row.Metrics.LatencyP99 == nil {
			return 0, false
		}
		return *row.Metrics.LatencyP99, true
	}},
	{Name: "uncertain_writes", Unit: "count", Value: func(row runRow) (float64, bool) {
		return float64(row.Metrics.Uncertain), true
	}},
	{Name: "omitted_operations", Unit: "count", Value: func(row runRow) (float64, bool) {
		return float64(row.Metrics.Omitted), true
	}},
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

func buildDescriptiveRows(rows []runRow) ([]descriptiveRow, error) {
	result := make([]descriptiveRow, 0, len(scenarios)*len(descriptiveMetrics))
	for _, scenario := range scenarios {
		for _, metric := range descriptiveMetrics {
			values := make([]float64, 0)
			for _, row := range rows {
				if row.Scenario != scenario.Label {
					continue
				}
				value, available := metric.Value(row)
				if available {
					values = append(values, value)
				}
			}
			summary, err := experiment.SummarizeDescriptive(values)
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", scenario.Label, metric.Name, err)
			}
			result = append(result, descriptiveRow{
				Scenario: scenario.Label,
				Metric:   metric.Name,
				Unit:     metric.Unit,
				Summary:  summary,
			})
		}
	}
	return result, nil
}

func metricValues(rows []runRow, scenario, metricName string) ([]float64, error) {
	for _, metric := range descriptiveMetrics {
		if metric.Name != metricName {
			continue
		}
		values := make([]float64, 0)
		for _, row := range rows {
			if row.Scenario != scenario {
				continue
			}
			value, available := metric.Value(row)
			if available {
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			return nil, fmt.Errorf("%s/%s: no hay observaciones disponibles", scenario, metricName)
		}
		return values, nil
	}
	return nil, fmt.Errorf("métrica desconocida: %s", metricName)
}

func buildEffectRows(rows []runRow, replicas int, confidence float64, analysisSeed int64) ([]effectRow, error) {
	const baseline = "sin fallas"
	result := make([]effectRow, 0, (len(scenarios)-1)*len(descriptiveMetrics))
	for _, scenario := range scenarios[1:] {
		for _, metric := range descriptiveMetrics {
			baselineValues, err := metricValues(rows, baseline, metric.Name)
			if err != nil {
				return nil, err
			}
			scenarioValues, err := metricValues(rows, scenario.Label, metric.Name)
			if err != nil {
				return nil, err
			}
			bootstrapSeed := experiment.DeriveAnalysisSeed(analysisSeed, scenario.Label, metric.Name)
			estimate, err := experiment.EstimateBootstrapEffects(baselineValues, scenarioValues, experiment.BootstrapConfig{
				Replicas:   replicas,
				Confidence: confidence,
				Seed:       bootstrapSeed,
			})
			if err != nil {
				return nil, fmt.Errorf("%s/%s: %w", scenario.Label, metric.Name, err)
			}
			result = append(result, effectRow{
				Baseline:      baseline,
				Scenario:      scenario.Label,
				Metric:        metric.Name,
				Unit:          metric.Unit,
				AnalysisSeed:  analysisSeed,
				BootstrapSeed: bootstrapSeed,
				Replicas:      replicas,
				Confidence:    confidence,
				Estimate:      estimate,
			})
		}
	}
	return result, nil
}

func writeEffectsCSV(path string, rows []effectRow) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	writeErr := writer.Write([]string{
		"baseline",
		"scenario",
		"metric",
		"unit",
		"n_baseline",
		"n_scenario",
		"baseline_median",
		"scenario_median",
		"median_difference",
		"median_difference_ci_low",
		"median_difference_ci_high",
		"relative_change_percent",
		"cliffs_delta",
		"cliffs_delta_ci_low",
		"cliffs_delta_ci_high",
		"bootstrap_replicas",
		"confidence_level",
		"analysis_seed",
		"bootstrap_seed",
	})
	for _, row := range rows {
		if writeErr != nil {
			break
		}
		e := row.Estimate
		writeErr = writer.Write([]string{
			row.Baseline,
			row.Scenario,
			row.Metric,
			row.Unit,
			strconv.Itoa(e.BaselineN),
			strconv.Itoa(e.ScenarioN),
			strconv.FormatFloat(e.BaselineMedian, 'f', -1, 64),
			strconv.FormatFloat(e.ScenarioMedian, 'f', -1, 64),
			strconv.FormatFloat(e.MedianDifference, 'f', -1, 64),
			strconv.FormatFloat(e.MedianDifferenceCILow, 'f', -1, 64),
			strconv.FormatFloat(e.MedianDifferenceCIHigh, 'f', -1, 64),
			formatFloat(e.RelativeChangePercent),
			strconv.FormatFloat(e.CliffsDelta, 'f', -1, 64),
			strconv.FormatFloat(e.CliffsDeltaCILow, 'f', -1, 64),
			strconv.FormatFloat(e.CliffsDeltaCIHigh, 'f', -1, 64),
			strconv.Itoa(row.Replicas),
			strconv.FormatFloat(row.Confidence, 'f', -1, 64),
			strconv.FormatInt(row.AnalysisSeed, 10),
			strconv.FormatInt(row.BootstrapSeed, 10),
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

func writeDescriptiveCSV(path string, rows []descriptiveRow) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return err
	}
	writer := csv.NewWriter(file)
	writeErr := writer.Write([]string{
		"scenario",
		"metric",
		"unit",
		"n",
		"min",
		"q1",
		"median",
		"q3",
		"max",
		"iqr",
		"mad",
		"mean",
		"stddev",
	})
	for _, row := range rows {
		if writeErr != nil {
			break
		}
		writeErr = writer.Write([]string{
			row.Scenario,
			row.Metric,
			row.Unit,
			strconv.Itoa(row.Summary.N),
			strconv.FormatFloat(row.Summary.Min, 'f', -1, 64),
			strconv.FormatFloat(row.Summary.Q1, 'f', -1, 64),
			strconv.FormatFloat(row.Summary.Median, 'f', -1, 64),
			strconv.FormatFloat(row.Summary.Q3, 'f', -1, 64),
			strconv.FormatFloat(row.Summary.Max, 'f', -1, 64),
			strconv.FormatFloat(row.Summary.IQR, 'f', -1, 64),
			strconv.FormatFloat(row.Summary.MAD, 'f', -1, 64),
			strconv.FormatFloat(row.Summary.Mean, 'f', -1, 64),
			formatFloat(row.Summary.StdDev),
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
	descriptiveOutput := flag.String("descriptive-output", "", "archivo CSV nuevo para estadística descriptiva")
	effectsOutput := flag.String("effects-output", "", "archivo CSV nuevo para estimaciones de efecto")
	bootstrapReplicas := flag.Int("bootstrap-replicas", 100000, "número de réplicas bootstrap")
	confidenceLevel := flag.Float64("confidence-level", 0.95, "nivel de confianza bootstrap")
	analysisSeed := flag.Int64("analysis-seed", 20260912, "semilla maestra del análisis estadístico")
	runs := flag.Int("runs", 30, "número esperado de runs válidos por escenario")
	revision := flag.String("revision", finalRevision, "revisión Git experimental esperada")
	flag.Parse()

	if *dir == "" {
		return fmt.Errorf("se requiere -dir")
	}
	if *output == "" {
		return fmt.Errorf("se requiere -output")
	}
	if *descriptiveOutput != "" && *output == *descriptiveOutput {
		return fmt.Errorf("-output y -descriptive-output deben ser archivos diferentes")
	}
	if *effectsOutput != "" && (*effectsOutput == *output || *effectsOutput == *descriptiveOutput) {
		return fmt.Errorf("-effects-output debe ser diferente de las otras salidas")
	}
	if *effectsOutput != "" {
		if *bootstrapReplicas < 1 {
			return fmt.Errorf("-bootstrap-replicas debe ser mayor que cero")
		}
		if *confidenceLevel <= 0 || *confidenceLevel >= 1 {
			return fmt.Errorf("-confidence-level debe estar entre cero y uno")
		}
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
	var descriptiveRows []descriptiveRow
	if *descriptiveOutput != "" {
		descriptiveRows, err = buildDescriptiveRows(rows)
		if err != nil {
			return err
		}
		if err := writeDescriptiveCSV(*descriptiveOutput, descriptiveRows); err != nil {
			return fmt.Errorf("no se pudo escribir %s: %w", *descriptiveOutput, err)
		}
	}
	var effectRows []effectRow
	if *effectsOutput != "" {
		effectRows, err = buildEffectRows(rows, *bootstrapReplicas, *confidenceLevel, *analysisSeed)
		if err != nil {
			return err
		}
		if err := writeEffectsCSV(*effectsOutput, effectRows); err != nil {
			return fmt.Errorf("no se pudo escribir %s: %w", *effectsOutput, err)
		}
	}
	printVerification(summaries)
	fmt.Printf("Runs extraídos: %d\n", len(rows))
	if *descriptiveOutput == "" && *effectsOutput == "" {
		fmt.Printf("CSV: %s\n", *output)
	} else {
		fmt.Printf("CSV por run: %s\n", *output)
		if *descriptiveOutput != "" {
			fmt.Printf("Descriptivos: %d filas\n", len(descriptiveRows))
			fmt.Printf("CSV descriptivo: %s\n", *descriptiveOutput)
		}
		if *effectsOutput != "" {
			fmt.Printf("Efectos: %d comparaciones\n", len(effectRows))
			fmt.Printf("CSV de efectos: %s\n", *effectsOutput)
		}
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
