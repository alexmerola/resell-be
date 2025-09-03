# Makefile for Resell Inventory Management System

# Load environment variables from .env file if it exists
ifneq (,$(wildcard ./.env))
    include .env
    export
endif

# Variables
APP_NAME := resell-api
WORKER_NAME := resell-worker
SEEDER_NAME := resell-seeder
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date -u '+%Y-%m-%d_%H:%M:%S')
GO_VERSION := $(shell go version | cut -d' ' -f3)
LDFLAGS := -ldflags "-X main.Version=$(VERSION) -X main.BuildTime=$(BUILD_TIME) -X main.GoVersion=$(GO_VERSION) -w -s"

# Go commands
GOCMD := go
GOBUILD := $(GOCMD) build
GOTEST := $(GOCMD) test
GOGET := $(GOCMD) get
GOMOD := $(GOCMD) mod
GOFMT := gofmt
GOLINT := golangci-lint

# Directories
CMD_DIR := ./cmd
INTERNAL_DIR := ./internal
MIGRATIONS_DIR := ./migrations
BUILD_DIR := ./build
BIN_DIR := ./bin

# Database - Use .env values if available, otherwise defaults
DB_HOST ?= localhost
DB_PORT ?= 5432
DB_USER ?= resell
DB_PASSWORD ?= resell_dev_2025
DB_NAME ?= resell_inventory
DB_SSL_MODE ?= disable
DATABASE_URL := postgresql://$(DB_USER):$(DB_PASSWORD)@$(DB_HOST):$(DB_PORT)/$(DB_NAME)?sslmode=$(DB_SSL_MODE)

# Redis
REDIS_HOST ?= localhost
REDIS_PORT ?= 6379

# Docker Compose project name (used for volume names)
COMPOSE_PROJECT_NAME := $(shell basename $(PWD))

# Colors for output
RED := \033[0;31m
GREEN := \033[0;32m
YELLOW := \033[1;33m
NC := \033[0m # No Color

.PHONY: all build clean test help

## help: Display this help message
help:
	@echo "$(GREEN)Available targets:$(NC)"
	@awk 'BEGIN {FS = ":.*##"} /^[a-zA-Z_-]+:.*?##/ { printf "  $(YELLOW)%-20s$(NC) %s\n", $$1, $$2 } /^##@/ { printf "\n$(GREEN)%s$(NC)\n", substr($$0, 5) }' $(MAKEFILE_LIST)

##@ Setup & Installation

## setup: Complete project setup
setup: check-env install-tools mod-download docker-up wait-for-db migrate-up
	@echo "$(GREEN)✓ Project setup complete!$(NC)"
	@echo "$(YELLOW)Next steps:$(NC)"
	@echo "  1. Review .env configuration"
	@echo "  2. Run 'make dev' to start development server"
	@echo "  3. Access API at http://localhost:8080"

## check-env: Check if .env file exists, create from example if not
check-env:
	@if [ ! -f .env ]; then \
		echo "$(YELLOW)Creating .env from .env.example...$(NC)"; \
		cp .env.example .env; \
		echo "$(GREEN)✓ .env file created. Please review and modify as needed.$(NC)"; \
	else \
		echo "$(GREEN)✓ .env file already exists$(NC)"; \
	fi

##@ Development

## dev: Run application with hot reload using Air
dev: check-air check-env
	@echo "$(GREEN)Starting development server with hot reload...$(NC)"
	@air

## run: Run the API server directly
run: check-env
	@echo "$(GREEN)Running API server...$(NC)"
	@go run $(LDFLAGS) $(CMD_DIR)/api/main.go

## run-worker: Run the async worker
run-worker: check-env
	@echo "$(GREEN)Running async worker...$(NC)"
	@go run $(LDFLAGS) $(CMD_DIR)/worker/main.go

## run-seeder: Run the database seeder
run-seeder: check-env
	@echo "$(GREEN)Running database seeder...$(NC)"
	@go run $(CMD_DIR)/seeder/main.go \
		-invoices=./invoices \
		-auctions=./auctions.xlsx \
		-log-level=info

## build: Build all binaries
build: build-api build-worker build-seeder

## build-api: Build the API server binary
build-api:
	@echo "$(GREEN)Building API server...$(NC)"
	@mkdir -p $(BIN_DIR)
	@$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(APP_NAME) $(CMD_DIR)/api/main.go
	@echo "$(GREEN)✓ API server built: $(BIN_DIR)/$(APP_NAME)$(NC)"

## build-worker: Build the worker binary
build-worker:
	@echo "$(GREEN)Building worker...$(NC)"
	@mkdir -p $(BIN_DIR)
	@$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(WORKER_NAME) $(CMD_DIR)/worker/main.go
	@echo "$(GREEN)✓ Worker built: $(BIN_DIR)/$(WORKER_NAME)$(NC)"

## build-seeder: Build the seeder binary
build-seeder:
	@echo "$(GREEN)Building seeder...$(NC)"
	@mkdir -p $(BIN_DIR)
	@$(GOBUILD) $(LDFLAGS) -o $(BIN_DIR)/$(SEEDER_NAME) $(CMD_DIR)/seeder/main.go
	@echo "$(GREEN)✓ Seeder built: $(BIN_DIR)/$(SEEDER_NAME)$(NC)"

## clean: Remove build artifacts
clean:
	@echo "$(RED)Cleaning build artifacts...$(NC)"
	@rm -rf $(BIN_DIR)
	@rm -f coverage.out coverage.html
	@echo "$(GREEN)✓ Cleaned$(NC)"

##@ Testing

## test: Run all tests
test:
	@echo "$(GREEN)Running tests...$(NC)"
	@$(GOTEST) -v -race -coverprofile=coverage.out ./...

## test-unit: Run unit tests only
test-unit:
	@echo "$(GREEN)Running unit tests...$(NC)"
	@$(GOTEST) -v -short -race ./...

## test-integration: Run integration tests
test-integration:
	@echo "$(GREEN)Running integration tests...$(NC)"
	@$(GOTEST) -v -race -run Integration ./test/integration/...

## test-coverage: Generate test coverage report
test-coverage: test
	@echo "$(GREEN)Generating coverage report...$(NC)"
	@go tool cover -html=coverage.out -o coverage.html
	@echo "$(GREEN)✓ Coverage report generated: coverage.html$(NC)"

## bench: Run benchmarks
bench:
	@echo "$(GREEN)Running benchmarks...$(NC)"
	@$(GOTEST) -bench=. -benchmem ./...

##@ Code Quality

## lint: Run golangci-lint
lint: check-lint
	@echo "$(GREEN)Running linter...$(NC)"
	@$(GOLINT) run ./...

## fmt: Format code using gofmt
fmt:
	@echo "$(GREEN)Formatting code...$(NC)"
	@$(GOFMT) -w -s .
	@echo "$(GREEN)✓ Code formatted$(NC)"

## vet: Run go vet
vet:
	@echo "$(GREEN)Running go vet...$(NC)"
	@go vet ./...
	@echo "$(GREEN)✓ Vet passed$(NC)"

## mod-tidy: Tidy go modules
mod-tidy:
	@echo "$(GREEN)Tidying go modules...$(NC)"
	@$(GOMOD) tidy
	@echo "$(GREEN)✓ Modules tidied$(NC)"

## mod-download: Download go modules
mod-download:
	@echo "$(GREEN)Downloading go modules...$(NC)"
	@$(GOMOD) download
	@echo "$(GREEN)✓ Modules downloaded$(NC)"

##@ Database

## wait-for-db: Wait for database to be ready
wait-for-db:
	@echo "$(YELLOW)Waiting for database to be ready...$(NC)"
	@timeout=60; \
	while [ $$timeout -gt 0 ]; do \
		if docker-compose exec -T postgres pg_isready -U $(DB_USER) -d $(DB_NAME) >/dev/null 2>&1; then \
			echo "$(GREEN)✓ Database is ready$(NC)"; \
			break; \
		fi; \
		echo "Waiting for database... ($$timeout seconds remaining)"; \
		sleep 2; \
		timeout=$$((timeout-2)); \
	done; \
	if [ $$timeout -le 0 ]; then \
		echo "$(RED)✗ Database failed to become ready$(NC)"; \
		exit 1; \
	fi

## migrate-up: Run database migrations up
migrate-up: check-migrate check-env
	@echo "$(GREEN)Running migrations up...$(NC)"
	@echo "Using DATABASE_URL: $(DATABASE_URL)"
	@migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" up
	@echo "$(GREEN)✓ Migrations applied$(NC)"

## migrate-down: Rollback last migration
migrate-down: check-migrate check-env
	@echo "$(YELLOW)Rolling back last migration...$(NC)"
	@migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" down 1
	@echo "$(GREEN)✓ Migration rolled back$(NC)"

## migrate-create: Create a new migration (usage: make migrate-create NAME=migration_name)
migrate-create: check-migrate
ifndef NAME
	@echo "$(RED)Error: NAME is required. Usage: make migrate-create NAME=migration_name$(NC)"
	@exit 1
endif
	@echo "$(GREEN)Creating migration: $(NAME)...$(NC)"
	@migrate create -ext sql -dir $(MIGRATIONS_DIR) -seq $(NAME)
	@echo "$(GREEN)✓ Migration created$(NC)"

## migrate-force: Force set migration version (usage: make migrate-force VERSION=1)
migrate-force: check-migrate check-env
ifndef VERSION
	@echo "$(RED)Error: VERSION is required. Usage: make migrate-force VERSION=1$(NC)"
	@exit 1
endif
	@echo "$(YELLOW)Forcing migration version to $(VERSION)...$(NC)"
	@migrate -path $(MIGRATIONS_DIR) -database "$(DATABASE_URL)" force $(VERSION)
	@echo "$(GREEN)✓ Migration version forced$(NC)"

## db-reset: Reset database (drop, create, migrate)
db-reset: docker-db-reset wait-for-db migrate-up
	@echo "$(GREEN)✓ Database reset complete$(NC)"

## seed-db: Seed database from Excel file
seed-db: check-seeder-deps check-env
ifdef FILE
	@echo "$(GREEN)Seeding database from $(FILE)...$(NC)"
	@go run $(CMD_DIR)/seeder/main.go -auctions=$(FILE) -log-level=info
else
	@echo "$(GREEN)Seeding database with default data...$(NC)"
	@go run $(CMD_DIR)/seeder/main.go -log-level=info
endif

##@ Docker

## docker-build: Build Docker images
docker-build:
	@echo "$(GREEN)Building Docker images...$(NC)"
	@docker-compose build
	@echo "$(GREEN)✓ Docker images built$(NC)"

## docker-up: Start all services with Docker Compose
docker-up:
	@echo "$(GREEN)Starting Docker services...$(NC)"
	@docker-compose up -d
	@echo "$(GREEN)✓ Services started$(NC)"
	@echo "$(YELLOW)Waiting for services to be ready...$(NC)"
	@sleep 5
	@docker-compose ps

## docker-down: Stop all Docker services
docker-down:
	@echo "$(YELLOW)Stopping Docker services...$(NC)"
	@docker-compose down
	@echo "$(GREEN)✓ Services stopped$(NC)"

## docker-logs: View Docker logs
docker-logs:
	@docker-compose logs -f

## docker-clean: Stop and remove all containers, networks, volumes
docker-clean:
	@echo "$(RED)Cleaning Docker environment...$(NC)"
	@docker-compose down -v --remove-orphans
	@echo "$(GREEN)✓ Docker environment cleaned$(NC)"

## docker-db-reset: Reset the database container
docker-db-reset:
	@echo "$(YELLOW)Resetting database container...$(NC)"
	@docker-compose stop postgres
	@docker-compose rm -f postgres
	@docker volume rm $(COMPOSE_PROJECT_NAME)_postgres_data 2>/dev/null || true
	@docker-compose up -d postgres
	@echo "$(YELLOW)Waiting for PostgreSQL to be ready...$(NC)"
	@sleep 10
	@echo "$(GREEN)✓ Database container reset$(NC)"

## docker-redis-cli: Connect to Redis CLI
docker-redis-cli:
	@docker-compose exec redis redis-cli

## docker-psql: Connect to PostgreSQL
docker-psql:
	@docker-compose exec postgres psql -U $(DB_USER) -d $(DB_NAME)

##@ Async Jobs (Asynq)

## asynq-dash: Open Asynq monitoring dashboard
asynq-dash:
	@echo "$(GREEN)Starting Asynq dashboard on http://localhost:8081$(NC)"
	@docker run --rm \
		--name asynqmon \
		-p 8081:8080 \
		hibiken/asynqmon:latest \
		--redis-addr=$(REDIS_HOST):$(REDIS_PORT)

## asynq-stats: View queue statistics
asynq-stats:
	@echo "$(GREEN)Asynq Queue Statistics:$(NC)"
	@asynq stats -u redis://$(REDIS_HOST):$(REDIS_PORT)

##@ Import/Export

## import-pdf: Import PDFs from directory (usage: make import-pdf DIR=./invoices)
import-pdf: build-seeder
ifdef DIR
	@echo "$(GREEN)Importing PDFs from $(DIR)...$(NC)"
	@$(BIN_DIR)/$(SEEDER_NAME) -invoices=$(DIR) -log-level=info
else
	@echo "$(RED)Error: DIR is required. Usage: make import-pdf DIR=./invoices$(NC)"
	@exit 1
endif

## import-excel: Import Excel file (usage: make import-excel FILE=./data.xlsx)
import-excel: build-seeder
ifdef FILE
	@echo "$(GREEN)Importing Excel file $(FILE)...$(NC)"
	@$(BIN_DIR)/$(SEEDER_NAME) -auctions=$(FILE) -log-level=info
else
	@echo "$(RED)Error: FILE is required. Usage: make import-excel FILE=./data.xlsx$(NC)"
	@exit 1
endif

## export-excel: Export inventory to Excel
export-excel: build-api
	@echo "$(GREEN)Exporting to Excel...$(NC)"
	@curl -o inventory_export.xlsx http://localhost:8080/api/v1/export/excel
	@echo "$(GREEN)✓ Exported to inventory_export.xlsx$(NC)"

## export-json: Export inventory to JSON
export-json: build-api
	@echo "$(GREEN)Exporting to JSON...$(NC)"
	@curl -o inventory_export.json http://localhost:8080/api/v1/export/json
	@echo "$(GREEN)✓ Exported to inventory_export.json$(NC)"

## install-tools: Install required development tools
install-tools:
	@echo "$(GREEN)Installing development tools...$(NC)"
	@go install github.com/air-verse/air@latest
	@go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install github.com/hibiken/asynq/tools/asynq@latest
	@echo "$(GREEN)✓ Tools installed$(NC)"

##@ Deployment

## deploy-staging: Deploy to staging environment
deploy-staging: test build
	@echo "$(GREEN)Deploying to staging...$(NC)"
	# Add your staging deployment commands here
	@echo "$(GREEN)✓ Deployed to staging$(NC)"

## deploy-prod: Deploy to production
deploy-prod: test build
	@echo "$(RED)Deploying to PRODUCTION...$(NC)"
	@echo "$(YELLOW)Are you sure? [y/N]$(NC)"
	@read -r response; \
	if [ "$$response" = "y" ]; then \
		echo "$(GREEN)Deploying to production...$(NC)"; \
		# Add your production deployment commands here
		echo "$(GREEN)✓ Deployed to production$(NC)"; \
	else \
		echo "$(YELLOW)Deployment cancelled$(NC)"; \
	fi

##@ Utilities

## check-air: Check if Air is installed
check-air:
	@command -v air >/dev/null 2>&1 || { \
		echo "$(RED)Air is not installed. Installing...$(NC)"; \
		go install github.com/air-verse/air@latest; \
	}

## check-migrate: Check if migrate is installed
check-migrate:
	@command -v migrate >/dev/null 2>&1 || { \
		echo "$(RED)migrate is not installed. Installing...$(NC)"; \
		go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest; \
	}

## check-lint: Check if golangci-lint is installed
check-lint:
	@command -v golangci-lint >/dev/null 2>&1 || { \
		echo "$(RED)golangci-lint is not installed. Installing...$(NC)"; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	}

## check-seeder-deps: Check seeder dependencies
check-seeder-deps:
	@echo "$(GREEN)Checking seeder dependencies...$(NC)"

## version: Show version information
version:
	@echo "$(GREEN)Resell Inventory Management System$(NC)"
	@echo "Version: $(VERSION)"
	@echo "Go Version: $(GO_VERSION)"
	@echo "Build Time: $(BUILD_TIME)"

# Default target
all: help