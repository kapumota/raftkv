package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/kapumota/raftkv/internal/experiment"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	scenario := flag.String("scenario", "", "ruta del escenario YAML")
	output := flag.String("output", "", "archivo JSON nuevo para resultados")
	check := flag.Bool("check", false, "validar sin ejecutar fallas")
	flag.Parse()
	if *scenario == "" {
		return fmt.Errorf("se requiere -scenario")
	}
	file, err := os.Open(*scenario)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el escenario: %w", err)
	}
	config, err := experiment.LoadConfig(file)
	file.Close()
	if err != nil {
		return err
	}
	if *check {
		fmt.Println("Escenario válido:", config.Name)
		return nil
	}
	lock, err := os.OpenFile(os.TempDir()+"/raftkv-experiment.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("no se pudo abrir el bloqueo del runner: %w", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return fmt.Errorf("otro experimento está usando el runner")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if *output == "" {
		return fmt.Errorf("se requiere -output para conservar los resultados")
	}
	resultFile, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("no se pudo crear el resultado; no se sobrescriben archivos: %w", err)
	}
	defer resultFile.Close()
	revision, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return fmt.Errorf("no se pudo identificar la revisión de Git: %w", err)
	}
	changes, err := exec.Command("git", "status", "--porcelain").Output()
	if err != nil {
		return fmt.Errorf("no se pudo registrar el estado de Git: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, runErr := experiment.RunExperimentContext(ctx, config)
	envelope := struct {
		Revision string                      `json:"revision_git"`
		Changes  string                      `json:"cambios_git"`
		Scenario string                      `json:"archivo_escenario"`
		Result   experiment.ExperimentResult `json:"experimento"`
	}{strings.TrimSpace(string(revision)), string(changes), *scenario, result}
	encoder := json.NewEncoder(resultFile)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(envelope); err != nil {
		return fmt.Errorf("no se pudo guardar el resultado: %w", err)
	}
	if err := resultFile.Sync(); err != nil {
		return fmt.Errorf("no se pudo sincronizar el resultado: %w", err)
	}
	return runErr
}
