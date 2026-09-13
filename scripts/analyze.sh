#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

RUNS="${RUNS:-30}"
REVISION="${REVISION:-8ba7a131c4f452aac014a4c628794062fa1a8c9b}"
CAMPAIGN="${CAMPAIGN:-final}"
ANALYSIS_SEED="${ANALYSIS_SEED:-20260912}"
BOOTSTRAP_REPLICAS="${BOOTSTRAP_REPLICAS:-100000}"
PERMUTATION_REPLICAS="${PERMUTATION_REPLICAS:-100000}"
CONFIDENCE_LEVEL="${CONFIDENCE_LEVEL:-0.95}"
ALPHA="${ALPHA:-0.05}"
RAW_DIR="${RAW_DIR:-experiments/raw/${REVISION}/${CAMPAIGN}}"
PROCESSED_DIR="${PROCESSED_DIR:-experiments/processed/${REVISION}/${CAMPAIGN}}"
STATISTICS_DIR="${STATISTICS_DIR:-${PROCESSED_DIR}/statistics}"
RAW_MANIFEST="${RAW_MANIFEST:-${PROCESSED_DIR}/raw-manifest.sha256}"

for value_name in RUNS BOOTSTRAP_REPLICAS PERMUTATION_REPLICAS; do
  value="${!value_name}"
  if ! [[ "$value" =~ ^[1-9][0-9]*$ ]]; then
    echo "$value_name debe ser un entero mayor que cero" >&2
    exit 1
  fi
done

if ! [[ "$ANALYSIS_SEED" =~ ^-?[0-9]+$ ]]; then
  echo "ANALYSIS_SEED debe ser un entero" >&2
  exit 1
fi

if ! [[ "$CAMPAIGN" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "CAMPAIGN solo admite letras, números, punto, guion y guion bajo" >&2
  exit 1
fi

if ! git cat-file -e "${REVISION}^{commit}" 2>/dev/null; then
  echo "La revisión experimental no existe en el repositorio: $REVISION" >&2
  exit 1
fi

if ! git merge-base --is-ancestor "$REVISION" HEAD; then
  echo "La revisión experimental no es ancestro de HEAD: $REVISION" >&2
  exit 1
fi

if [[ ! -d "$RAW_DIR" ]]; then
  echo "No existe el directorio de datos crudos: $RAW_DIR" >&2
  exit 1
fi

if [[ ! -f "$RAW_MANIFEST" ]]; then
  echo "No existe el manifest G4 de datos crudos: $RAW_MANIFEST" >&2
  exit 1
fi

if [[ "$RAW_MANIFEST" = /* ]]; then
  raw_manifest_abs="$RAW_MANIFEST"
else
  raw_manifest_abs="$ROOT/$RAW_MANIFEST"
fi

# Solo se permiten cambios generados dentro del directorio estadístico de H2.
dirty="$(
  git status --porcelain --untracked-files=all |
    awk -v allowed="$STATISTICS_DIR/" '
      {
        path = substr($0, 4)
        if (index(path, allowed) != 1) {
          print
        }
      }
    '
)"
if [[ -n "$dirty" ]]; then
  echo "make analyze exige código y evidencia cruda sin cambios." >&2
  printf '%s\n' "$dirty" >&2
  exit 1
fi

json_count="$(
  find "$RAW_DIR" -maxdepth 1 -type f -name '*.json' -printf '.' | wc -c
)"
expected_count=$((4 * RUNS))
if [[ "$json_count" -ne "$expected_count" ]]; then
  echo "Se esperaban $expected_count JSON y se encontraron $json_count en $RAW_DIR" >&2
  exit 1
fi

if ! (
  cd "$RAW_DIR"
  sha256sum -c "$raw_manifest_abs"
) >/dev/null; then
  echo "La evidencia cruda no coincide con el manifest G4: $RAW_MANIFEST" >&2
  exit 1
fi

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

processor="$tmp_dir/benchmark-stats"
go build -o "$processor" ./cmd/benchmark-stats

"$processor" \
  -dir "$RAW_DIR" \
  -revision "$REVISION" \
  -runs "$RUNS" \
  -output "$tmp_dir/per-run.csv" \
  -descriptive-output "$tmp_dir/descriptive.csv" \
  -effects-output "$tmp_dir/effects.csv" \
  -comparisons-output "$tmp_dir/comparisons.csv" \
  -bootstrap-replicas "$BOOTSTRAP_REPLICAS" \
  -permutation-replicas "$PERMUTATION_REPLICAS" \
  -confidence-level "$CONFIDENCE_LEVEL" \
  -alpha "$ALPHA" \
  -analysis-seed "$ANALYSIS_SEED" \
  > /dev/null

mapfile -t analysis_sources < <(
  {
    find cmd/benchmark-stats internal/experiment \
      -maxdepth 1 -type f -name '*.go' ! -name '*_test.go' -print
    printf '%s\n' Makefile go.mod go.sum scripts/analyze.sh
  } | LC_ALL=C sort
)

for file in "${analysis_sources[@]}"; do
  sha256sum "$file"
done > "$tmp_dir/analysis-source-manifest.sha256"

tested_comparisons="$(
  awk -F, '
    NR == 1 {
      for (i = 1; i <= NF; i++) {
        if ($i == "tested") tested_column = i
      }
      next
    }
    tested_column > 0 && $tested_column == "true" { count++ }
    END { print count + 0 }
  ' "$tmp_dir/comparisons.csv"
)"
comparison_count="$(($(wc -l < "$tmp_dir/comparisons.csv") - 1))"
descriptive_count="$(($(wc -l < "$tmp_dir/descriptive.csv") - 1))"
metric_count="$((descriptive_count / 4))"
raw_manifest_sha256="$(sha256sum "$RAW_MANIFEST" | awk '{print $1}')"
analysis_source_manifest_sha256="$(
  sha256sum "$tmp_dir/analysis-source-manifest.sha256" | awk '{print $1}'
)"

cat > "$tmp_dir/analysis-metadata.txt" <<EOF_META
revision_experimental=$REVISION
campana=$CAMPAIGN
runs_por_escenario=$RUNS
escenarios=4
runs_totales=$expected_count
metricas=$metric_count
comparaciones=$comparison_count
comparaciones_testeadas=$tested_comparisons
bootstrap_replicas=$BOOTSTRAP_REPLICAS
permutation_replicas=$PERMUTATION_REPLICAS
confidence_level=$CONFIDENCE_LEVEL
alpha=$ALPHA
analysis_seed=$ANALYSIS_SEED
raw_manifest_sha256=$raw_manifest_sha256
analysis_source_manifest_sha256=$analysis_source_manifest_sha256
quantiles=Hyndman-Fan type 7
mad=raw
stddev=sample
permutation_statistic=median_difference
permutation_alternative=two-sided
multiplicity_adjustment=Holm
EOF_META

artifact_files=(
  per-run.csv
  descriptive.csv
  effects.csv
  comparisons.csv
  analysis-metadata.txt
  analysis-source-manifest.sha256
)

(
  cd "$tmp_dir"
  for file in "${artifact_files[@]}"; do
    sha256sum "$file"
  done
) > "$tmp_dir/analysis-manifest.sha256"
artifact_files+=(analysis-manifest.sha256)

if [[ -d "$STATISTICS_DIR" ]]; then
  for file in "${artifact_files[@]}"; do
    if [[ ! -f "$STATISTICS_DIR/$file" ]]; then
      echo "Falta el artefacto existente: $STATISTICS_DIR/$file" >&2
      exit 1
    fi
    if ! cmp -s "$tmp_dir/$file" "$STATISTICS_DIR/$file"; then
      echo "La reproducción difiere en: $STATISTICS_DIR/$file" >&2
      exit 1
    fi
  done
  echo "Análisis H2 reproducido sin diferencias: $STATISTICS_DIR"
  exit 0
fi

mkdir -p "$STATISTICS_DIR"
for file in "${artifact_files[@]}"; do
  install -m 0644 "$tmp_dir/$file" "$STATISTICS_DIR/$file"
done

echo "Artefactos estadísticos H2 generados en: $STATISTICS_DIR"
