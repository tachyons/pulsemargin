.PHONY: run build setup-db seed

SEED_ROWS ?= 1000000

run:
	go run ./cmd/api

build:
	go build -o bin/pulsemargin-api ./cmd/api

seed:
	python3 scripts/generate_csv_data.py --rows $(SEED_ROWS)
	go run ./cmd/seed

test:
	go test -v ./...
