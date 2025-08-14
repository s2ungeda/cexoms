cmake_minimum_required(VERSION 3.16)
project(oms_core VERSION 1.0.0 LANGUAGES CXX)

# C++17 standard (for compatibility with GCC 9)
set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Compiler flags
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -Wall -Wextra -Wpedantic")
set(CMAKE_CXX_FLAGS_DEBUG "-g -O0 -fsanitize=address -fsanitize=undefined")
set(CMAKE_CXX_FLAGS_RELEASE "-O3 -march=native -mtune=native -DNDEBUG")

# Find required packages
find_package(Threads REQUIRED)

# Include directories
include_directories(${CMAKE_CURRENT_SOURCE_DIR}/include)

# Source files
set(SOURCES
    engine/order_manager.cpp
)

# Header files
set(HEADERS
    include/types.h
    include/ring_buffer.h
    include/order_manager.h
)

# Create static library
add_library(oms_core STATIC ${SOURCES})

# Link libraries
target_link_libraries(oms_core
    PRIVATE
        Threads::Threads
)

# Set properties
set_target_properties(oms_core PROPERTIES
    VERSION ${PROJECT_VERSION}
    POSITION_INDEPENDENT_CODE ON
)

# Install targets
install(TARGETS oms_core
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
)

# Install headers
install(FILES ${HEADERS}
    DESTINATION include/oms
)

# Build tests if enabled
option(BUILD_TESTS "Build unit tests" OFF)
if(BUILD_TESTS AND EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/tests/CMakeLists.txt")
    enable_testing()
    add_subdirectory(tests)
endif()

# Build benchmarks if enabled
option(BUILD_BENCHMARKS "Build performance benchmarks" OFF)
if(BUILD_BENCHMARKS AND EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/benchmarks/CMakeLists.txt")
    add_subdirectory(benchmarks)
endif()

# Export compile commands for development tools
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)