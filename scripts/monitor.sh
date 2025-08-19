#!/bin/bash

# Real-time Monitoring Dashboard for Multi-Exchange OMS

# Colors
GREEN='\033[0;32m'
RED='\033[0;31m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m'
BOLD='\033[1m'

# Clear screen and hide cursor
clear
tput civis

# Cleanup on exit
trap 'tput cnorm; exit' INT TERM

while true; do
    # Move cursor to top
    tput cup 0 0
    
    # Header
    echo -e "${BOLD}${CYAN}╔════════════════════════════════════════════════════════════════════════╗${NC}"
    echo -e "${BOLD}${CYAN}║           Multi-Exchange OMS - Real-time Monitor Dashboard             ║${NC}"
    echo -e "${BOLD}${CYAN}╠════════════════════════════════════════════════════════════════════════╣${NC}"
    echo -e "${BOLD}${CYAN}║${NC} Time: $(date '+%Y-%m-%d %H:%M:%S') | Uptime: $(uptime -p | sed 's/up //')${CYAN}║${NC}"
    echo -e "${BOLD}${CYAN}╚════════════════════════════════════════════════════════════════════════╝${NC}"
    echo ""
    
    # Service Status
    echo -e "${BOLD}${BLUE}▶ Service Status${NC}"
    echo "┌─────────────────────────┬─────────┬──────────┬──────────┬─────────────┐"
    echo "│ Service                 │ Status  │ PID      │ CPU %    │ Memory      │"
    echo "├─────────────────────────┼─────────┼──────────┼──────────┼─────────────┤"
    
    # Check each service
    services=("oms-core" "oms-server" "binance-spot" "binance-futures")
    for service in "${services[@]}"; do
        if pgrep -f "$service" > /dev/null; then
            pid=$(pgrep -f "$service" | head -1)
            stats=$(ps -p $pid -o %cpu,%mem,rss --no-headers)
            cpu=$(echo $stats | awk '{printf "%.1f", $1}')
            mem=$(echo $stats | awk '{printf "%.1f", $2}')
            rss=$(echo $stats | awk '{printf "%.1f", $3/1024}')
            
            printf "│ %-23s │ ${GREEN}%-7s${NC} │ %-8s │ %6s%% │ %6sMB │\n" \
                   "$service" "ONLINE" "$pid" "$cpu" "$rss"
        else
            printf "│ %-23s │ ${RED}%-7s${NC} │ %-8s │ %8s │ %11s │\n" \
                   "$service" "OFFLINE" "-" "-" "-"
        fi
    done
    echo "└─────────────────────────┴─────────┴──────────┴──────────┴─────────────┘"
    echo ""
    
    # Performance Metrics from logs
    echo -e "${BOLD}${BLUE}▶ Performance Metrics${NC}"
    echo "┌────────────────────────────────────┬──────────────────────────────────┐"
    
    # Parse C++ core stats
    if [ -f logs/oms-core.log ]; then
        risk_checks=$(tail -50 logs/oms-core.log | grep "Risk checks:" | tail -1 | sed 's/.*Risk checks: \([0-9]*\).*/\1/')
        risk_latency=$(tail -50 logs/oms-core.log | grep "Risk checks:" | tail -1 | sed 's/.*avg latency: \([0-9.]*\).*/\1/')
        arb_opps=$(tail -50 logs/oms-core.log | grep "Arbitrage opportunities:" | tail -1 | sed 's/.*opportunities: \([0-9]*\).*/\1/')
        mm_quotes=$(tail -50 logs/oms-core.log | grep "Market maker quotes:" | tail -1 | sed 's/.*quotes: \([0-9]*\).*/\1/')
        
        printf "│ %-34s │ %32s │\n" "Risk Checks" "${risk_checks:-0}"
        printf "│ %-34s │ %29s μs │\n" "Risk Check Latency" "${risk_latency:-0}"
        printf "│ %-34s │ %32s │\n" "Arbitrage Opportunities" "${arb_opps:-0}"
        printf "│ %-34s │ %32s │\n" "Market Maker Quotes" "${mm_quotes:-0}"
    fi
    echo "└────────────────────────────────────┴──────────────────────────────────┘"
    echo ""
    
    # System Resources
    echo -e "${BOLD}${BLUE}▶ System Resources${NC}"
    echo "┌────────────────────────────────────┬──────────────────────────────────┐"
    
    # CPU usage
    cpu_usage=$(top -bn1 | grep "Cpu(s)" | awk '{print $2}' | cut -d'%' -f1)
    cpu_bar=$(perl -e "print '█' x int($cpu_usage/5); print '░' x (20-int($cpu_usage/5))")
    printf "│ CPU Usage                          │ [%-20s] %5.1f%% │\n" "$cpu_bar" "$cpu_usage"
    
    # Memory usage
    mem_info=$(free -m | grep Mem)
    mem_total=$(echo $mem_info | awk '{print $2}')
    mem_used=$(echo $mem_info | awk '{print $3}')
    mem_percent=$(echo "scale=1; $mem_used * 100 / $mem_total" | bc)
    mem_bar=$(perl -e "print '█' x int($mem_percent/5); print '░' x (20-int($mem_percent/5))")
    printf "│ Memory Usage                       │ [%-20s] %5.1f%% │\n" "$mem_bar" "$mem_percent"
    
    # Network connections
    connections=$(ss -tn state established | wc -l)
    printf "│ Network Connections                │ %32d │\n" "$connections"
    
    echo "└────────────────────────────────────┴──────────────────────────────────┘"
    echo ""
    
    # Recent Activity
    echo -e "${BOLD}${BLUE}▶ Recent Activity${NC}"
    echo "┌────────────────────────────────────────────────────────────────────────┐"
    
    # Show last few heartbeats
    if [ -f logs/binance-spot.log ]; then
        spot_last=$(tail -1 logs/binance-spot.log | cut -d' ' -f1,2)
        printf "│ Binance Spot   : Last heartbeat at %-35s │\n" "$spot_last"
    fi
    
    if [ -f logs/binance-futures.log ]; then
        futures_last=$(tail -1 logs/binance-futures.log | cut -d' ' -f1,2)
        printf "│ Binance Futures: Last heartbeat at %-35s │\n" "$futures_last"
    fi
    
    echo "└────────────────────────────────────────────────────────────────────────┘"
    echo ""
    
    # Footer
    echo -e "${CYAN}Press Ctrl+C to exit${NC}"
    
    # Refresh every 2 seconds
    sleep 2
done