package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kapumota/raftkv/internal/experiment"
)

type runFile struct {
	Revision string                      `json:"revision_git"`
	Changes  string                      `json:"cambios_git"`
	Result   experiment.ExperimentResult `json:"experimento"`
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
		return result, fmt.Errorf("se requiere un único resultado JSON")
	}
	return result, nil
}

func format(value *float64) string {
	if value == nil {
		return "no disponible"
	}
	return fmt.Sprintf("%.3f", *value)
}

func run() error {
	three := flag.String("three", "", "resultado de tres nodos")
	five := flag.String("five", "", "resultado de cinco nodos")
	flag.Parse()
	if *three == "" || *five == "" {
		return fmt.Errorf("se requieren -three y -five")
	}
	a, err := load(*three)
	if err != nil {
		return err
	}
	b, err := load(*five)
	if err != nil {
		return err
	}
	if a.Revision == "" || a.Revision != b.Revision || a.Changes != "" || b.Changes != "" {
		return fmt.Errorf("se requiere la misma revisión y un árbol de trabajo limpio en ambas ejecuciones")
	}
	metrics, err := experiment.CompareNormalRuns(a.Result, b.Result)
	if err != nil {
		return err
	}
	fmt.Println("Nodos | Throughput (escrituras/s) | p50 (ms) | p95 (ms) | p99 (ms) | Inciertas | Omitidas")
	for i, n := range []int{3, 5} {
		m := metrics[i]
		fmt.Printf("%d | %s | %s | %s | %s | %d | %d\n", n, format(m.Throughput), format(m.LatencyP50), format(m.LatencyP95), format(m.LatencyP99), m.Uncertain, m.Omitted)
	}
	fmt.Println("Comparación descriptiva de dos ejecuciones; no es una conclusión estadística.")
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
