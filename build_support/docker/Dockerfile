# build/docker/Dockerfile
# Development Dockerfile with hot reload support

FROM golang:1.22-alpine AS base

# Install system dependencies
RUN apk add --no-cache \
    git \
    make \
    curl \
    gcc \
    musl-dev \
    postgresql-client \
    ca-certificates \
    tzdata

# Set working directory
WORKDIR /app

# Install development tools
RUN go install github.com/cosmtrek/air@latest && \
    go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest && \
    go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest && \
    go install github.com/hibiken/asynq/tools/asynq@latest && \
    go install golang.org/x/tools/cmd/goimports@latest

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && \
    go mod verify

# Copy source code
COPY . .

# Expose ports
EXPOSE 8080 8081

# Use air for hot reload in development
CMD ["air", "-c", ".air.toml"]