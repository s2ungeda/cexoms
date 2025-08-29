#!/bin/bash

# Build script for TCP Server

echo "Building mExOms TCP Server..."

# Create build directory
mkdir -p core/build
cd core/build

# Configure with CMake
cmake .. -DCMAKE_BUILD_TYPE=Release

# Build TCP server
make tcp_server_lib tcp-server tcp-client-example -j$(nproc)

# Check if build succeeded
if [ $? -eq 0 ]; then
    echo "Build successful!"
    echo "Binaries located at:"
    echo "  - Server: bin/tcp-server"
    echo "  - Client: bin/tcp-client-example"
else
    echo "Build failed!"
    exit 1
fi

cd ../..

# Make binaries executable
chmod +x bin/tcp-server
chmod +x bin/tcp-client-example

echo "Done!"