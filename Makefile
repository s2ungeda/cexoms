.PHONY: all build test clean install-deps proto run-nats run-redis

# Variables
GO := go
CXX := g++
CXXFLAGS := -std=c++17 -O3 -march=native -pthread -Wall -Wextra
LDFLAGS := -lpthread -lnats -lredis++ -lhiredis -lssl -lcrypto

# Directories
BUILD_DIR := build
BIN_DIR := bin
PROTO_DIR := proto
CORE_DIR := core

# Targets
all: build

install-deps:
	@echo "Installing Go dependencies..."
	go mod download
	go mod tidy
	@echo "Installing system dependencies..."
	@echo "Please ensure you have installed: libnats-dev, libhiredis-dev, protobuf-compiler"

build: build-core build-services

build-core:
	@echo "Building C++ core engine..."
	@mkdir -p $(BUILD_DIR) $(BIN_DIR)
	cd $(CORE_DIR) && \
		cmake -B $(BUILD_DIR) -DCMAKE_BUILD_TYPE=Release && \
		cmake --build $(BUILD_DIR) --parallel
	$(CXX) $(CXXFLAGS) -I$(CORE_DIR)/include \
		$(CORE_DIR)/engine/main.cpp \
		$(CORE_DIR)/$(BUILD_DIR)/liboms_core.a \
		-o $(BIN_DIR)/oms-engine $(LDFLAGS)

build-services:
	@echo "Building Go services..."
	@mkdir -p $(BIN_DIR)
	$(GO) build -o $(BIN_DIR)/binance-spot ./cmd/binance-spot/
	$(GO) build -o $(BIN_DIR)/binance-futures ./cmd/binance-futures/
	$(GO) build -o $(BIN_DIR)/grpc-gateway ./cmd/grpc-gateway/
	$(GO) build -o $(BIN_DIR)/grpc-client ./cmd/grpc-client/
	$(GO) build -o $(BIN_DIR)/test-grpc ./cmd/test-grpc/
	$(GO) build -o $(BIN_DIR)/monitor ./cmd/monitor/

proto:
	@echo "Generating protobuf files..."
	@mkdir -p pkg/proto
	protoc -I $(PROTO_DIR) \
		--go_out=pkg/proto --go_opt=paths=source_relative \
		--go-grpc_out=pkg/proto --go-grpc_opt=paths=source_relative \
		$(PROTO_DIR)/oms/v1/*.proto

test:
	@echo "Running Go tests..."
	$(GO) test -v -race -cover ./...
	@echo "Running C++ tests..."
	@if [ -f $(BIN_DIR)/core-tests ]; then \
		$(BIN_DIR)/core-tests; \
	fi

test-benchmark:
	@echo "Running performance benchmarks..."
	$(GO) test -bench=. -benchmem ./test/benchmark/...

clean:
	@echo "Cleaning build artifacts..."
	rm -rf $(BUILD_DIR) $(BIN_DIR)
	rm -rf vendor/
	find . -name "*.test" -delete
	find . -name "*.out" -delete

run-nats:
	@echo "Starting NATS server..."
	docker run -d --name nats-oms \
		-p 4222:4222 -p 8222:8222 -p 6222:6222 \
		nats:latest -js

run-redis:
	@echo "Starting Redis server..."
	docker run -d --name redis-oms \
		-p 6379:6379 \
		redis:7-alpine

run-postgres:
	@echo "Starting PostgreSQL server..."
	docker run -d --name postgres-oms \
		-e POSTGRES_PASSWORD=oms_password \
		-e POSTGRES_DB=oms_db \
		-p 5432:5432 \
		postgres:15-alpine

run-vault:
	@echo "Starting HashiCorp Vault..."
	docker run -d --name vault-oms \
		--cap-add=IPC_LOCK \
		-e VAULT_DEV_ROOT_TOKEN_ID=root-token \
		-p 8200:8200 \
		hashicorp/vault

docker-build:
	@echo "Building Docker images..."
	docker build -f deployments/docker/Dockerfile.engine -t oms-engine:latest .
	docker build -f deployments/docker/Dockerfile.services -t oms-services:latest .

docker-compose-up:
	@echo "Starting all services with docker-compose..."
	docker-compose -f deployments/docker/docker-compose.yml up -d

docker-compose-down:
	@echo "Stopping all services..."
	docker-compose -f deployments/docker/docker-compose.yml down

fmt:
	@echo "Formatting Go code..."
	$(GO) fmt ./...
	@echo "Formatting C++ code..."
	find $(CORE_DIR) -name "*.cpp" -o -name "*.h" | xargs clang-format -i

lint:
	@echo "Linting Go code..."
	golangci-lint run
	@echo "Linting C++ code..."
	cpplint --recursive $(CORE_DIR)/

help:
	@echo "Available targets:"
	@echo "  make install-deps    - Install dependencies"
	@echo "  make build          - Build all components"
	@echo "  make test           - Run all tests"
	@echo "  make test-benchmark - Run performance benchmarks"
	@echo "  make proto          - Generate protobuf files"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make run-nats       - Start NATS server"
	@echo "  make run-redis      - Start Redis server"
	@echo "  make run-postgres   - Start PostgreSQL server"
	@echo "  make run-vault      - Start HashiCorp Vault"
	@echo "  make docker-build   - Build Docker images"
	@echo "  make fmt            - Format code"
	@echo "  make lint           - Lint code"