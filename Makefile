.PHONY: fmt fmt-check vet test test-race build validate up down logs

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

validate: fmt-check vet test-race build

up:
	docker compose up --build

down:
	docker compose down

logs:
	docker compose logs -f
