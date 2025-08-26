#!/bin/bash

# Container orchestration script for mExOms

set -e

# Configuration
ACTION=${1:-status}
SERVICE=${2:-all}
TIMEOUT=${TIMEOUT:-300}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Service dependencies
declare -A SERVICE_DEPS=(
    ["oms-server"]="postgres redis nats vault"
    ["binance-spot"]="oms-server redis nats"
    ["binance-futures"]="oms-server redis nats"
    ["monitor"]="oms-server prometheus"
)

# Service health endpoints
declare -A HEALTH_ENDPOINTS=(
    ["oms-server"]="http://localhost:8081/health"
    ["binance-spot"]="http://localhost:8083/health"
    ["binance-futures"]="http://localhost:8084/health"
    ["monitor"]="http://localhost:8082/health"
    ["postgres"]="pg_isready -U mexoms"
    ["redis"]="redis-cli ping"
    ["nats"]="nc -zv localhost 4222"
)

# Logging functions
log() { echo -e "${GREEN}[ORCH]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
warning() { echo -e "${YELLOW}[WARN]${NC} $1"; }
info() { echo -e "${BLUE}[INFO]${NC} $1"; }

# Check if service is healthy
is_healthy() {
    local service=$1
    local endpoint=${HEALTH_ENDPOINTS[$service]}
    
    if [[ -z "$endpoint" ]]; then
        return 0  # No health check defined
    fi
    
    case "$service" in
        postgres|redis|nats)
            docker-compose exec -T "$service" $endpoint &>/dev/null
            ;;
        *)
            curl -sf "$endpoint" &>/dev/null
            ;;
    esac
}

# Wait for service to be healthy
wait_for_service() {
    local service=$1
    local timeout=${2:-60}
    local elapsed=0
    
    info "Waiting for $service to be healthy..."
    
    while ! is_healthy "$service"; do
        if [ $elapsed -ge $timeout ]; then
            error "$service failed to become healthy within $timeout seconds"
        fi
        
        sleep 1
        ((elapsed++))
        
        if [ $((elapsed % 10)) -eq 0 ]; then
            warning "$service still not healthy after $elapsed seconds"
        fi
    done
    
    log "$service is healthy"
}

# Start service with dependencies
start_service() {
    local service=$1
    
    # Start dependencies first
    local deps=${SERVICE_DEPS[$service]}
    if [[ -n "$deps" ]]; then
        for dep in $deps; do
            if ! docker-compose ps | grep -q "$dep.*Up"; then
                log "Starting dependency: $dep"
                start_service "$dep"
            fi
        done
    fi
    
    # Start the service
    log "Starting $service..."
    docker-compose up -d "$service"
    
    # Wait for health
    wait_for_service "$service"
}

# Stop service gracefully
stop_service() {
    local service=$1
    local timeout=${2:-30}
    
    log "Stopping $service gracefully..."
    
    # Send SIGTERM
    docker-compose kill -s TERM "$service"
    
    # Wait for graceful shutdown
    local elapsed=0
    while docker-compose ps | grep -q "$service.*Up" && [ $elapsed -lt $timeout ]; do
        sleep 1
        ((elapsed++))
    done
    
    # Force stop if still running
    if docker-compose ps | grep -q "$service.*Up"; then
        warning "$service didn't stop gracefully, forcing..."
        docker-compose stop "$service"
    fi
    
    log "$service stopped"
}

# Restart service with zero downtime
restart_service() {
    local service=$1
    
    if [[ "$service" == "oms-server" ]]; then
        # Special handling for main server
        rolling_restart_server
    else
        # Standard restart
        stop_service "$service"
        start_service "$service"
    fi
}

# Rolling restart for OMS server
rolling_restart_server() {
    log "Performing rolling restart of OMS server..."
    
    # Scale up
    docker-compose up -d --scale oms-server=2
    sleep 10
    
    # Get container IDs
    local containers=$(docker-compose ps -q oms-server)
    local first=$(echo "$containers" | head -1)
    
    # Stop first instance
    docker stop "$first"
    wait_for_service "oms-server"
    
    # Remove old instance
    docker rm "$first"
    
    # Scale back down
    docker-compose up -d --scale oms-server=1
    
    log "Rolling restart completed"
}

# Show service status
show_status() {
    echo -e "\n${BLUE}mExOms Service Status${NC}"
    echo "====================="
    
    local services="postgres redis nats vault oms-server binance-spot binance-futures monitor prometheus grafana"
    
    for service in $services; do
        printf "%-20s" "$service:"
        
        if docker-compose ps | grep -q "$service.*Up"; then
            if is_healthy "$service"; then
                echo -e "${GREEN}Running (Healthy)${NC}"
            else
                echo -e "${YELLOW}Running (Unhealthy)${NC}"
            fi
        else
            echo -e "${RED}Stopped${NC}"
        fi
    done
    
    echo -e "\n${BLUE}Resource Usage${NC}"
    echo "=============="
    docker stats --no-stream --format "table {{.Name}}\t{{.CPUPerc}}\t{{.MemUsage}}\t{{.NetIO}}" | grep mexoms
}

# Scale service
scale_service() {
    local service=$1
    local count=$2
    
    if [[ -z "$count" ]]; then
        error "Scale count not specified"
    fi
    
    log "Scaling $service to $count instances..."
    docker-compose up -d --scale "$service=$count"
    
    # Wait for all instances
    for i in $(seq 1 "$count"); do
        wait_for_service "$service"
    done
}

# Backup service data
backup_service() {
    local service=$1
    local backup_dir="backups/$(date +%Y%m%d-%H%M%S)"
    
    mkdir -p "$backup_dir"
    
    case "$service" in
        postgres)
            log "Backing up PostgreSQL..."
            docker-compose exec -T postgres pg_dumpall -U mexoms > "$backup_dir/postgres.sql"
            ;;
        redis)
            log "Backing up Redis..."
            docker-compose exec -T redis redis-cli BGSAVE
            sleep 2
            docker cp "$(docker-compose ps -q redis)":/data/dump.rdb "$backup_dir/redis.rdb"
            ;;
        all)
            backup_service postgres
            backup_service redis
            ;;
        *)
            warning "No backup procedure for $service"
            ;;
    esac
    
    log "Backup saved to $backup_dir"
}

# Execute command in service
exec_service() {
    local service=$1
    shift
    local cmd="$@"
    
    log "Executing in $service: $cmd"
    docker-compose exec "$service" $cmd
}

# Main action handler
case "$ACTION" in
    start)
        if [[ "$SERVICE" == "all" ]]; then
            log "Starting all services..."
            docker-compose up -d
            
            # Wait for critical services
            for service in postgres redis nats vault oms-server; do
                wait_for_service "$service"
            done
        else
            start_service "$SERVICE"
        fi
        ;;
        
    stop)
        if [[ "$SERVICE" == "all" ]]; then
            log "Stopping all services..."
            docker-compose down
        else
            stop_service "$SERVICE"
        fi
        ;;
        
    restart)
        if [[ "$SERVICE" == "all" ]]; then
            $0 stop all
            $0 start all
        else
            restart_service "$SERVICE"
        fi
        ;;
        
    status)
        show_status
        ;;
        
    scale)
        scale_service "$SERVICE" "$3"
        ;;
        
    backup)
        backup_service "$SERVICE"
        ;;
        
    exec)
        shift 2
        exec_service "$SERVICE" "$@"
        ;;
        
    logs)
        if [[ "$SERVICE" == "all" ]]; then
            docker-compose logs -f
        else
            docker-compose logs -f "$SERVICE"
        fi
        ;;
        
    health)
        if [[ "$SERVICE" == "all" ]]; then
            for service in postgres redis nats vault oms-server binance-spot binance-futures monitor; do
                printf "%-20s" "$service:"
                is_healthy "$service" && echo -e "${GREEN}Healthy${NC}" || echo -e "${RED}Unhealthy${NC}"
            done
        else
            is_healthy "$SERVICE" && echo -e "${GREEN}$SERVICE is healthy${NC}" || echo -e "${RED}$SERVICE is unhealthy${NC}"
        fi
        ;;
        
    *)
        echo "Usage: $0 {start|stop|restart|status|scale|backup|exec|logs|health} [service] [options]"
        echo ""
        echo "Services: all, postgres, redis, nats, vault, oms-server, binance-spot, binance-futures, monitor"
        echo ""
        echo "Examples:"
        echo "  $0 start all              # Start all services"
        echo "  $0 stop oms-server        # Stop OMS server"
        echo "  $0 restart binance-spot   # Restart Binance Spot connector"
        echo "  $0 scale oms-server 3     # Scale OMS server to 3 instances"
        echo "  $0 backup postgres        # Backup PostgreSQL"
        echo "  $0 exec redis redis-cli   # Execute redis-cli in Redis container"
        echo "  $0 health all             # Check health of all services"
        exit 1
        ;;
esac