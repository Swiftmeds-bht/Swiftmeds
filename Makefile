SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE     := docker compose -f deploy/docker-compose.yml --env-file .env
# Every Go module listed in go.work
GO_MODULES   = $(shell go list -m -f '{{.Dir}}')

.PHONY: help proto up down seed test lint coverage

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

proto: ## Generate Go code from proto/**/*.proto into proto/gen/go
	@./scripts/gen-proto.sh

up: ## Start all services, Postgres, RabbitMQ and Traefik
	@test -f .env || { echo "Missing .env: run 'cp .env.example .env' first."; exit 1; }
	$(COMPOSE) up -d --build

down: ## Stop and remove all containers
	$(COMPOSE) down

seed: ## Start seed dependencies, wait for migrations, and load demo fixtures
	@test -f .env || { echo "Missing .env: run 'cp .env.example .env' first."; exit 1; }
	$(COMPOSE) up -d --build postgres rabbitmq catalog pharmacy-registry geo inventory pricing
	@set -e; \
	$(COMPOSE) exec -T postgres sh -c 'until pg_isready -U "$$POSTGRES_USER" -d postgres; do sleep 2; done'; \
	for spec in "catalog_db catalog_products" "pharmacy_registry_db pharmacies" "geo_db pharmacy_locations" "inventory_db inventory_stock" "pricing_db prices"; do \
		set -- $$spec; \
		echo "Waiting for $$1 migrations..."; \
		$(COMPOSE) exec -T postgres sh /opt/swiftmed/seed/wait-for-migration.sh "$$1" "$$2"; \
	done
	$(COMPOSE) exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d pharmacy_registry_db -f /opt/swiftmed/seed/pharmacy-registry.sql'
	$(COMPOSE) exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d geo_db -f /opt/swiftmed/seed/geo.sql'
	$(COMPOSE) exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d inventory_db -f /opt/swiftmed/seed/inventory.sql'
	$(COMPOSE) exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d pricing_db -f /opt/swiftmed/seed/pricing.sql'
	@echo "Demo seed loaded. Prices are synthetic and for development/testing only."

test: ## Run unit tests in every Go module in go.work
	@for d in $(GO_MODULES); do \
		if [ -z "$$(cd $$d && go list ./... 2>/dev/null)" ]; then echo "==> $$d (no packages yet, skipped)"; continue; fi; \
		echo "==> $$d"; \
		(cd $$d && go test -cover ./...) || exit 1; \
	done

coverage: ## Fail if a core service's domain+app coverage is below 80%
	@./scripts/coverage-gate.sh

lint: ## Run golangci-lint in every Go module in go.work
	@for d in $(GO_MODULES); do \
		if [ -z "$$(cd $$d && go list ./... 2>/dev/null)" ]; then echo "==> $$d (no packages yet, skipped)"; continue; fi; \
		echo "==> $$d"; \
		(cd $$d && golangci-lint run ./...) || exit 1; \
	done
