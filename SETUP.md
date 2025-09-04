Test Structure & Organization
resell-be/
├── test/
│   ├── fixtures/                 # Test data files
│   │   ├── sample_invoice.pdf
│   │   ├── sample_inventory.xlsx
│   │   ├── invalid.pdf
│   │   └── test_data.sql
│   ├── integration/              # Integration tests
│   │   ├── inventory_test.go
│   │   ├── import_test.go
│   │   ├── export_test.go
│   │   └── helpers_test.go
│   ├── e2e/                      # End-to-end tests
│   │   ├── api_workflow_test.go
│   │   └── pdf_import_flow_test.go
│   └── mocks/                    # Generated mocks
│       ├── mock_repository.go
│       ├── mock_service.go
│       └── mock_cache.go
├── internal/
│   ├── core/
│   │   ├── domain/
│   │   │   └── inventory_test.go
│   │   └── services/
│   │       └── inventory_service_test.go
│   ├── handlers/
│   │   ├── inventory_handler_test.go
│   │   ├── export_handler_test.go
│   │   ├── import_handler_test.go
│   │   └── dashboard_handler_test.go
│   ├── adapters/
│   │   └── db/
│   │       └── inventory_repository_test.go
│   └── workers/
│       ├── pdf_processor_test.go
│       └── excel_processor_test.go
└── Makefile                      # Updated with test targets
Test Dependencies
Add to go.mod:
gorequire (
    github.com/stretchr/testify v1.9.0
    go.uber.org/mock v0.4.0
    github.com/ory/dockertest/v3 v3.10.0
    github.com/DATA-DOG/go-sqlmock v1.5.2
    github.com/alicebob/miniredis/v2 v2.31.0
    github.com/jarcoal/httpmock v1.3.1
)
Testing Principles
1. Test Naming Convention

Test functions: Test<FunctionName>_<Scenario>
Example: TestSaveItem_WithValidData_Success

2. Table-Driven Tests

Use subtests for multiple scenarios
Clear test case names
Comprehensive edge case coverage

3. Test Organization

Arrange: Set up test data and dependencies
Act: Execute the function under test
Assert: Verify the results

4. Mocking Strategy

Mock external dependencies (DB, Redis, HTTP)
Use interfaces for dependency injection
Generate mocks with mockgen

5. Test Coverage Goals

Unit tests: 80%+ coverage
Integration tests: Critical paths
E2E tests: Main user workflows

# 🚀 Resell Inventory Management System - Setup Guide

## Quick Reference Card

```bash
# Most Common Commands
make setup          # Complete initial setup
make dev           # Start development server
make test          # Run tests
make docker-up     # Start Docker services
make docker-down   # Stop Docker services
make migrate-up    # Run migrations
make seed-db       # Seed database
make lint          # Run linter
make build         # Build binaries
make clean         # Clean build artifacts
```
---

## Prerequisites Installation

### 1. Install Required Software

```bash
# macOS (using Homebrew)
brew install go@1.22
brew install postgresql@16
brew install redis
brew install make
brew install git

# Ubuntu/Debian
sudo apt update
sudo apt install -y golang-1.22 postgresql-16 redis-server make git

# Windows (using Chocolatey)
choco install golang
choco install postgresql16
choco install redis-64
choco install make
choco install git
```

### 2. Install Docker & Docker Compose

```bash
# macOS
brew install --cask docker

# Ubuntu/Debian
curl -fsSL https://get.docker.com -o get-docker.sh
sudo sh get-docker.sh
sudo usermod -aG docker $USER

# Verify installation
docker --version
docker-compose --version
```

## Project Setup

### 1. Clone & Initialize Project

```bash
# Clone your repository (adjust URL as needed)
git clone https://github.com/ammerola/resell-be.git
cd resell-be

# Create required directories
mkdir -p {bin,tmp,uploads,invoices,exports,logs}
touch invoices/.gitkeep uploads/.gitkeep

# Copy environment configuration
cp .env.example .env

# Edit .env with your preferred editor
nano .env  # or vim, code, etc.
```

### 2. Initialize Go Module

```bash
# Initialize if not already done
go mod init github.com/ammerola/resell-be

# Download dependencies
go mod download
go mod verify
go mod tidy
```

### 3. Install Development Tools

```bash
# Install all required tools
make install-tools

# Or install manually:
go install github.com/air-verse/air@latest
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
go install github.com/hibiken/asynq/tools/asynq@latest

# Verify installations
air -v
migrate -version
golangci-lint version
asynq version
```

## Database Setup

### Option 1: Using Docker (Recommended for Development)

```bash
# Start all services with Docker Compose
make docker-up

# Wait for services to be ready (about 10 seconds)
sleep 10

# Check service status
docker-compose ps

# Run migrations
make migrate-up

# Verify database connection
docker-compose exec postgres psql -U resell -d resell_inventory -c '\dt'
```

### Option 2: Local PostgreSQL

```bash
# Create database and user
sudo -u postgres psql << EOF
CREATE USER resell WITH PASSWORD 'resell_dev_2025';
CREATE DATABASE resell_inventory OWNER resell;
GRANT ALL PRIVILEGES ON DATABASE resell_inventory TO resell;
EOF

# Run migrations
export DATABASE_URL="postgresql://resell:resell_dev_2025@localhost:5432/resell_inventory?sslmode=disable"
make migrate-up
```

## Initial Data Setup

### 1. Prepare Your Data Files

```bash
# Create sample auction metadata Excel file
# Place your Excel file with auction data in the project root
# Expected columns: invoice_id, auction_id, date, buyers_premium_percent, sales_tax_percent

# Place PDF invoices in the invoices directory
cp /path/to/your/pdf/invoices/*.pdf ./invoices/
```

### 2. Run the Seeder

```bash
# Basic seeding with PDFs only
make run-seeder

# With auction metadata Excel file
go run cmd/seeder/main.go \
  -invoices=./invoices \
  -auctions=./auctions.xlsx \
  -log-level=info

# Dry run to preview without saving
go run cmd/seeder/main.go \
  -invoices=./invoices \
  -auctions=./auctions.xlsx \
  -dry-run=true

# Force reprocess all invoices
go run cmd/seeder/main.go \
  -invoices=./invoices \
  -auctions=./auctions.xlsx \
  -force=true
```

## Running the Application

### Development Mode (with Hot Reload)

```bash
# Start with Air hot reload
make dev

# Or manually
air -c .air.toml

# The API will be available at:
# http://localhost:8080/api/v1/health
```

### Production Mode

```bash
# Build binaries
make build

# Run API server
./bin/resell-api

# In another terminal, run worker
./bin/resell-worker

# Or use Docker
docker-compose -f docker-compose.prod.yml up
```

## Verify Installation

### 1. Check API Health

```bash
# Check health endpoint
curl http://localhost:8080/api/v1/health

# Expected response:
# {"status":"healthy","version":"1.0.0","services":{"database":"healthy","redis":"healthy","workers":"healthy"}}
```

### 2. Test Database Connection

```bash
# Connect to PostgreSQL
make docker-psql

# Or directly
psql -h localhost -U resell -d resell_inventory

# Run test query
SELECT COUNT(*) FROM inventory;
```

### 3. Access Web Interfaces

```bash
# Open in browser:
# - API:          http://localhost:8080
# - Asynq Dashboard: http://localhost:8081
# - pgAdmin:      http://localhost:5050 (admin@resell.local / admin123)
# - MinIO:        http://localhost:9001 (minioadmin / minioadmin123)
# - Mailhog:      http://localhost:8025
```

## Common Operations

### Database Management

```bash
# View current migration status
migrate -path ./migrations -database $DATABASE_URL version

# Create new migration
make migrate-create NAME=add_new_table

# Rollback last migration
make migrate-down

# Reset database completely
make db-reset

# Backup database
docker-compose exec postgres pg_dump -U resell resell_inventory > backup_$(date +%Y%m%d).sql

# Restore database
docker-compose exec -T postgres psql -U resell resell_inventory < backup_20250903.sql
```

### Import/Export Operations

```bash
# Import PDFs from directory
make import-pdf DIR=./new_invoices

# Import Excel file
make import-excel FILE=./inventory_data.xlsx

# Export to Excel
curl -o inventory_export.xlsx http://localhost:8080/api/v1/export/excel

# Export to JSON
curl -o inventory_export.json http://localhost:8080/api/v1/export/json
```

### Testing & Quality

```bash
# Run all tests
make test

# Run with coverage
make test-coverage

# Run linter
make lint

# Format code
make fmt

# Run go vet
make vet

# Run benchmarks
make bench
```

### Docker Operations

```bash
# View logs
make docker-logs

# Stop services
make docker-down

# Clean everything (including volumes)
make docker-clean

# Rebuild images
make docker-build

# Access Redis CLI
make docker-redis-cli

# Execute commands in containers
docker-compose exec postgres bash
docker-compose exec redis redis-cli
```

## Troubleshooting

### Port Already in Use

```bash
# Find process using port 8080
lsof -i :8080

# Kill process
kill -9 <PID>

# Or change port in .env file
APP_PORT=8081
```

### Database Connection Issues

```bash
# Check PostgreSQL is running
docker-compose ps postgres

# Check logs
docker-compose logs postgres

# Test connection
psql postgresql://resell:resell_dev_2025@localhost:5432/resell_inventory

# Reset database container
make docker-db-reset
```

### Migration Errors

```bash
# Check current version
migrate -path ./migrations -database $DATABASE_URL version

# Force version if stuck
make migrate-force VERSION=4

# View migration files
ls -la ./migrations/
```

### Redis Connection Issues

```bash
# Check Redis is running
docker-compose ps redis

# Test connection
redis-cli ping

# Clear Redis cache
redis-cli FLUSHALL
```

## Development Workflow

### 1. Daily Development

```bash
# Start your day
cd ~/projects/resell-be
git pull origin main
make docker-up
make dev

# Make changes, Air will auto-reload

# Before committing
make fmt
make lint
make test

# Commit changes
git add .
git commit -m "feat: add new feature"
git push origin feature/your-feature
```

### 2. Adding New Features

```bash
# Create feature branch
git checkout -b feature/new-feature

# Add dependencies if needed
go get github.com/some/package

# Update go.mod
go mod tidy

# Create migration if needed
make migrate-create NAME=add_feature_tables

# Run migration
make migrate-up

# Test thoroughly
make test

# Create pull request
```

### 3. Debugging

```bash
# Enable debug logging
export LOG_LEVEL=debug
export DEBUG_SQL=true

# Use Delve debugger
go install github.com/go-delve/delve/cmd/dlv@latest
dlv debug ./cmd/api/main.go

# Check Asynq queue status
make asynq-stats

# Monitor logs
make docker-logs
tail -f logs/*.log
```

## Production Deployment Checklist

- [ ] Update .env for production values
- [ ] Set strong passwords for all services
- [ ] Enable SSL/TLS
- [ ] Configure backups
- [ ] Set up monitoring (Prometheus/Grafana)
- [ ] Configure log aggregation
- [ ] Set up CI/CD pipeline
- [ ] Configure auto-scaling
- [ ] Set up alerts
- [ ] Document deployment process
- [ ] Create runbooks for common issues
- [ ] Test disaster recovery

## Next Steps

1. **Customize Configuration**: Edit `.env` for your specific needs
2. **Import Your Data**: Add your PDF invoices and run seeder
3. **Start Development**: Run `make dev` and start coding
4. **Access Dashboard**: Open http://localhost:8080 in your browser
5. **Review API Docs**: Check DESIGN.md for API endpoints

## Support & Resources

- **Documentation**: See `DESIGN.md` for detailed architecture
- **API Reference**: Available at `/api/v1/docs` when running
- **Issues**: Report bugs via GitHub Issues
- **Updates**: Pull latest changes with `git pull origin main`