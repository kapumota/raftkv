#!/usr/bin/env bash
set -euo pipefail

RUNS="${RUNS:-30}"
OUT_DIR="${OUT_DIR:-/tmp/raftkv-g3}"
PROJECT="${PROJECT:-raftkv-g3}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.benchmarks-5.yml}"

if ! [[ "$RUNS" =~ ^[1-9][0-9]*$ ]]; then
  echo "RUNS debe ser un entero mayor que cero" >&2
  exit 1
fi

mkdir -p "$OUT_DIR"

go build -o /tmp/raftkv-experiment ./cmd/experiment-runner
go build -o /tmp/raftkv-failures ./cmd/benchmark-failures

scenarios=(
  normal-faults-5
  follower-down-5
  leader-down-5
  leader-partition-5
)

cleanup() {
  docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down -v >/dev/null 2>&1 || true
}

trap cleanup EXIT INT TERM

docker compose -p "$PROJECT" -f "$COMPOSE_FILE" build

for scenario in "${scenarios[@]}"; do
  for ((run = 1; run <= RUNS; run++)); do
    printf -v suffix "%03d" "$run"
    output="$OUT_DIR/${scenario}-${suffix}.json"

    if [[ -e "$output" ]]; then
      echo "El resultado ya existe: $output" >&2
      exit 1
    fi

    echo "Escenario $scenario, ejecución $run/$RUNS"
    cleanup
    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" up -d --no-build

    /tmp/raftkv-experiment \
      -scenario "experiments/configs/${scenario}.yaml" \
      -output "$output"

    docker compose -p "$PROJECT" -f "$COMPOSE_FILE" down -v
  done
done

/tmp/raftkv-failures -dir "$OUT_DIR" -runs "$RUNS"

trap - EXIT INT TERM
cleanup
