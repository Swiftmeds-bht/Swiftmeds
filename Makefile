SHELL := /bin/bash
.DEFAULT_GOAL := help

COMPOSE     := docker compose -f deploy/docker-compose.yml --env-file .env
PROTO_FILES := $(shell find proto -name '*.proto' -not -path 'proto/gen/*')
# Every Go module listed in go.work
GO_MODULES   = $(shell go list -m -f '{{.Dir}}')

.PHONY: help proto up down test lint

help: ## List available targets
	@awk 'BEGIN {FS = ":.*## "} /^[a-zA-Z_-]+:.*## / {printf "  \033[36m%-8s\033[0m %s\n", $$1, $$2}' $(MAKEFILE_LIST)

proto: ## Generate Go code from proto/**/*.proto into proto/gen/go
	@if [ -z "$(PROTO_FILES)" ]; then \
		echo "No .proto files yet."; \
	else \
		protoc -I proto \
			--go_out=proto/gen/go --go_opt=paths=source_relative \
			--go-grpc_out=proto/gen/go --go-grpc_opt=paths=source_relative \
			$(PROTO_FILES); \
	fi

up: ## Start all services, Postgres, RabbitMQ and Traefik
	@test -f .env || { echo "Missing .env: run 'cp .env.example .env' first."; exit 1; }
	$(COMPOSE) up -d --build

down: ## Stop and remove all containers
	$(COMPOSE) down

test: ## Run unit tests in every Go module in go.work
	@for d in $(GO_MODULES); do \
		echo "==> $$d"; \
		(cd $$d && go test -cover ./...) || exit 1; \
	done

lint: ## Run golangci-lint in every Go module in go.work
	@for d in $(GO_MODULES); do \
		echo "==> $$d"; \
		(cd $$d && golangci-lint run ./...) || exit 1; \
	done
