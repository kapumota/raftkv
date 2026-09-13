.PHONY: fmt fmt-check vet test test-race build scripts-check validate up down logs experiment reproduce analyze

RUNS ?= 30
REVISION ?=
CAMPAIGN ?= final
ANALYSIS_SEED ?= 20260912
BOOTSTRAP_REPLICAS ?= 100000
PERMUTATION_REPLICAS ?= 100000
CONFIDENCE_LEVEL ?= 0.95
ALPHA ?= 0.05

fmt:
	gofmt -w ./cmd ./internal

fmt-check:
	@test -z "$$(gofmt -l ./cmd ./internal)" || \
		(echo "Hay archivos Go sin formato. Ejecuta: make fmt"; exit 1)

vet:
	go vet ./...

test:
	go test ./...

test-race:
	go test -race ./...

build:
	go build ./...

scripts-check:
	bash -n scripts/benchmark-failures.sh scripts/experiment.sh scripts/reproduce.sh scripts/analyze.sh

validate: fmt-check vet test-race build scripts-check

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f

experiment:
	RUNS="$(RUNS)" CAMPAIGN="$(CAMPAIGN)" ./scripts/experiment.sh

reproduce:
	RUNS="$(RUNS)" REVISION="$(REVISION)" CAMPAIGN="$(CAMPAIGN)" ./scripts/reproduce.sh

analyze:
	RUNS="$(RUNS)" REVISION="$(REVISION)" CAMPAIGN="$(CAMPAIGN)" \
		ANALYSIS_SEED="$(ANALYSIS_SEED)" BOOTSTRAP_REPLICAS="$(BOOTSTRAP_REPLICAS)" \
		PERMUTATION_REPLICAS="$(PERMUTATION_REPLICAS)" CONFIDENCE_LEVEL="$(CONFIDENCE_LEVEL)" \
		ALPHA="$(ALPHA)" ./scripts/analyze.sh
