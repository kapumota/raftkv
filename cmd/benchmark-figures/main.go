package main

import (
	"crypto/sha256"
	"encoding/csv"
	"flag"
	"fmt"
	"html"
	"io"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

const (
	defaultRevision = "8ba7a131c4f452aac014a4c628794062fa1a8c9b"
	svgWidth        = 960
)

var scenarioOrder = []string{
	"sin fallas",
	"follower caído",
	"leader caído",
	"partición de leader",
}

var metricOrder = []string{
	"throughput",
	"latency_p50_ms",
	"latency_p95_ms",
	"latency_p99_ms",
	"uncertain_writes",
	"omitted_operations",
}

type descriptiveRow struct {
	Scenario string
	Metric   string
	Unit     string
	Q1       float64
	Median   float64
	Q3       float64
}

type comparisonRow struct {
	Scenario     string
	Metric       string
	Unit         string
	Cliff        float64
	CliffLow     float64
	CliffHigh    float64
	HolmAdjusted float64
}

type figureSpec struct {
	Filename string
	Metric   string
	Title    string
	Axis     string
}

func main() {
	descriptivePath := flag.String("descriptive", "", "CSV descriptivo de H2")
	comparisonsPath := flag.String("comparisons", "", "CSV de comparaciones inferenciales de H2")
	outputDir := flag.String("output-dir", "experiments/figures", "directorio de salida")
	revision := flag.String("revision", defaultRevision, "revisión experimental mostrada en las figuras")
	flag.Parse()

	if *descriptivePath == "" || *comparisonsPath == "" {
		fmt.Fprintln(os.Stderr, "-descriptive y -comparisons son obligatorios")
		os.Exit(2)
	}

	if err := generateFigures(*descriptivePath, *comparisonsPath, *outputDir, *revision); err != nil {
		fmt.Fprintf(os.Stderr, "error al generar figuras: %v\n", err)
		os.Exit(1)
	}
}

func generateFigures(descriptivePath, comparisonsPath, outputDir, revision string) error {
	descriptive, err := readDescriptive(descriptivePath)
	if err != nil {
		return err
	}
	comparisons, err := readComparisons(comparisonsPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("crear directorio de salida: %w", err)
	}

	specs := []figureSpec{
		{Filename: "throughput.svg", Metric: "throughput", Title: "Throughput por escenario", Axis: "writes/s"},
		{Filename: "latency-p95.svg", Metric: "latency_p95_ms", Title: "Latencia p95 por escenario", Axis: "ms"},
		{Filename: "latency-p99.svg", Metric: "latency_p99_ms", Title: "Latencia p99 por escenario", Axis: "ms"},
		{Filename: "uncertain-writes.svg", Metric: "uncertain_writes", Title: "Escrituras inciertas por escenario", Axis: "operaciones por run"},
	}

	generated := make([]string, 0, len(specs)+1)
	for _, spec := range specs {
		content, err := renderDescriptiveFigure(descriptive, spec, revision)
		if err != nil {
			return err
		}
		path := filepath.Join(outputDir, spec.Filename)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return fmt.Errorf("escribir %s: %w", path, err)
		}
		generated = append(generated, spec.Filename)
	}

	forest, err := renderCliffsDeltaFigure(comparisons, revision)
	if err != nil {
		return err
	}
	forestName := "cliffs-delta.svg"
	if err := os.WriteFile(filepath.Join(outputDir, forestName), []byte(forest), 0o644); err != nil {
		return fmt.Errorf("escribir %s: %w", forestName, err)
	}
	generated = append(generated, forestName)

	sort.Strings(generated)
	var manifest strings.Builder
	for _, name := range generated {
		data, err := os.ReadFile(filepath.Join(outputDir, name))
		if err != nil {
			return fmt.Errorf("leer %s para manifest: %w", name, err)
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&manifest, "%x  %s\n", sum, name)
	}
	if err := os.WriteFile(filepath.Join(outputDir, "figures-manifest.sha256"), []byte(manifest.String()), 0o644); err != nil {
		return fmt.Errorf("escribir manifest de figuras: %w", err)
	}
	return nil
}

func readDescriptive(path string) ([]descriptiveRow, error) {
	rows, err := readCSV(path)
	if err != nil {
		return nil, fmt.Errorf("leer descriptivos: %w", err)
	}
	out := make([]descriptiveRow, 0, len(rows))
	for i, row := range rows {
		q1, err := fieldFloat(row, "q1")
		if err != nil {
			return nil, fmt.Errorf("fila descriptiva %d: %w", i+2, err)
		}
		median, err := fieldFloat(row, "median")
		if err != nil {
			return nil, fmt.Errorf("fila descriptiva %d: %w", i+2, err)
		}
		q3, err := fieldFloat(row, "q3")
		if err != nil {
			return nil, fmt.Errorf("fila descriptiva %d: %w", i+2, err)
		}
		out = append(out, descriptiveRow{Scenario: row["scenario"], Metric: row["metric"], Unit: row["unit"], Q1: q1, Median: median, Q3: q3})
	}
	return out, nil
}

func readComparisons(path string) ([]comparisonRow, error) {
	rows, err := readCSV(path)
	if err != nil {
		return nil, fmt.Errorf("leer comparaciones: %w", err)
	}
	out := make([]comparisonRow, 0, len(rows))
	for i, row := range rows {
		cliff, err := fieldFloat(row, "cliffs_delta")
		if err != nil {
			return nil, fmt.Errorf("fila de comparación %d: %w", i+2, err)
		}
		low, err := fieldFloat(row, "cliffs_delta_ci_low")
		if err != nil {
			return nil, fmt.Errorf("fila de comparación %d: %w", i+2, err)
		}
		high, err := fieldFloat(row, "cliffs_delta_ci_high")
		if err != nil {
			return nil, fmt.Errorf("fila de comparación %d: %w", i+2, err)
		}
		holm, err := fieldFloat(row, "holm_adjusted_p")
		if err != nil {
			return nil, fmt.Errorf("fila de comparación %d: %w", i+2, err)
		}
		out = append(out, comparisonRow{Scenario: row["scenario"], Metric: row["metric"], Unit: row["unit"], Cliff: cliff, CliffLow: low, CliffHigh: high, HolmAdjusted: holm})
	}
	return out, nil
}

func readCSV(path string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	header, err := r.Read()
	if err != nil {
		return nil, err
	}
	rows := []map[string]string{}
	for {
		record, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if len(record) != len(header) {
			return nil, fmt.Errorf("número de columnas inválido")
		}
		row := make(map[string]string, len(header))
		for i, key := range header {
			row[key] = record[i]
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func fieldFloat(row map[string]string, key string) (float64, error) {
	raw, ok := row[key]
	if !ok || strings.TrimSpace(raw) == "" {
		return 0, fmt.Errorf("falta %s", key)
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, fmt.Errorf("%s inválido: %q", key, raw)
	}
	return value, nil
}

func renderDescriptiveFigure(rows []descriptiveRow, spec figureSpec, revision string) (string, error) {
	selected := make(map[string]descriptiveRow)
	minValue := math.Inf(1)
	maxValue := math.Inf(-1)
	for _, row := range rows {
		if row.Metric != spec.Metric {
			continue
		}
		selected[row.Scenario] = row
		minValue = math.Min(minValue, row.Q1)
		maxValue = math.Max(maxValue, row.Q3)
	}
	if len(selected) != len(scenarioOrder) {
		return "", fmt.Errorf("%s: se esperaban %d escenarios y se encontraron %d", spec.Metric, len(scenarioOrder), len(selected))
	}
	if !(maxValue > minValue) {
		maxValue = minValue + 1
	}
	padding := (maxValue - minValue) * 0.12
	if spec.Metric == "uncertain_writes" && minValue > 0 {
		minValue = 0
	}
	minValue -= padding
	if minValue < 0 && (spec.Metric == "throughput" || spec.Metric == "uncertain_writes") {
		minValue = 0
	}
	maxValue += padding

	const left, right, top, rowGap = 220.0, 60.0, 92.0, 58.0
	plotWidth := float64(svgWidth) - left - right
	height := int(top + rowGap*float64(len(scenarioOrder)) + 76)
	x := func(v float64) float64 { return left + (v-minValue)/(maxValue-minValue)*plotWidth }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", svgWidth, height, svgWidth, height)
	writeStyle(&b)
	fmt.Fprintf(&b, `<text class="title" x="%d" y="34">%s</text>`+"\n", svgWidth/2, esc(spec.Title))
	fmt.Fprintf(&b, `<text class="subtitle" x="%d" y="58">mediana e IQR, n=30 por escenario, revision %s</text>`+"\n", svgWidth/2, esc(shortRevision(revision)))

	for i := 0; i <= 4; i++ {
		value := minValue + float64(i)*(maxValue-minValue)/4
		xpos := x(value)
		fmt.Fprintf(&b, `<line class="grid" x1="%.2f" y1="72" x2="%.2f" y2="%d"/>`+"\n", xpos, xpos, height-58)
		fmt.Fprintf(&b, `<text class="tick" x="%.2f" y="%d">%s</text>`+"\n", xpos, height-36, formatTick(value))
	}

	for i, scenario := range scenarioOrder {
		row := selected[scenario]
		y := top + float64(i)*rowGap
		fmt.Fprintf(&b, `<text class="label" x="%g" y="%.2f">%s</text>`+"\n", left-18, y+5, esc(scenario))
		fmt.Fprintf(&b, `<line class="interval" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`+"\n", x(row.Q1), y, x(row.Q3), y)
		fmt.Fprintf(&b, `<line class="cap" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`+"\n", x(row.Q1), y-8, x(row.Q1), y+8)
		fmt.Fprintf(&b, `<line class="cap" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`+"\n", x(row.Q3), y-8, x(row.Q3), y+8)
		fmt.Fprintf(&b, `<circle class="point" cx="%.2f" cy="%.2f" r="5"/>`+"\n", x(row.Median), y)
		fmt.Fprintf(&b, `<text class="value" x="%.2f" y="%.2f">%s</text>`+"\n", x(row.Median)+10, y-10, formatValue(row.Median, spec.Metric))
	}
	fmt.Fprintf(&b, `<text class="axis" x="%d" y="%d">%s</text>`+"\n", int(left+plotWidth/2), height-8, esc(spec.Axis))
	b.WriteString("</svg>\n")
	return b.String(), nil
}

func renderCliffsDeltaFigure(rows []comparisonRow, revision string) (string, error) {
	expected := len(metricOrder) * 3
	if len(rows) != expected {
		return "", fmt.Errorf("Cliff's delta: se esperaban %d comparaciones y se encontraron %d", expected, len(rows))
	}
	index := map[string]comparisonRow{}
	for _, row := range rows {
		index[row.Scenario+"\x00"+row.Metric] = row
	}
	scenarios := scenarioOrder[1:]
	const left, right, top, rowGap = 330.0, 110.0, 92.0, 34.0
	plotWidth := float64(svgWidth) - left - right
	height := int(top + rowGap*float64(expected) + 72)
	x := func(v float64) float64 { return left + (v+1)/2*plotWidth }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d">`+"\n", svgWidth, height, svgWidth, height)
	writeStyle(&b)
	fmt.Fprintf(&b, `<text class="title" x="%d" y="34">Cliff's delta frente al baseline</text>`+"\n", svgWidth/2)
	fmt.Fprintf(&b, `<text class="subtitle" x="%d" y="58">IC bootstrap 95%%; * indica Holm ajustado &lt; 0.05; revision %s</text>`+"\n", svgWidth/2, esc(shortRevision(revision)))
	for _, tick := range []float64{-1, -0.5, 0, 0.5, 1} {
		xpos := x(tick)
		cls := "grid"
		if tick == 0 {
			cls = "zero"
		}
		fmt.Fprintf(&b, `<line class="%s" x1="%.2f" y1="72" x2="%.2f" y2="%d"/>`+"\n", cls, xpos, xpos, height-48)
		fmt.Fprintf(&b, `<text class="tick" x="%.2f" y="%d">%.1f</text>`+"\n", xpos, height-28, tick)
	}
	rowIndex := 0
	for _, scenario := range scenarios {
		for _, metric := range metricOrder {
			row, ok := index[scenario+"\x00"+metric]
			if !ok {
				return "", fmt.Errorf("falta comparación %s/%s", scenario, metric)
			}
			y := top + float64(rowIndex)*rowGap
			marker := ""
			pointClass := "point-open"
			if row.HolmAdjusted < 0.05 {
				marker = " *"
				pointClass = "point"
			}
			label := fmt.Sprintf("%s / %s%s", scenario, metricLabel(metric), marker)
			fmt.Fprintf(&b, `<text class="label-small" x="%g" y="%.2f">%s</text>`+"\n", left-14, y+4, esc(label))
			fmt.Fprintf(&b, `<line class="interval" x1="%.2f" y1="%.2f" x2="%.2f" y2="%.2f"/>`+"\n", x(row.CliffLow), y, x(row.CliffHigh), y)
			fmt.Fprintf(&b, `<circle class="%s" cx="%.2f" cy="%.2f" r="4.5"/>`+"\n", pointClass, x(row.Cliff), y)
			fmt.Fprintf(&b, `<text class="pvalue" x="%.2f" y="%.2f">pH=%s</text>`+"\n", float64(svgWidth)-right+8, y+4, formatP(row.HolmAdjusted))
			rowIndex++
		}
	}
	fmt.Fprintf(&b, `<text class="axis" x="%d" y="%d">Cliff's delta</text>`+"\n", int(left+plotWidth/2), height-4)
	b.WriteString("</svg>\n")
	return b.String(), nil
}

func writeStyle(b *strings.Builder) {
	b.WriteString(`<style>
text{font-family:Arial,Helvetica,sans-serif;fill:#111}.title{font-size:22px;font-weight:700;text-anchor:middle}.subtitle{font-size:12px;text-anchor:middle}.label{font-size:14px;text-anchor:end}.label-small{font-size:11px;text-anchor:end}.tick{font-size:11px;text-anchor:middle}.axis{font-size:13px;text-anchor:middle;font-weight:600}.value{font-size:11px}.pvalue{font-size:10px}.grid{stroke:#ddd;stroke-width:1}.zero{stroke:#555;stroke-width:1.5}.interval{stroke:#111;stroke-width:2}.cap{stroke:#111;stroke-width:1.5}.point{fill:#111;stroke:#111}.point-open{fill:#fff;stroke:#111;stroke-width:1.5}
</style>` + "\n")
}

func esc(s string) string { return html.EscapeString(s) }
func shortRevision(s string) string {
	if len(s) > 8 {
		return s[:8]
	}
	return s
}

func metricLabel(metric string) string {
	switch metric {
	case "throughput":
		return "throughput"
	case "latency_p50_ms":
		return "p50"
	case "latency_p95_ms":
		return "p95"
	case "latency_p99_ms":
		return "p99"
	case "uncertain_writes":
		return "inciertas"
	case "omitted_operations":
		return "omitidas"
	default:
		return metric
	}
}

func formatTick(v float64) string {
	av := math.Abs(v)
	if av >= 100 {
		return fmt.Sprintf("%.0f", v)
	}
	if av >= 10 {
		return fmt.Sprintf("%.1f", v)
	}
	return fmt.Sprintf("%.2f", v)
}

func formatValue(v float64, metric string) string {
	switch metric {
	case "throughput":
		return fmt.Sprintf("%.3f", v)
	case "uncertain_writes":
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.0f", v)
	}
}

func formatP(p float64) string {
	if p < 0.001 {
		return "<0.001"
	}
	if p >= 0.9995 {
		return "1.000"
	}
	return fmt.Sprintf("%.3f", p)
}
