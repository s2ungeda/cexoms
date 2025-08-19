cmake_minimum_required(VERSION 3.16)
project(OMS_Core VERSION 1.0.0 LANGUAGES CXX)

# C++20 standard
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Compiler flags
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -Wall -Wextra -Wpedantic")
set(CMAKE_CXX_FLAGS_DEBUG "-g -O0 -DDEBUG")
set(CMAKE_CXX_FLAGS_RELEASE "-O3 -DNDEBUG -march=native")

# Enable position independent code
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# Find required packages
find_package(Threads REQUIRED)

# Include directories
include_directories(
    ${CMAKE_CURRENT_SOURCE_DIR}/include
)

# Source files
set(CORE_SOURCES
    src/main.cpp
)

set(LIB_SOURCES
    src/strategies/arbitrage_detector.cpp
    src/strategies/market_maker.cpp
    src/risk/risk_engine.cpp
)

# Create static library
add_library(oms_core_lib STATIC ${LIB_SOURCES})
target_link_libraries(oms_core_lib 
    Threads::Threads
    atomic
)

# Create executable
add_executable(oms-core ${CORE_SOURCES})
target_link_libraries(oms-core 
    oms_core_lib
    Threads::Threads
    atomic
)

# Set runtime path
set_target_properties(oms-core PROPERTIES
    RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
)

# Enable Link Time Optimization
include(CheckIPOSupported)
check_ipo_supported(RESULT ipo_supported OUTPUT error)
if(ipo_supported)
    set_property(TARGET oms-core PROPERTY INTERPROCEDURAL_OPTIMIZATION TRUE)
    set_property(TARGET oms_core_lib PROPERTY INTERPROCEDURAL_OPTIMIZATION TRUE)
endif()

# CPU affinity and performance tuning
if(CMAKE_SYSTEM_NAME STREQUAL "Linux")
    target_compile_definitions(oms-core PRIVATE USE_CPU_AFFINITY)
endif()

# Testing
option(BUILD_TESTS "Build tests" OFF)
if(BUILD_TESTS AND EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/tests")
    enable_testing()
    add_subdirectory(tests)
endif()

# Benchmarks
option(BUILD_BENCHMARKS "Build benchmarks" OFF)
if(BUILD_BENCHMARKS AND EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/benchmarks")
    add_subdirectory(benchmarks)
endif()

# Installation
install(TARGETS oms-core oms_core_lib
    RUNTIME DESTINATION bin
    LIBRARY DESTINATION lib
    ARCHIVE DESTINATION lib
)

install(DIRECTORY include/
    DESTINATION include
    FILES_MATCHING PATTERN "*.h"
)

# Summary
message(STATUS "")
message(STATUS "OMS Core Configuration:")
message(STATUS "  Version: ${PROJECT_VERSION}")
message(STATUS "  C++ Standard: ${CMAKE_CXX_STANDARD}")
message(STATUS "  Build Type: ${CMAKE_BUILD_TYPE}")
message(STATUS "  Compiler: ${CMAKE_CXX_COMPILER}")
message(STATUS "  Compiler Flags: ${CMAKE_CXX_FLAGS}")
message(STATUS "  Tests: ${BUILD_TESTS}")
message(STATUS "  Benchmarks: ${BUILD_BENCHMARKS}")
message(STATUS "")