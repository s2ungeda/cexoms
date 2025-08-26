#!/bin/bash

# Docker build script for mExOms

set -e

# Configuration
REGISTRY=${DOCKER_REGISTRY:-"mexoms"}
VERSION=${VERSION:-"latest"}
PLATFORMS=${PLATFORMS:-"linux/amd64,linux/arm64"}

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Function to print colored output
log() {
    echo -e "${GREEN}[BUILD]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Check if Docker is installed
if ! command -v docker &> /dev/null; then
    error "Docker is not installed"
fi

# Check if buildx is available
if ! docker buildx version &> /dev/null; then
    warning "Docker buildx not found, installing..."
    docker buildx create --name mexoms-builder --use
fi

# Function to build and push image
build_image() {
    local service=$1
    local dockerfile=$2
    local build_args=$3
    
    log "Building $service image..."
    
    docker buildx build \
        --platform "$PLATFORMS" \
        --tag "$REGISTRY/$service:$VERSION" \
        --tag "$REGISTRY/$service:latest" \
        --file "$dockerfile" \
        $build_args \
        --push \
        .
    
    if [ $? -eq 0 ]; then
        log "Successfully built $service"
    else
        error "Failed to build $service"
    fi
}

# Build C++ core first
log "Building C++ core library..."
docker buildx build \
    --platform "$PLATFORMS" \
    --target cpp-builder \
    --tag "$REGISTRY/mexoms-core:$VERSION" \
    --file Dockerfile \
    .

# Build main OMS server
build_image "mexoms-server" "build/docker/Dockerfile.server" ""

# Build Binance Spot connector
build_image "mexoms-binance-spot" "build/docker/Dockerfile.binance" "--build-arg CONNECTOR_TYPE=spot"

# Build Binance Futures connector
build_image "mexoms-binance-futures" "build/docker/Dockerfile.binance" "--build-arg CONNECTOR_TYPE=futures"

# Build monitoring service
build_image "mexoms-monitor" "Dockerfile" ""

# Build all-in-one image
log "Building all-in-one image..."
build_image "mexoms-all" "Dockerfile" ""

log "All images built successfully!"

# Print image sizes
log "Image sizes:"
docker images | grep "$REGISTRY" | grep "$VERSION"