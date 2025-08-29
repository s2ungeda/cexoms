cmake_minimum_required(VERSION 3.16)
project(tcp_server)

# Find Protocol Buffers
find_package(Protobuf REQUIRED)

# TCP Server library sources
set(TCP_SERVER_SOURCES
    tcp_server.cpp
    client_session.cpp
    message_handler.cpp
    broadcaster.cpp
)

# Platform-specific sources
if(WIN32)
    list(APPEND TCP_SERVER_SOURCES tcp_server_windows.cpp)
else()
    list(APPEND TCP_SERVER_SOURCES tcp_server_linux.cpp)
endif()

# Create TCP server library
add_library(tcp_server_lib STATIC ${TCP_SERVER_SOURCES})

# Include directories
target_include_directories(tcp_server_lib PUBLIC
    ${CMAKE_CURRENT_SOURCE_DIR}
    ${CMAKE_CURRENT_SOURCE_DIR}/..
    ${PROTOBUF_INCLUDE_DIRS}
)

# Link libraries
target_link_libraries(tcp_server_lib
    ${PROTOBUF_LIBRARIES}
    Threads::Threads
    atomic
)

# TCP Server executable
add_executable(tcp-server ${CMAKE_CURRENT_SOURCE_DIR}/../../cmd/tcp-server/main.cpp)

target_link_libraries(tcp-server
    tcp_server_lib
    ${PROTOBUF_LIBRARIES}
    Threads::Threads
)

# TCP Client example
add_executable(tcp-client-example ${CMAKE_CURRENT_SOURCE_DIR}/../../examples/tcp_client_example.cpp)

target_link_libraries(tcp-client-example
    Threads::Threads
)

# TCP Benchmark tool
add_executable(tcp-benchmark ${CMAKE_CURRENT_SOURCE_DIR}/../../examples/tcp_benchmark.cpp)

target_link_libraries(tcp-benchmark
    Threads::Threads
)

# Set output directory
set_target_properties(tcp-server tcp-client-example tcp-benchmark PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}/../../bin
)