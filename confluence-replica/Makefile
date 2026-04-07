SHELL := /bin/bash

-include .env

CONFIG ?= config/config.yaml
PG_DATA_DIR ?= $(HOME)/.local/viki/confluence/postgres-data
POSTGRES_DB ?= confluence_replica
POSTGRES_USER ?= postgres
BIN_DIR ?= bin
PARENT_ID ?=
DATE ?= $(shell date +%F)

.PHONY: help env-init data-dir runtime-up runtime-up-no-ollama runtime-down \
	postgres-up postgres-down postgres-restart postgres-ps postgres-logs \
	db-migrate db-vector-check db-shell api bootstrap sync digest test \
	build build-api build-replica

help:
	@echo "Common targets:"
	@echo "  make env-init            # Copy .env.example -> .env if missing"
	@echo "  make runtime-up          # Start postgres + ollama + pull embedding model if needed"
	@echo "  make runtime-up-no-ollama# Start postgres only"
	@echo "  make runtime-down        # Stop compose services"
	@echo "  make postgres-up         # Start postgres container"
	@echo "  make postgres-down       # Stop postgres container"
	@echo "  make postgres-logs       # Tail postgres logs"
	@echo "  make db-migrate          # Apply migrations/001_init.sql"
	@echo "  make db-vector-check     # Verify pgvector extension"
	@echo "  make db-shell            # Open psql shell in postgres container"
	@echo "  make build               # Build both binaries into ./bin"
	@echo "  make api                 # Run API service"
	@echo "  make bootstrap PARENT_ID=<id>"
	@echo "  make sync PARENT_ID=<id>"
	@echo "  make digest [DATE=YYYY-MM-DD]"
	@echo "  make test                # Run go test ./..."

env-init:
	@if [[ ! -f .env ]]; then cp .env.example .env; fi
	@echo ".env ready"

data-dir:
	@mkdir -p "$(PG_DATA_DIR)"
	@echo "data dir: $(PG_DATA_DIR)"

runtime-up: env-init data-dir
	@./scripts/bootstrap-runtime.sh --config "$(CONFIG)"

runtime-up-no-ollama: env-init data-dir
	@./scripts/bootstrap-runtime.sh --config "$(CONFIG)" --no-ollama

runtime-down:
	@docker compose down

postgres-up: data-dir
	@docker compose up -d postgres

postgres-down:
	@docker compose stop postgres

postgres-restart:
	@docker compose restart postgres

postgres-ps:
	@docker compose ps

postgres-logs:
	@docker compose logs -f postgres

db-migrate:
	@for f in $$(ls migrations/*.sql | sort); do \
		echo "applying $$f"; \
		cat "$$f" | docker compose exec -T postgres psql -v ON_ERROR_STOP=1 -U "$(POSTGRES_USER)" -d "$(POSTGRES_DB)"; \
	done

db-vector-check:
	@docker compose exec -T postgres psql -U "$(POSTGRES_USER)" -d "$(POSTGRES_DB)" -c "SELECT extname FROM pg_extension WHERE extname='vector';"

db-shell:
	@docker compose exec postgres psql -U "$(POSTGRES_USER)" -d "$(POSTGRES_DB)"

api:
	@CONF_REPLICA_CONFIG="$(CONFIG)" go run ./cmd/api

build: build-api build-replica

build-api:
	@mkdir -p "$(BIN_DIR)"
	@go build -o "$(BIN_DIR)/api" ./cmd/api
	@echo "built $(BIN_DIR)/api"

build-replica:
	@mkdir -p "$(BIN_DIR)"
	@go build -o "$(BIN_DIR)/replica" ./cmd/replica
	@echo "built $(BIN_DIR)/replica"

bootstrap:
	@if [[ -z "$(PARENT_ID)" ]]; then echo "PARENT_ID is required"; exit 1; fi
	@go run ./cmd/replica bootstrap --config "$(CONFIG)" --parent-id "$(PARENT_ID)"

sync:
	@if [[ -z "$(PARENT_ID)" ]]; then echo "PARENT_ID is required"; exit 1; fi
	@go run ./cmd/replica sync --config "$(CONFIG)" --parent-id "$(PARENT_ID)"

digest:
	@go run ./cmd/replica digest --config "$(CONFIG)" --date "$(DATE)"

test:
	@go test ./...
