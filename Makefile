.PHONY: help build up down restart logs shell db-shell migrate clean build-agent test-agent

help:
	@echo "Available commands:"
	@echo "  make build       - Build Docker images"
	@echo "  make up          - Start all services"
	@echo "  make down        - Stop all services"
	@echo "  make restart     - Restart all services"
	@echo "  make logs        - View logs from all services"
	@echo "  make api-logs    - View API logs"
	@echo "  make shell       - Open shell in API container"
	@echo "  make db-shell    - Open PostgreSQL shell"
	@echo "  make migrate     - Run database migrations"
	@echo "  make clean       - Remove volumes and containers"
	@echo "  make scan        - Trigger manual snapshot scan"
	@echo "  make build-agent - Build the snapshot daemon binary"
	@echo "  make test-agent  - Run agent tests"

build:
	docker-compose build

up:
	docker-compose up -d

down:
	docker-compose down

restart:
	docker-compose restart

logs:
	docker-compose logs -f

api-logs:
	docker-compose logs -f api

shell:
	docker-compose exec api /bin/bash

db-shell:
	docker-compose exec postgres psql -U chainsnaps -d chainsnaps

migrate:
	docker-compose exec api alembic upgrade head

clean:
	docker-compose down -v

scan:
	@echo "Triggering manual snapshot scan..."
	@API_KEY=$$(grep API_KEYS .env | cut -d '=' -f2 | cut -d ',' -f1); \
	curl -X POST http://localhost:8000/api/v1/snapshots/scan \
		-H "X-API-Key: $$API_KEY" \
		-H "Content-Type: application/json"

build-agent:
	@echo "Building snapshot daemon..."
	cd agent/cmd/snapperd && go generate
	cd agent && go build -o bin/snapperd ./cmd/snapperd
	@echo "Binary built at agent/bin/snapperd"

test-agent:
	@echo "Running agent tests..."
	cd agent && go test -v ./...
