#!/bin/bash
#
# OMS System Monitoring Script
#

# Configuration
OMS_HOME="/opt/oms"
LOG_DIR="/var/log/oms"
ALERT_EMAIL="ops@example.com"
NATS_URL="http://localhost:8222"
GRPC_PORT="50051"

# Thresholds
CPU_THRESHOLD=90
MEM_THRESHOLD=85
DISK_THRESHOLD=80
LATENCY_THRESHOLD=1000  # microseconds

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Functions
check_service() {
    local service=$1
    if systemctl is-active --quiet "${service}.service"; then
        echo -e "${GREEN}✓${NC} ${service}: Running"
        return 0
    else
        echo -e "${RED}✗${NC} ${service}: Not running"
        return 1
    fi
}

check_cpu_usage() {
    echo -e "\n${BLUE}CPU Usage by Core:${NC}"
    
    # Get OMS process CPU usage per core
    ps -eLo pid,tid,psr,pcpu,comm | grep -E "oms-|nats" | \
        awk '{cpu[$3]+=$4; count[$3]++} 
             END {for (i in cpu) printf "  Core %d: %.1f%% (%d threads)\n", i, cpu[i], count[i]}' | \
        sort -n
    
    # Check for high CPU usage
    local total_cpu=$(ps aux | grep -E "oms-|nats" | awk '{sum+=$3} END {print int(sum)}')
    if [ "$total_cpu" -gt "$CPU_THRESHOLD" ]; then
        echo -e "  ${YELLOW}Warning: High CPU usage (${total_cpu}%)${NC}"
    fi
}

check_memory_usage() {
    echo -e "\n${BLUE}Memory Usage:${NC}"
    
    # System memory
    local mem_info=$(free -m | awk 'NR==2{printf "  Total: %sMB, Used: %sMB (%.1f%%)\n", $2, $3, $3*100/$2}')
    echo "$mem_info"
    
    # OMS process memory
    ps aux | grep -E "oms-|nats" | grep -v grep | \
        awk '{printf "  %-20s %6.1fMB (%.1f%%)\n", $11, $6/1024, $4}' | \
        sort -k2 -nr
    
    # Check shared memory
    local shm_used=$(df -h /dev/shm | awk 'NR==2{print $5}' | sed 's/%//')
    echo -e "  Shared Memory: ${shm_used}% used"
}

check_disk_usage() {
    echo -e "\n${BLUE}Disk Usage:${NC}"
    
    # Check main partitions
    df -h | grep -E "^/dev/" | while read line; do
        local usage=$(echo $line | awk '{print $5}' | sed 's/%//')
        local mount=$(echo $line | awk '{print $6}')
        
        if [ "$usage" -gt "$DISK_THRESHOLD" ]; then
            echo -e "  ${YELLOW}${mount}: ${usage}% (Warning)${NC}"
        else
            echo "  ${mount}: ${usage}%"
        fi
    done
    
    # Check OMS data directory
    if [ -d "${OMS_HOME}/data" ]; then
        local oms_size=$(du -sh ${OMS_HOME}/data 2>/dev/null | awk '{print $1}')
        echo "  OMS Data: ${oms_size}"
    fi
}

check_network_connectivity() {
    echo -e "\n${BLUE}Network Connectivity:${NC}"
    
    # Check NATS
    if curl -s "${NATS_URL}/varz" > /dev/null; then
        local connections=$(curl -s "${NATS_URL}/varz" | jq -r '.connections // 0')
        echo -e "  ${GREEN}✓${NC} NATS: ${connections} connections"
    else
        echo -e "  ${RED}✗${NC} NATS: Cannot connect"
    fi
    
    # Check gRPC
    if timeout 2 nc -z localhost ${GRPC_PORT} 2>/dev/null; then
        echo -e "  ${GREEN}✓${NC} gRPC Gateway: Port ${GRPC_PORT} open"
    else
        echo -e "  ${RED}✗${NC} gRPC Gateway: Port ${GRPC_PORT} closed"
    fi
    
    # Check exchange connectivity
    # Add exchange-specific checks here
}

check_latency() {
    echo -e "\n${BLUE}Performance Metrics:${NC}"
    
    # Check if metrics file exists
    local metrics_file="${OMS_HOME}/data/metrics/latest.json"
    if [ -f "$metrics_file" ]; then
        # Parse metrics (example format)
        local order_latency=$(jq -r '.order_processing.p99 // 0' "$metrics_file" 2>/dev/null)
        local risk_latency=$(jq -r '.risk_check.p99 // 0' "$metrics_file" 2>/dev/null)
        
        echo "  Order Processing (p99): ${order_latency}μs"
        echo "  Risk Check (p99): ${risk_latency}μs"
        
        if [ "$order_latency" -gt "$LATENCY_THRESHOLD" ]; then
            echo -e "  ${YELLOW}Warning: High order processing latency${NC}"
        fi
    else
        echo "  Metrics not available"
    fi
}

check_logs() {
    echo -e "\n${BLUE}Recent Errors:${NC}"
    
    # Check for recent errors in logs
    local error_count=0
    
    for service in oms-engine oms-binance-spot oms-grpc-gateway; do
        if [ -f "${LOG_DIR}/${service}.log" ]; then
            local errors=$(tail -n 1000 "${LOG_DIR}/${service}.log" 2>/dev/null | grep -i "error" | wc -l)
            if [ "$errors" -gt 0 ]; then
                echo "  ${service}: ${errors} errors in last 1000 lines"
                error_count=$((error_count + errors))
            fi
        fi
    done
    
    if [ "$error_count" -eq 0 ]; then
        echo "  No recent errors found"
    fi
}

generate_summary() {
    echo -e "\n${BLUE}System Summary:${NC}"
    
    # Uptime
    local uptime=$(uptime -p)
    echo "  System uptime: ${uptime}"
    
    # Service uptime
    for service in oms-engine oms-binance-spot; do
        if systemctl is-active --quiet "${service}.service"; then
            local start_time=$(systemctl show -p ActiveEnterTimestamp "${service}.service" | cut -d= -f2)
            if [ -n "$start_time" ]; then
                echo "  ${service} uptime: $(date -d "$start_time" '+%d days %H hours')"
            fi
        fi
    done
    
    # Trading volume (if available)
    # Add trading statistics here
}

# Main monitoring loop
main() {
    clear
    echo "========================================"
    echo "       OMS System Monitor"
    echo "       $(date)"
    echo "========================================"
    
    # Service status
    echo -e "\n${BLUE}Service Status:${NC}"
    check_service "nats"
    check_service "oms-engine"
    check_service "oms-binance-spot"
    check_service "oms-binance-futures"
    check_service "oms-grpc-gateway"
    
    # System resources
    check_cpu_usage
    check_memory_usage
    check_disk_usage
    
    # Network and connectivity
    check_network_connectivity
    
    # Performance metrics
    check_latency
    
    # Log analysis
    check_logs
    
    # Summary
    generate_summary
    
    echo -e "\n========================================"
}

# Handle command line arguments
case "${1:-}" in
    --once)
        main
        ;;
    --continuous)
        while true; do
            main
            sleep "${2:-60}"
            clear
        done
        ;;
    --json)
        # Output in JSON format for external monitoring systems
        # TODO: Implement JSON output
        echo '{"status": "not_implemented"}'
        ;;
    *)
        echo "Usage: $0 [--once|--continuous [interval]|--json]"
        echo "  --once       Run once and exit"
        echo "  --continuous Run continuously (default: 60s interval)"
        echo "  --json       Output in JSON format"
        exit 1
        ;;
esac