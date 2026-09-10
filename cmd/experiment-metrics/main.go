package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/kapumota/raftkv/internal/experiment"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	input := flag.String("input", "", "resultado JSON del runner")
	output := flag.String("output", "", "archivo nuevo de métricas")
	flag.Parse()
	if *input == "" || *output == "" {
		return fmt.Errorf("se requieren -input y -output")
	}
	file, err := os.Open(*input)
	if err != nil {
		return err
	}
	data, err := io.ReadAll(io.LimitReader(file, 256*1024*1024+1))
	file.Close()
	if err != nil {
		return err
	}
	if len(data) > 256*1024*1024 {
		return fmt.Errorf("el archivo supera 256 MiB")
	}
	var source struct {
		Revision string                      `json:"revision_git"`
		Changes  string                      `json:"cambios_git"`
		Scenario string                      `json:"archivo_escenario"`
		Result   experiment.ExperimentResult `json:"experimento"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&source); err != nil {
		return fmt.Errorf("resultado JSON inválido: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("se esperaba un único resultado JSON")
	}
	metrics, err := experiment.CalculateMetrics(source.Result)
	if err != nil {
		return err
	}
	result := struct {
		Version  int                `json:"version_metricas"`
		SHA256   string             `json:"raw_sha256"`
		Revision string             `json:"revision_git"`
		Changes  string             `json:"cambios_git"`
		Scenario string             `json:"archivo_escenario"`
		Metrics  experiment.Metrics `json:"metrics"`
	}{1, fmt.Sprintf("%x", sha256.Sum256(data)), source.Revision, source.Changes, source.Scenario, metrics}
	out, err := os.OpenFile(*output, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("no se pudo crear el archivo de métricas: %w", err)
	}
	defer out.Close()
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(result); err != nil {
		return err
	}
	return out.Sync()
}
