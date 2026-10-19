.PHONY: run build setup-db seed

run:
	go run ./cmd/api/main.go

build:
	go build -o bin/pulsemargin-api ./cmd/api/main.go

seed:
	python3 scripts/generate_csv_data.py
	go run ./cmd/seed/main.go

test:
	go test -v ./...
