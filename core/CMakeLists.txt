cmake_minimum_required(VERSION 3.16)
project(OMS_Core VERSION 1.0.0 LANGUAGES CXX)

# C++20 standard
set(CMAKE_CXX_STANDARD 20)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)

# Compiler flags
set(CMAKE_CXX_FLAGS "${CMAKE_CXX_FLAGS} -Wall -Wextra -Wpedantic")
set(CMAKE_CXX_FLAGS_DEBUG "-g -O0 -DDEBUG")
set(CMAKE_CXX_FLAGS_RELEASE "-O3 -DNDEBUG -march=native -mavx2 -mfma")

# Enable position independent code
set(CMAKE_POSITION_INDEPENDENT_CODE ON)

# Find required packages
find_package(Threads REQUIRED)

# Find NUMA library (optional)
find_library(NUMA_LIBRARY numa)
if(NUMA_LIBRARY)
    message(STATUS "Found NUMA library: ${NUMA_LIBRARY}")
    add_definitions(-DHAS_NUMA)
else()
    message(STATUS "NUMA library not found, NUMA support disabled")
endif()

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
    src/risk/multi_account_risk_engine.cpp
    src/risk/lock_free_risk_checker.cpp
    src/account/multi_account_risk_engine_simple.cpp
    src/performance/cpu_affinity.cpp
    src/performance/memory_pool.cpp
    src/performance/simd_operations.cpp
)

# Create static library
add_library(oms_core_lib STATIC ${LIB_SOURCES})
target_link_libraries(oms_core_lib 
    Threads::Threads
    atomic
)

# Link NUMA library if found
if(NUMA_LIBRARY)
    target_link_libraries(oms_core_lib ${NUMA_LIBRARY})
endif()

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
option(BUILD_TESTS "Build tests" ON)
if(BUILD_TESTS AND EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/tests")
    enable_testing()
    
    # Multi-account risk engine test (simple)
    add_executable(test_multi_account_risk_simple tests/test_multi_account_risk_simple.cpp)
    target_link_libraries(test_multi_account_risk_simple
        oms_core_lib
        Threads::Threads
        atomic
    )
    set_target_properties(test_multi_account_risk_simple PROPERTIES
        RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
    )
    add_test(NAME MultiAccountRiskSimpleTest COMMAND test_multi_account_risk_simple)
    
    # Multi-account risk engine test (advanced)
    add_executable(test_multi_account_risk_engine tests/test_multi_account_risk_engine.cpp)
    target_link_libraries(test_multi_account_risk_engine
        oms_core_lib
        Threads::Threads
        atomic
    )
    set_target_properties(test_multi_account_risk_engine PROPERTIES
        RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
    )
    add_test(NAME MultiAccountRiskEngineTest COMMAND test_multi_account_risk_engine)
    
    # Performance optimization test
    add_executable(test_performance_optimization tests/test_performance_optimization.cpp)
    target_link_libraries(test_performance_optimization
        oms_core_lib
        Threads::Threads
        atomic
    )
    if(NUMA_LIBRARY)
        target_link_libraries(test_performance_optimization ${NUMA_LIBRARY})
    endif()
    set_target_properties(test_performance_optimization PROPERTIES
        RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
    )
    add_test(NAME PerformanceOptimizationTest COMMAND test_performance_optimization)
endif()

# Examples
option(BUILD_EXAMPLES "Build examples" ON)
if(BUILD_EXAMPLES AND EXISTS "${CMAKE_CURRENT_SOURCE_DIR}/examples")
    # Performance demo example
    add_executable(performance_demo examples/performance_demo.cpp)
    target_link_libraries(performance_demo
        oms_core_lib
        Threads::Threads
        atomic
    )
    if(NUMA_LIBRARY)
        target_link_libraries(performance_demo ${NUMA_LIBRARY})
    endif()
    set_target_properties(performance_demo PROPERTIES
        RUNTIME_OUTPUT_DIRECTORY ${CMAKE_BINARY_DIR}
    )
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