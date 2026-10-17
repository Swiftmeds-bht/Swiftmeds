SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE     := docker compose -f deploy/docker-compose.yml --env-file .env
# Every Go module listed in go.work
GO_MODULES   = $(shell go list -m -f '{{.Dir}}')

.PHONY: help proto up down test lint

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

proto: ## Generate Go code from proto/**/*.proto into proto/gen/go
	@./scripts/gen-proto.sh

up: ## Start all services, Postgres, RabbitMQ and Traefik
	@test -f .env || { echo "Missing .env: run 'cp .env.example .env' first."; exit 1; }
	$(COMPOSE) up -d --build

down: ## Stop and remove all containers
	$(COMPOSE) down

test: ## Run unit tests in every Go module in go.work
	@for d in $(GO_MODULES); do \
		if [ -z "$$(cd $$d && go list ./... 2>/dev/null)" ]; then echo "==> $$d (no packages yet, skipped)"; continue; fi; \
		echo "==> $$d"; \
		(cd $$d && go test -cover ./...) || exit 1; \
	done

lint: ## Run golangci-lint in every Go module in go.work
	@for d in $(GO_MODULES); do \
		if [ -z "$$(cd $$d && go list ./... 2>/dev/null)" ]; then echo "==> $$d (no packages yet, skipped)"; continue; fi; \
		echo "==> $$d"; \
		(cd $$d && golangci-lint run ./...) || exit 1; \
	done
