#!/bin/bash

set -e

echo "🧪 Running Resell Inventory Tests"
echo "================================="

# Set test environment
export APP_ENV=test
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=test_inventory
export REDIS_HOST=localhost
export REDIS_PORT=6379

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Run unit tests
echo -e "\n${YELLOW}Running Unit Tests...${NC}"
go test -v -race -short ./...

# Run integration tests (requires Docker)
if command -v docker &> /dev/null; then
    echo -e "\n${YELLOW}Running Integration Tests...${NC}"
    go test -v -race -tags=integration ./...
else
    echo -e "${YELLOW}Docker not found, skipping integration tests${NC}"
fi

# Run E2E tests (optional)
if [[ "$1" == "--e2e" ]]; then
    echo -e "\n${YELLOW}Running E2E Tests...${NC}"
    go test -v -tags=e2e ./test/e2e/...
fi

# Generate coverage report
echo -e "\n${YELLOW}Generating Coverage Report...${NC}"
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html

echo -e "\n${GREEN}✅ All tests completed!${NC}"
echo "Coverage report: coverage.html"

# Check coverage threshold
COVERAGE=$(go tool cover -func=coverage.out | grep total | awk '{print $3}' | sed 's/%//')
THRESHOLD=70

if (( $(echo "$COVERAGE < $THRESHOLD" | bc -l) )); then
    echo -e "${RED}❌ Coverage ${COVERAGE}% is below threshold ${THRESHOLD}%${NC}"
    exit 1
else
    echo -e "${GREEN}✅ Coverage ${COVERAGE}% meets threshold ${THRESHOLD}%${NC}"
fi