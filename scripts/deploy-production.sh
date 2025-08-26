#!/bin/bash

# Production deployment script for mExOms

set -e

# Configuration
ENVIRONMENT=${1:-production}
DEPLOY_MODE=${2:-rolling} # rolling or blue-green
NAMESPACE=${NAMESPACE:-mexoms}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Logging functions
log() {
    echo -e "${GREEN}[DEPLOY]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
    exit 1
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

info() {
    echo -e "${BLUE}[INFO]${NC} $(date '+%Y-%m-%d %H:%M:%S') - $1"
}

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    # Check Docker
    if ! command -v docker &> /dev/null; then
        error "Docker is not installed"
    fi
    
    # Check docker-compose
    if ! command -v docker-compose &> /dev/null; then
        error "docker-compose is not installed"
    fi
    
    # Check environment file
    if [ ! -f ".env.$ENVIRONMENT" ]; then
        error "Environment file .env.$ENVIRONMENT not found"
    fi
    
    log "Prerequisites check passed"
}

# Backup current deployment
backup_current() {
    log "Creating backup of current deployment..."
    
    BACKUP_DIR="backups/deploy-$(date +%Y%m%d-%H%M%S)"
    mkdir -p "$BACKUP_DIR"
    
    # Backup database
    docker-compose exec -T postgres pg_dump -U mexoms mexoms > "$BACKUP_DIR/database.sql"
    
    # Backup configuration
    cp -r configs "$BACKUP_DIR/"
    
    # Backup docker-compose files
    cp docker-compose*.yml "$BACKUP_DIR/"
    
    log "Backup created at $BACKUP_DIR"
}

# Health check function
health_check() {
    local service=$1
    local max_attempts=${2:-30}
    local attempt=1
    
    info "Checking health of $service..."
    
    while [ $attempt -le $max_attempts ]; do
        if docker-compose exec -T "$service" wget -q -O- http://localhost:8081/health 2>/dev/null; then
            log "$service is healthy"
            return 0
        fi
        
        warning "Health check attempt $attempt/$max_attempts failed for $service"
        sleep 5
        ((attempt++))
    done
    
    error "$service failed health check after $max_attempts attempts"
}

# Rolling deployment
rolling_deployment() {
    log "Starting rolling deployment..."
    
    # Pull latest images
    log "Pulling latest images..."
    docker-compose -f docker-compose.prod.yml pull
    
    # Update services one by one
    SERVICES="postgres redis nats vault oms-server binance-spot binance-futures monitor"
    
    for service in $SERVICES; do
        log "Updating $service..."
        
        # Stop old container
        docker-compose -f docker-compose.prod.yml stop "$service"
        
        # Remove old container
        docker-compose -f docker-compose.prod.yml rm -f "$service"
        
        # Start new container
        docker-compose -f docker-compose.prod.yml up -d "$service"
        
        # Wait for health check
        health_check "$service"
        
        log "$service updated successfully"
        sleep 5
    done
}

# Blue-green deployment
blue_green_deployment() {
    log "Starting blue-green deployment..."
    
    # Determine current and new environment
    CURRENT_COLOR=$(docker-compose ps | grep -q "mexoms-blue" && echo "blue" || echo "green")
    NEW_COLOR=$([[ "$CURRENT_COLOR" == "blue" ]] && echo "green" || echo "blue")
    
    log "Current environment: $CURRENT_COLOR, deploying to: $NEW_COLOR"
    
    # Start new environment
    docker-compose -f "docker-compose.$NEW_COLOR.yml" up -d
    
    # Wait for all services to be healthy
    for service in oms-server binance-spot binance-futures monitor; do
        health_check "$service-$NEW_COLOR"
    done
    
    # Run smoke tests
    run_smoke_tests "$NEW_COLOR"
    
    # Switch traffic to new environment
    log "Switching traffic to $NEW_COLOR environment..."
    # This would typically involve updating load balancer or proxy configuration
    
    # Stop old environment after successful switch
    sleep 30
    log "Stopping $CURRENT_COLOR environment..."
    docker-compose -f "docker-compose.$CURRENT_COLOR.yml" down
}

# Run smoke tests
run_smoke_tests() {
    local env_suffix=${1:-""}
    log "Running smoke tests..."
    
    # Test API endpoints
    endpoints=(
        "http://localhost:8080/api/v1/health"
        "http://localhost:8081/health"
        "http://localhost:8081/metrics"
        "http://localhost:9090/health"
    )
    
    for endpoint in "${endpoints[@]}"; do
        if curl -sf "$endpoint" > /dev/null; then
            log "✓ $endpoint is accessible"
        else
            error "✗ $endpoint is not accessible"
        fi
    done
    
    # Test NATS connection
    docker-compose exec -T oms-server nc -zv nats 4222 || error "NATS connection failed"
    
    # Test Redis connection
    docker-compose exec -T redis redis-cli ping || error "Redis connection failed"
    
    # Test PostgreSQL connection
    docker-compose exec -T postgres pg_isready -U mexoms || error "PostgreSQL connection failed"
    
    log "Smoke tests passed"
}

# Post-deployment tasks
post_deployment() {
    log "Running post-deployment tasks..."
    
    # Run database migrations
    log "Running database migrations..."
    docker-compose exec -T oms-server /app/bin/migrate up
    
    # Warm up caches
    log "Warming up caches..."
    docker-compose exec -T oms-server /app/bin/cache-warmer
    
    # Verify all strategies are loaded
    log "Verifying strategies..."
    docker-compose exec -T oms-server /app/bin/strategy-check
    
    # Send deployment notification
    send_notification "Deployment completed successfully"
}

# Send notification
send_notification() {
    local message=$1
    log "Sending notification: $message"
    
    # Implement your notification method here
    # Example: Slack, Discord, email, etc.
}

# Rollback deployment
rollback() {
    error "Deployment failed, starting rollback..."
    
    # Find latest backup
    LATEST_BACKUP=$(ls -t backups/deploy-* | head -1)
    
    if [ -z "$LATEST_BACKUP" ]; then
        error "No backup found for rollback"
    fi
    
    log "Rolling back to $LATEST_BACKUP..."
    
    # Stop current deployment
    docker-compose -f docker-compose.prod.yml down
    
    # Restore configuration
    cp -r "$LATEST_BACKUP/configs" .
    
    # Restore database
    docker-compose -f docker-compose.prod.yml up -d postgres
    sleep 10
    docker-compose exec -T postgres psql -U mexoms mexoms < "$LATEST_BACKUP/database.sql"
    
    # Start previous version
    docker-compose -f docker-compose.prod.yml up -d
    
    send_notification "Deployment rolled back due to failure"
}

# Main deployment flow
main() {
    log "Starting deployment to $ENVIRONMENT using $DEPLOY_MODE mode"
    
    # Set up error handling
    trap rollback ERR
    
    # Check prerequisites
    check_prerequisites
    
    # Load environment
    export $(cat ".env.$ENVIRONMENT" | xargs)
    
    # Create backup
    backup_current
    
    # Deploy based on mode
    case "$DEPLOY_MODE" in
        "rolling")
            rolling_deployment
            ;;
        "blue-green")
            blue_green_deployment
            ;;
        *)
            error "Unknown deployment mode: $DEPLOY_MODE"
            ;;
    esac
    
    # Run post-deployment tasks
    post_deployment
    
    # Final health check
    run_smoke_tests
    
    log "Deployment completed successfully!"
    
    # Remove error trap
    trap - ERR
}

# Run main function
main "$@"