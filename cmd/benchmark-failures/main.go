package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/kapumota/raftkv/internal/experiment"
)

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

func load(path string) (runFile, error) {
	var result runFile
	file, err := os.Open(path)
	if err != nil {
		return result, err
	}
	defer file.Close()

	decoder := json.NewDecoder(file)
	if err := decoder.Decode(&result); err != nil {
		return result, err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return result, fmt.Errorf("%s: se requiere un único resultado JSON", path)
	}
	return result, nil
}

func loadGroup(dir string, spec scenarioSpec, expected int, revision *string) ([]experiment.ExperimentResult, error) {
	paths, err := filepath.Glob(filepath.Join(dir, spec.Pattern))
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)
	if len(paths) != expected {
		return nil, fmt.Errorf("%s: se esperaban %d ejecuciones y se encontraron %d", spec.Label, expected, len(paths))
	}

	results := make([]experiment.ExperimentResult, 0, len(paths))
	for _, path := range paths {
		run, err := load(path)
		if err != nil {
			return nil, err
		}
		if run.Revision == "" || run.Changes != "" {
			return nil, fmt.Errorf("%s: se requiere una revisión Git identificada y un árbol limpio", path)
		}
		if *revision == "" {
			*revision = run.Revision
		} else if run.Revision != *revision {
			return nil, fmt.Errorf("%s: todas las ejecuciones deben usar la misma revisión Git", path)
		}

		config := run.Result.Config
		if config.Nodes != 5 || config.Deployment != "benchmark" ||
			config.Name != "benchmark_fallas" ||
			config.Fault.Type != spec.Type || config.Fault.Target != spec.Target {
			return nil, fmt.Errorf("%s: la configuración no corresponde al escenario %s", path, spec.Label)
		}
		results = append(results, run.Result)
	}
	return results, nil
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

func format(value *float64) string {
	if value == nil {
		return "no disponible"
	}
	return fmt.Sprintf("%.3f", *value)
}

func run() error {
	dir := flag.String("dir", "", "directorio con resultados JSON de G3")
	runs := flag.Int("runs", 30, "número esperado de ejecuciones por escenario")
	flag.Parse()

	if *dir == "" {
		return fmt.Errorf("se requiere -dir")
	}
	if *runs < 1 {
		return fmt.Errorf("-runs debe ser mayor que cero")
	}

	specs := []scenarioSpec{
		{Label: "sin fallas", Pattern: "normal-faults-5-*.json", Type: "ninguna", Target: ""},
		{Label: "follower caído", Pattern: "follower-down-5-*.json", Type: "caida", Target: "seguidor"},
		{Label: "leader caído", Pattern: "leader-down-5-*.json", Type: "caida", Target: "lider"},
		{Label: "partición de leader", Pattern: "leader-partition-5-*.json", Type: "particion", Target: "lider"},
	}

	var revision string
	var baseline experiment.ExperimentConfig
	fmt.Println("Escenario | Runs | Throughput mediano | p50 mediana (ms) | p95 mediana (ms) | p99 mediana (ms) | Inciertas mediana | Omitidas mediana | Restauradas")
	for i, spec := range specs {
		results, err := loadGroup(*dir, spec, *runs, &revision)
		if err != nil {
			return err
		}
		if i == 0 {
			baseline = results[0].Config
		} else if !sameWorkload(baseline, results[0].Config) {
			return fmt.Errorf("%s: la carga difiere del baseline", spec.Label)
		}

		summary, err := experiment.AggregateBenchmarkRuns(results)
		if err != nil {
			return fmt.Errorf("%s: %w", spec.Label, err)
		}
		fmt.Printf("%s | %d | %s | %s | %s | %s | %.3f | %.3f | %d/%d\n",
			spec.Label,
			summary.Runs,
			format(summary.ThroughputMedian),
			format(summary.LatencyP50Median),
			format(summary.LatencyP95Median),
			format(summary.LatencyP99Median),
			summary.UncertainMedian,
			summary.OmittedMedian,
			summary.RestoredRuns,
			summary.Runs,
		)
	}
	fmt.Printf("Revisión Git: %s\n", revision)
	fmt.Println("Resumen descriptivo de ejecuciones repetidas; G3 no realiza inferencia estadística.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
