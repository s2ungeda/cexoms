#!/bin/bash

# Integration Test Suite for Multi-Exchange OMS

echo "==================================="
echo "Multi-Exchange OMS Integration Test"
echo "==================================="
echo ""

# Color codes
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
NC='\033[0m'

# Test results
PASSED=0
FAILED=0

# Test function
run_test() {
    local name=$1
    local cmd=$2
    local expected=$3
    
    printf "Testing %-40s ... " "$name"
    
    result=$(eval "$cmd" 2>&1)
    
    if [[ "$result" == *"$expected"* ]]; then
        echo -e "${GREEN}PASSED${NC}"
        ((PASSED++))
    else
        echo -e "${RED}FAILED${NC}"
        echo "  Expected: $expected"
        echo "  Got: $result"
        ((FAILED++))
    fi
}

# 1. Service Health Checks
echo "1. Service Health Checks"
echo "------------------------"

run_test "C++ Core Engine running" \
    "pgrep -f oms-core > /dev/null && echo 'running'" \
    "running"

run_test "gRPC server listening" \
    "nc -zv localhost 50051 2>&1" \
    "succeeded"

run_test "Binance Spot connector" \
    "pgrep -f binance-spot > /dev/null && echo 'running'" \
    "running"

run_test "Binance Futures connector" \
    "pgrep -f binance-futures > /dev/null && echo 'running'" \
    "running"

# 2. API Connectivity Tests
echo ""
echo "2. API Connectivity Tests"
echo "-------------------------"

run_test "Binance Spot API ping" \
    "curl -s https://api.binance.com/api/v3/ping" \
    "{}"

run_test "Binance Futures API ping" \
    "curl -s https://fapi.binance.com/fapi/v1/ping" \
    "{}"

run_test "Binance server time" \
    "curl -s https://api.binance.com/api/v3/time | grep -o serverTime" \
    "serverTime"

# 3. Log File Tests
echo ""
echo "3. Log File Tests"
echo "-----------------"

run_test "OMS Core log exists" \
    "test -f logs/oms-core.log && echo 'exists'" \
    "exists"

run_test "Core engine started message" \
    "grep -q 'OMS Core Engine Starting' logs/oms-core.log && echo 'found'" \
    "found"

run_test "Risk Engine started" \
    "grep -q 'Risk Engine started' logs/oms-core.log && echo 'found'" \
    "found"

# 4. Performance Tests
echo ""
echo "4. Performance Tests"
echo "--------------------"

# Check CPU usage
run_test "CPU usage reasonable" \
    "ps aux | grep -E 'oms-core|binance-' | grep -v grep | awk '{sum += \$3} END {print (sum < 50) ? \"ok\" : \"high\"}'" \
    "ok"

# Check memory usage
run_test "Memory usage reasonable" \
    "ps aux | grep -E 'oms-core|binance-' | grep -v grep | awk '{sum += \$4} END {print (sum < 10) ? \"ok\" : \"high\"}'" \
    "ok"

# 5. Inter-service Communication
echo ""
echo "5. Inter-service Communication"
echo "------------------------------"

# Test gRPC connectivity
run_test "gRPC port open" \
    "netstat -tln 2>/dev/null | grep -q ':50051' && echo 'open' || echo 'closed'" \
    "open"

# Summary
echo ""
echo "================================="
echo "Test Summary"
echo "================================="
echo -e "Passed: ${GREEN}$PASSED${NC}"
echo -e "Failed: ${RED}$FAILED${NC}"
echo ""

if [ $FAILED -eq 0 ]; then
    echo -e "${GREEN}All tests passed!${NC} ✓"
    exit 0
else
    echo -e "${RED}Some tests failed!${NC} ✗"
    exit 1
fi