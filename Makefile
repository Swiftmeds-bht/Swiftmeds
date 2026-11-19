SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE     := docker compose -f deploy/docker-compose.yml --env-file .env
# Seconds `make up` waits for every container to report healthy. A first
# build on a clean machine downloads and compiles all 20 images.
UP_TIMEOUT  ?= 900
# Every Go module listed in go.work
GO_MODULES   = $(shell go list -m -f '{{.Dir}}')

.PHONY: help proto proto-check openapi-lint up down seed smoke e2e test lint coverage mutation

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

proto: ## Generate Go code from proto/**/*.proto into proto/gen/go
	@./scripts/gen-proto.sh

proto-check: ## Fail if generated Go protobuf code is out of date
	@bash ./scripts/check-proto-drift.sh

openapi-lint: ## Lint docs/api/openapi.yaml
	@npx --yes @redocly/cli@1.34.5 lint docs/api/openapi.yaml

up: ## Build and start all 20 services, wait until every one is healthy, then load the demo data
	@test -f .env || { echo "Missing .env: run 'cp .env.example .env' first."; exit 1; }
	$(COMPOSE) up -d --build --wait --wait-timeout $(UP_TIMEOUT)
	$(COMPOSE) --profile seed run --rm seed
	@$(COMPOSE) ps --format 'table {{.Service}}\t{{.Status}}'

down: ## Stop and remove all containers
	$(COMPOSE) down

seed: ## Load the demo data again (safe to repeat); starts what it needs first
	@test -f .env || { echo "Missing .env: run 'cp .env.example .env' first."; exit 1; }
	$(COMPOSE) --profile seed run --rm seed

smoke: ## Search, reserve and collect through the gateway, against a running `make up` stack
	@./scripts/smoke-test.sh

e2e: ## Cypress browser tests (tests/e2e): start the stack with short holds, seed it, run headless in Docker
	@./tests/e2e/run.sh --start-stack

test: ## Run unit tests in every Go module in go.work
	@for d in $(GO_MODULES); do \
		if [ -z "$$(cd $$d && go list ./... 2>/dev/null)" ]; then echo "==> $$d (no packages yet, skipped)"; continue; fi; \
		echo "==> $$d"; \
		(cd $$d && go test -race -cover ./...) || exit 1; \
	done

coverage: ## Fail if any Go service's domain+app coverage is below 65%
	@./scripts/coverage-gate.sh

mutation: ## Run Gremlins on the Ranking, Inventory and Holds domain/app packages
	@./scripts/mutation-tests.sh

lint: ## Run golangci-lint in every Go module in go.work
	@for d in $(GO_MODULES); do \
		if [ -z "$$(cd $$d && go list ./... 2>/dev/null)" ]; then echo "==> $$d (no packages yet, skipped)"; continue; fi; \
		echo "==> $$d"; \
		(cd $$d && golangci-lint run ./...) || exit 1; \
	done
