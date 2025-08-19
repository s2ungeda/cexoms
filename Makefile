# Multi-Exchange OMS Makefile

# Variables
GOCMD=go
GOBUILD=$(GOCMD) build
GOTEST=$(GOCMD) test
GOGET=$(GOCMD) get
GOMOD=$(GOCMD) mod
GOFMT=$(GOCMD) fmt
GOLINT=golangci-lint

CXX=g++
CXXFLAGS=-std=c++20 -O3 -Wall -Wextra -pthread -march=native
LDFLAGS=-lpthread -latomic

# Directories
CORE_DIR=core
GO_DIR=.
BIN_DIR=bin
BUILD_DIR=build

# Binary names
CORE_BIN=$(BIN_DIR)/oms-core
OMS_SERVER=$(BIN_DIR)/oms-server
BINANCE_SPOT=$(BIN_DIR)/binance-spot
BINANCE_FUTURES=$(BIN_DIR)/binance-futures

# Default target
.PHONY: all
all: clean deps build

# Install dependencies
.PHONY: deps
deps:
	@echo "Installing dependencies..."
	$(GOMOD) download
	$(GOMOD) tidy
	@echo "Installing protoc-gen-go..."
	go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
	go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest

# Build all components
.PHONY: build
build: build-core build-services

# Build C++ core
.PHONY: build-core
build-core:
	@echo "Building C++ core engine..."
	@mkdir -p $(BIN_DIR) $(BUILD_DIR)
	cd $(CORE_DIR) && mkdir -p build && cd build && cmake .. && make -j$(nproc)
	@cp $(CORE_DIR)/build/oms-core $(CORE_BIN)
	@echo "C++ core built successfully: $(CORE_BIN)"

# Build Go services
.PHONY: build-services
build-services: proto
	@echo "Building Go services..."
	@mkdir -p $(BIN_DIR)
	
	@echo "Building OMS server..."
	$(GOBUILD) -o $(OMS_SERVER) -v ./cmd/oms-server
	
	@echo "Building Binance Spot connector..."
	$(GOBUILD) -o $(BINANCE_SPOT) -v ./services/binance/spot
	
	@echo "Building Binance Futures connector..."
	$(GOBUILD) -o $(BINANCE_FUTURES) -v ./services/binance/futures
	
	@echo "Go services built successfully"

# Generate protobuf files
.PHONY: proto
proto:
	@echo "Generating protobuf files..."
	@mkdir -p pkg/proto
	protoc --go_out=. --go_opt=paths=source_relative \
		--go-grpc_out=. --go-grpc_opt=paths=source_relative \
		proto/*.proto

# Run tests
.PHONY: test
test: test-go test-core test-integration

# Run Go tests
.PHONY: test-go
test-go:
	@echo "Running Go tests..."
	$(GOTEST) -v -race -coverprofile=coverage.txt -covermode=atomic ./...

# Run C++ tests
.PHONY: test-core
test-core:
	@echo "Running C++ tests..."
	@if [ -f $(CORE_DIR)/build/tests/core-tests ]; then \
		$(CORE_DIR)/build/tests/core-tests; \
	else \
		echo "C++ tests not built yet"; \
	fi

# Run integration tests
.PHONY: test-integration
test-integration:
	@echo "Running integration tests..."
	$(GOTEST) -v -tags=integration ./tests/integration/...

# Run benchmarks
.PHONY: test-benchmark
test-benchmark:
	@echo "Running performance benchmarks..."
	$(GOTEST) -bench=. -benchmem ./...
	@if [ -f $(CORE_DIR)/build/tests/core-benchmarks ]; then \
		$(CORE_DIR)/build/tests/core-benchmarks; \
	fi

# Format code
.PHONY: fmt
fmt:
	@echo "Formatting Go code..."
	$(GOFMT) ./...
	@echo "Formatting C++ code..."
	find $(CORE_DIR) -name "*.cpp" -o -name "*.h" | xargs clang-format -i

# Lint code
.PHONY: lint
lint:
	@echo "Linting Go code..."
	$(GOLINT) run ./...
	@echo "Linting C++ code..."
	cd $(CORE_DIR) && cppcheck --enable=all --suppress=missingIncludeSystem .

# Clean build artifacts
.PHONY: clean
clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BIN_DIR) $(BUILD_DIR)
	rm -rf $(CORE_DIR)/build
	rm -f coverage.txt

# Infrastructure commands
.PHONY: run-nats
run-nats:
	@echo "Starting NATS server..."
	docker run -d --name nats-oms -p 4222:4222 -p 8222:8222 nats:latest -js

.PHONY: run-redis
run-redis:
	@echo "Starting Redis..."
	docker run -d --name redis-oms -p 6379:6379 redis:latest

.PHONY: run-postgres
run-postgres:
	@echo "Starting PostgreSQL..."
	docker run -d --name postgres-oms -p 5432:5432 \
		-e POSTGRES_PASSWORD=omspassword \
		-e POSTGRES_DB=omsdb \
		postgres:latest

.PHONY: run-vault
run-vault:
	@echo "Starting HashiCorp Vault..."
	docker run -d --name vault-oms -p 8200:8200 \
		-e VAULT_DEV_ROOT_TOKEN_ID=root \
		-e VAULT_DEV_LISTEN_ADDRESS=0.0.0.0:8200 \
		vault:latest

# Start all infrastructure
.PHONY: infra-up
infra-up: run-nats run-vault
	@echo "Infrastructure started"

# Stop all infrastructure
.PHONY: infra-down
infra-down:
	@echo "Stopping infrastructure..."
	docker stop nats-oms redis-oms postgres-oms vault-oms 2>/dev/null || true
	docker rm nats-oms redis-oms postgres-oms vault-oms 2>/dev/null || true

# Development helpers
.PHONY: dev
dev: deps fmt lint build test
	@echo "Development build complete"

# Install pre-commit hooks
.PHONY: install-hooks
install-hooks:
	@echo "Installing git hooks..."
	cp scripts/pre-commit .git/hooks/
	chmod +x .git/hooks/pre-commit

# Generate documentation
.PHONY: docs
docs:
	@echo "Generating documentation..."
	godoc -http=:6060 &
	@echo "Documentation server started at http://localhost:6060"

# Show help
.PHONY: help
help:
	@echo "Multi-Exchange OMS Makefile"
	@echo ""
	@echo "Usage:"
	@echo "  make all          - Clean, install deps, and build everything"
	@echo "  make build        - Build all components"
	@echo "  make build-core   - Build C++ core engine only"
	@echo "  make build-services - Build Go services only"
	@echo "  make test         - Run all tests"
	@echo "  make test-benchmark - Run performance benchmarks"
	@echo "  make fmt          - Format all code"
	@echo "  make lint         - Lint all code"
	@echo "  make clean        - Clean build artifacts"
	@echo "  make infra-up     - Start infrastructure services"
	@echo "  make infra-down   - Stop infrastructure services"
	@echo "  make dev          - Full development build (deps, fmt, lint, build, test)"
	@echo "  make help         - Show this help message"

# Default make target
.DEFAULT_GOAL := help