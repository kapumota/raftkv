.PHONY: fmt fmt-check vet test test-race build scripts-check validate up down logs experiment reproduce

RUNS ?= 30
REVISION ?=
CAMPAIGN ?= final

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
	bash -n scripts/benchmark-failures.sh scripts/experiment.sh scripts/reproduce.sh

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
