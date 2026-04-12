SHELL := /bin/bash

-include .env

CONFIG ?= config/config.yaml
BIN_DIR ?= bin
GO ?= go
SQLITE_TAGS ?= sqlite_fts5
INTEGRATION_TAGS ?= integration $(SQLITE_TAGS)
PARENT_ID ?=
PARENT_IDS ?=
DATE ?= $(shell date +%F)
KEYCHAIN_SERVICE ?= codex_confluence_pat
KEYCHAIN_ACCOUNT ?=
VERSION ?= $(shell git describe --tags --match 'confluence-replica/v*' --abbrev=0 2>/dev/null || echo dev)
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
BUILD_DATE ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS ?= -X confluence-replica/internal/version.Version=$(VERSION) -X confluence-replica/internal/version.Commit=$(COMMIT) -X confluence-replica/internal/version.BuildDate=$(BUILD_DATE)
GO_RUN = CGO_ENABLED=1 $(GO) run -tags "$(SQLITE_TAGS)"
GO_BUILD = CGO_ENABLED=1 $(GO) build -tags "$(SQLITE_TAGS)"
GO_TEST = CGO_ENABLED=1 $(GO) test -tags "$(SQLITE_TAGS)"
GO_TEST_INTEGRATION = CGO_ENABLED=1 $(GO) test -tags "$(INTEGRATION_TAGS)"

.PHONY: help env-init toolchain-check clean api mcp bootstrap sync rebuild update digest \
	test build build-api build-replica build-mcp mcp-integration update-token

help:
	@echo "Common targets:"
	@echo "  make env-init            # Copy .env.example -> .env if missing"
	@echo "  make toolchain-check     # Verify gcc is available for CGO sqlite builds"
	@echo "  make test                # Run go test with CGO_ENABLED=1 and sqlite_fts5"
	@echo "  make build               # Build api, replica, and mcp into ./bin"
	@echo "  make api                 # Run the HTTP API"
	@echo "  make mcp                 # Run the MCP stdio server"
	@echo "  make bootstrap           # Initial import from configured confluence.parent_ids"
	@echo "  make sync                # Incremental sync from configured roots"
	@echo "  make rebuild            # Delete sqlite db artifacts and rebuild from Confluence"
	@echo "  make digest [DATE=YYYY-MM-DD]"
	@echo "  make mcp-integration     # Build MCP binary and run the smoke test"
	@echo "  make update-token        # Interactively update Confluence PAT in macOS Keychain"

env-init:
	@if [[ ! -f .env ]]; then cp .env.example .env; fi
	@echo ".env ready"

toolchain-check:
	@command -v gcc >/dev/null || { echo "gcc is required because mattn/go-sqlite3 uses CGO"; exit 1; }

clean:
	@rm -rf "$(BIN_DIR)"

api: toolchain-check
	@CONF_REPLICA_CONFIG="$(CONFIG)" $(GO_RUN) ./cmd/api --config "$(CONFIG)"

mcp: toolchain-check
	@CONF_REPLICA_CONFIG="$(CONFIG)" $(GO_RUN) ./cmd/mcp --config "$(CONFIG)"

build: toolchain-check build-api build-replica build-mcp

build-api:
	@mkdir -p "$(BIN_DIR)"
	@$(GO_BUILD) -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/api" ./cmd/api
	@echo "built $(BIN_DIR)/api"

build-replica:
	@mkdir -p "$(BIN_DIR)"
	@$(GO_BUILD) -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/replica" ./cmd/replica
	@echo "built $(BIN_DIR)/replica"

build-mcp:
	@mkdir -p "$(BIN_DIR)"
	@$(GO_BUILD) -ldflags "$(LDFLAGS)" -o "$(BIN_DIR)/mcp" ./cmd/mcp
	@echo "built $(BIN_DIR)/mcp"

bootstrap: toolchain-check
	@args=(--config "$(CONFIG)"); \
	if [[ -n "$(PARENT_IDS)" ]]; then args+=(--parent-ids "$(PARENT_IDS)"); fi; \
	if [[ -n "$(PARENT_ID)" ]]; then args+=(--parent-id "$(PARENT_ID)"); fi; \
	CONF_REPLICA_CONFIG="$(CONFIG)" $(GO_RUN) ./cmd/replica bootstrap "$${args[@]}"

sync: toolchain-check
	@args=(--config "$(CONFIG)"); \
	if [[ -n "$(PARENT_IDS)" ]]; then args+=(--parent-ids "$(PARENT_IDS)"); fi; \
	if [[ -n "$(PARENT_ID)" ]]; then args+=(--parent-id "$(PARENT_ID)"); fi; \
	CONF_REPLICA_CONFIG="$(CONFIG)" $(GO_RUN) ./cmd/replica sync "$${args[@]}"

rebuild: toolchain-check
	@args=(--config "$(CONFIG)"); \
	if [[ -n "$(PARENT_IDS)" ]]; then args+=(--parent-ids "$(PARENT_IDS)"); fi; \
	if [[ -n "$(PARENT_ID)" ]]; then args+=(--parent-id "$(PARENT_ID)"); fi; \
	CONF_REPLICA_CONFIG="$(CONFIG)" $(GO_RUN) ./cmd/replica rebuild "$${args[@]}"

update: sync

digest: toolchain-check
	@CONF_REPLICA_CONFIG="$(CONFIG)" $(GO_RUN) ./cmd/replica digest --config "$(CONFIG)" --date "$(DATE)"

test: toolchain-check
	@$(GO_TEST) ./...

mcp-integration: build-mcp
	@MCP_BINARY="$(abspath $(BIN_DIR))/mcp" $(GO_TEST_INTEGRATION) ./integration -run TestMCPBinarySmoke -count=1

update-token:
	@service="$(KEYCHAIN_SERVICE)"; \
	account="$(KEYCHAIN_ACCOUNT)"; \
	default_account="$$(whoami)"; \
	if [[ -z "$$service" ]]; then \
		echo "KEYCHAIN_SERVICE is required"; \
		exit 1; \
	fi; \
	if [[ -z "$$account" ]]; then \
		read -r -p "Keychain account [$$default_account]: " input_account; \
		account="$${input_account:-$$default_account}"; \
	fi; \
	read -r -s -p "Enter new Confluence PAT: " token; \
	echo; \
	if [[ -z "$$token" ]]; then \
		echo "PAT cannot be empty"; \
		exit 1; \
	fi; \
	security add-generic-password -U -s "$$service" -a "$$account" -w "$$token"; \
	echo "Token updated: keychain://$$service?account=$$account"
