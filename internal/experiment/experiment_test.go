package experiment

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validScenario = `version: 1
nombre: prueba
semilla: 0
duracion_segundos: 3
clientes: 1
operaciones_por_segundo: 1
falla:
  tipo: caida
  objetivo: lider
  instante_segundos: 1
  duracion_segundos: 1
`

func TestLoadConfig(t *testing.T) {
	if _, err := LoadConfig(strings.NewReader(validScenario)); err != nil {
		t.Fatal(err)
	}
	for _, invalid := range []string{
		validScenario + "extra: 1\n",
		validScenario + "version: 1\n",
		validScenario + "---\n" + validScenario,
		strings.Replace(validScenario, "semilla: 0\n", "", 1),
		strings.Replace(validScenario, "clientes: 1", "clientes: 1.5", 1),
		strings.Replace(validScenario, "nombre: prueba", "nombre: 12", 1),
		strings.Replace(validScenario, "instante_segundos: 1", "instante_segundos: 3", 1),
		strings.Replace(validScenario, "duracion_segundos: 3", "duracion_segundos: 9223372036854775807", 1),
	} {
		if _, err := LoadConfig(strings.NewReader(invalid)); err == nil {
			t.Fatalf("se aceptó un escenario inválido: %s", invalid)
		}
	}
}

func TestScenarioFiles(t *testing.T) {
	files, err := filepath.Glob("../../experiments/*.yaml")
	if err != nil || len(files) != 4 {
		t.Fatalf("se esperaban cuatro escenarios: %v", err)
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := LoadConfig(strings.NewReader(string(data))); err != nil {
			t.Fatalf("%s: %v", path, err)
		}
	}
}

type fakeBackend struct {
	cancel          context.CancelFunc
	failApply       bool
	failRestore     bool
	applyNode       string
	restoreNode     string
	cleanupCanceled bool
}

func (b *fakeBackend) Statuses(context.Context) ([]NodeStatus, error) {
	var nodes []NodeStatus
	for i := 1; i <= 5; i++ {
		state := "seguidor"
		if i == 2 {
			state = "lider"
		}
		nodes = append(nodes, NodeStatus{ID: fmt.Sprintf("nodo-%d", i), State: state, Term: 1, CommitIndex: 1})
	}
	return nodes, nil
}
func (b *fakeBackend) Prepare(context.Context) (map[string]string, error) {
	return map[string]string{"inicial": "prueba"}, nil
}
func (b *fakeBackend) Write(context.Context, string, string, string) (int, error) { return 200, nil }
func (b *fakeBackend) Apply(_ context.Context, _ string, id string) error {
	b.applyNode = id
	if b.cancel != nil {
		b.cancel()
	}
	if b.failApply {
		return errors.New("falló la aplicación simulada")
	}
	return nil
}
func (b *fakeBackend) Restore(ctx context.Context, _ string, id string) error {
	b.restoreNode = id
	b.cleanupCanceled = ctx.Err() != nil
	if b.failRestore {
		return errors.New("falló la restauración simulada")
	}
	return nil
}

func TestRunnerRestoresAfterFailureOrCancellation(t *testing.T) {
	for _, failApply := range []bool{false, true} {
		config, _ := LoadConfig(strings.NewReader(validScenario))
		ctx, cancel := context.WithCancel(context.Background())
		b := &fakeBackend{cancel: cancel, failApply: failApply}
		result, err := runExperiment(ctx, config, b)
		cancel()
		if err == nil || result.Error == "" || b.applyNode != "nodo-2" || b.restoreNode != b.applyNode || b.cleanupCanceled {
			t.Fatalf("restauración incorrecta: resultado=%+v, backend=%+v, error=%v", result, b, err)
		}
	}
}

func TestRunnerReportsRestoreFailure(t *testing.T) {
	config, _ := LoadConfig(strings.NewReader(validScenario))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	b := &fakeBackend{cancel: cancel, failRestore: true}
	result, err := runExperiment(ctx, config, b)
	if err == nil || !strings.Contains(result.Error, "restauración simulada") {
		t.Fatalf("no se registró el fallo de restauración: %v", err)
	}
}

func TestRunnerCompletesScenario(t *testing.T) {
	config, _ := LoadConfig(strings.NewReader(validScenario))
	config.Fault.Target = "seguidor"
	b := &fakeBackend{}
	result, err := runExperiment(context.Background(), config, b)
	if err != nil || result.Error != "" || b.applyNode != "nodo-1" || b.restoreNode != b.applyNode || len(result.Operations) != 3 {
		t.Fatalf("resultado inesperado: %+v, error=%v", result, err)
	}
}
