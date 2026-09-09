package experiment

import (
	"fmt"
	"io"
	"strings"

	"gopkg.in/yaml.v3"
)

type FaultConfig struct {
	Type     string `yaml:"tipo" json:"tipo"`
	Target   string `yaml:"objetivo" json:"objetivo"`
	At       int    `yaml:"instante_segundos" json:"instante_segundos"`
	Duration int    `yaml:"duracion_segundos" json:"duracion_segundos"`
}

type ExperimentConfig struct {
	Version  int         `yaml:"version" json:"version"`
	Name     string      `yaml:"nombre" json:"nombre"`
	Seed     int64       `yaml:"semilla" json:"semilla"`
	Duration int         `yaml:"duracion_segundos" json:"duracion_segundos"`
	Clients  int         `yaml:"clientes" json:"clientes"`
	Rate     int         `yaml:"operaciones_por_segundo" json:"operaciones_por_segundo"`
	Fault    FaultConfig `yaml:"falla" json:"falla"`
}

// LoadConfig admite un único documento y exige los tipos escalares del contrato.
func LoadConfig(r io.Reader) (ExperimentConfig, error) {
	var config ExperimentConfig
	data, err := io.ReadAll(io.LimitReader(r, 65537))
	if err != nil || len(data) > 65536 {
		return config, fmt.Errorf("no se pudo leer el escenario o supera 64 KiB")
	}
	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		return config, fmt.Errorf("no se pudo leer el escenario: %w", err)
	}
	fields := map[string]string{"version": "!!int", "nombre": "!!str", "semilla": "!!int", "duracion_segundos": "!!int", "clientes": "!!int", "operaciones_por_segundo": "!!int", "falla": "!!map"}
	if len(document.Content) != 1 {
		return config, fmt.Errorf("se requiere un documento de configuración")
	}
	if err := validateMapping(document.Content[0], fields); err != nil {
		return config, err
	}
	for i := 0; i < len(document.Content[0].Content); i += 2 {
		if document.Content[0].Content[i].Value == "falla" {
			if err := validateMapping(document.Content[0].Content[i+1], map[string]string{"tipo": "!!str", "objetivo": "!!str", "instante_segundos": "!!int", "duracion_segundos": "!!int"}); err != nil {
				return config, err
			}
		}
	}
	if err := document.Decode(&config); err != nil {
		return config, fmt.Errorf("configuración inválida: %w", err)
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); err != io.EOF {
		return config, fmt.Errorf("se requiere exactamente un documento YAML")
	}
	return config, config.Validate()
}

func validateMapping(node *yaml.Node, fields map[string]string) error {
	if node.Kind != yaml.MappingNode || node.Tag != "!!map" {
		return fmt.Errorf("se esperaba un mapa YAML")
	}
	seen := make(map[string]bool)
	for i := 0; i < len(node.Content); i += 2 {
		key, value := node.Content[i], node.Content[i+1]
		tag, ok := fields[key.Value]
		if key.Kind != yaml.ScalarNode || key.Tag != "!!str" || !ok || seen[key.Value] {
			return fmt.Errorf("campo desconocido o duplicado: %s", key.Value)
		}
		if value.Kind == yaml.AliasNode || value.Tag != tag {
			return fmt.Errorf("tipo incorrecto para %s", key.Value)
		}
		seen[key.Value] = true
	}
	if len(seen) != len(fields) {
		return fmt.Errorf("faltan campos obligatorios")
	}
	return nil
}

func (c ExperimentConfig) Validate() error {
	if c.Version != 1 || strings.TrimSpace(c.Name) == "" || c.Seed < 0 {
		return fmt.Errorf("versión, nombre o semilla inválidos")
	}
	// Los límites acotan temporizadores, concurrencia y memoria de resultados.
	if c.Duration < 3 || c.Duration > 3600 || c.Clients < 1 || c.Clients > 128 || c.Rate < 1 || c.Rate > 1000 || c.Duration*c.Rate > 1000000 {
		return fmt.Errorf("duración, clientes o tasa fuera de los límites admitidos")
	}
	if c.Fault.Type != "caida" && c.Fault.Type != "particion" {
		return fmt.Errorf("tipo de falla desconocido")
	}
	if c.Fault.Target != "lider" && c.Fault.Target != "seguidor" {
		return fmt.Errorf("objetivo de falla desconocido")
	}
	if c.Fault.At <= 0 || c.Fault.At >= c.Duration || c.Fault.Duration <= 0 || c.Fault.Duration >= c.Duration-c.Fault.At {
		return fmt.Errorf("la falla debe dejar una ventana anterior y otra posterior")
	}
	return nil
}
