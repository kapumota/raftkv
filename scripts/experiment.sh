#!/usr/bin/env bash
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
cd "$ROOT"

RUNS="${RUNS:-30}"
CAMPAIGN="${CAMPAIGN:-final}"

if ! [[ "$RUNS" =~ ^[1-9][0-9]*$ ]]; then
  echo "RUNS debe ser un entero mayor que cero" >&2
  exit 1
fi

if ! [[ "$CAMPAIGN" =~ ^[A-Za-z0-9._-]+$ ]]; then
  echo "CAMPAIGN solo admite letras, números, punto, guion y guion bajo" >&2
  exit 1
fi

if [[ -n "$(git status --porcelain --untracked-files=all)" ]]; then
  echo "make experiment exige un árbol Git limpio" >&2
  git status --short >&2
  exit 1
fi

REVISION="$(git rev-parse HEAD)"
SHORT_REVISION="$(git rev-parse --short=12 HEAD)"
RAW_DIR="experiments/raw/${REVISION}/${CAMPAIGN}"
PROCESSED_DIR="experiments/processed/${REVISION}/${CAMPAIGN}"

if [[ -e "$RAW_DIR" ]]; then
  echo "El directorio de resultados ya existe: $RAW_DIR" >&2
  echo "No se sobrescribe evidencia experimental existente." >&2
  exit 1
fi

make validate

mkdir -p "$RAW_DIR"

RUNS="$RUNS" \
OUT_DIR="$RAW_DIR" \
PROJECT="raftkv-g4-${SHORT_REVISION}-${CAMPAIGN}" \
./scripts/benchmark-failures.sh

RUNS="$RUNS" \
REVISION="$REVISION" \
CAMPAIGN="$CAMPAIGN" \
RAW_DIR="$RAW_DIR" \
PROCESSED_DIR="$PROCESSED_DIR" \
./scripts/reproduce.sh

echo
echo "Campaña G4 finalizada."
echo "Revisión Git: $REVISION"
echo "Campaña: $CAMPAIGN"
echo "Crudos: $RAW_DIR"
echo "Procesados: $PROCESSED_DIR"
