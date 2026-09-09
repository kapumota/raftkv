#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

RUNS="${RUNS:-30}"
REVISION="${REVISION:-$(git rev-parse HEAD)}"
CAMPAIGN="${CAMPAIGN:-final}"
RAW_DIR="${RAW_DIR:-experiments/raw/${REVISION}/${CAMPAIGN}}"
PROCESSED_DIR="${PROCESSED_DIR:-experiments/processed/${REVISION}/${CAMPAIGN}}"

if ! [[ "$RUNS" =~ ^[1-9][0-9]*$ ]]; then
  echo "RUNS debe ser un entero mayor que cero" >&2
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

source_changes="$(
  git diff --name-only "$REVISION"..HEAD -- \
    . \
    ':(exclude)experiments/raw/**' \
    ':(exclude)experiments/processed/**'
)"
if [[ -n "$source_changes" ]]; then
  echo "El código o la configuración cambiaron desde la revisión experimental." >&2
  printf '%s\n' "$source_changes" >&2
  echo "Reproduzca desde una rama cuyo único cambio posterior sea evidencia raw/processed." >&2
  exit 1
fi

dirty="$(
  git status --porcelain --untracked-files=all |
    grep -vE '^[ MARCUD?!]{2} experiments/(raw|processed)/' || true
)"
if [[ -n "$dirty" ]]; then
  echo "make reproduce exige código y configuración sin cambios." >&2
  printf '%s\n' "$dirty" >&2
  exit 1
fi

if [[ ! -d "$RAW_DIR" ]]; then
  echo "No existe el directorio de datos crudos: $RAW_DIR" >&2
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

mkdir -p "$PROCESSED_DIR"

tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

processor="$tmp_dir/raftkv-failures"
go build -o "$processor" ./cmd/benchmark-failures

"$processor" \
  -dir "$RAW_DIR" \
  -runs "$RUNS" \
  > "$tmp_dir/summary.txt"

(
  cd "$RAW_DIR"
  for file in $(printf '%s\n' *.json | LC_ALL=C sort); do
    sha256sum "$file"
  done
) > "$tmp_dir/raw-manifest.sha256"

(
  cd experiments/configs
  for file in \
    normal-faults-5.yaml \
    follower-down-5.yaml \
    leader-down-5.yaml \
    leader-partition-5.yaml
  do
    sha256sum "$file"
  done
) > "$tmp_dir/config-manifest.sha256"

cat > "$tmp_dir/metadata.txt" <<EOF
revision_git=$REVISION
campana=$CAMPAIGN
runs_por_escenario=$RUNS
escenarios=4
resultados_esperados=$expected_count
EOF

install -m 0644 "$tmp_dir/summary.txt" "$PROCESSED_DIR/summary.txt"
install -m 0644 "$tmp_dir/raw-manifest.sha256" "$PROCESSED_DIR/raw-manifest.sha256"
install -m 0644 "$tmp_dir/config-manifest.sha256" "$PROCESSED_DIR/config-manifest.sha256"
install -m 0644 "$tmp_dir/metadata.txt" "$PROCESSED_DIR/metadata.txt"

echo "Resultados reproducidos en: $PROCESSED_DIR"
