# Multi-stage build for efficient image size

# Stage 1: Build C++ core
FROM ubuntu:22.04 AS cpp-builder

# Install C++ build dependencies
RUN apt-get update && apt-get install -y \
    build-essential \
    cmake \
    g++ \
    libboost-all-dev \
    libtbb-dev \
    git \
    && rm -rf /var/lib/apt/lists/*

# Set working directory
WORKDIR /build

# Copy C++ source
COPY core/ ./core/
COPY CMakeLists.txt ./

# Build C++ core
RUN cmake -B build -DCMAKE_BUILD_TYPE=Release && \
    cmake --build build --parallel $(nproc)

# Stage 2: Build Go services
FROM golang:1.21-alpine AS go-builder

# Install dependencies
RUN apk add --no-cache git gcc musl-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Copy C++ libraries from previous stage
COPY --from=cpp-builder /build/build/lib/*.so /usr/local/lib/
COPY --from=cpp-builder /build/core/include /usr/local/include/mExOms

# Build Go binaries
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o bin/oms-server ./cmd/server
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o bin/binance-spot ./cmd/binance-spot
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o bin/binance-futures ./cmd/binance-futures
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o bin/monitor ./cmd/monitor

# Stage 3: Final runtime image
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    libc6-compat \
    libstdc++ \
    tini

# Create non-root user
RUN addgroup -g 1000 oms && \
    adduser -u 1000 -G oms -s /bin/sh -D oms

# Set working directory
WORKDIR /app

# Copy C++ libraries
COPY --from=cpp-builder /build/build/lib/*.so /usr/local/lib/
RUN ldconfig /usr/local/lib

# Copy Go binaries
COPY --from=go-builder /app/bin/* /app/bin/

# Copy configuration files
COPY --chown=oms:oms configs/ /app/configs/

# Create necessary directories
RUN mkdir -p /app/logs /app/data && \
    chown -R oms:oms /app

# Switch to non-root user
USER oms

# Expose ports
EXPOSE 8080 8081 9090

# Use tini to handle signals properly
ENTRYPOINT ["/sbin/tini", "--"]

# Default command (can be overridden)
CMD ["/app/bin/oms-server"]