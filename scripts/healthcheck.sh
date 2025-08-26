#!/bin/sh

# Health check script for OMS services

SERVICE_NAME=${1:-oms-server}
HEALTH_ENDPOINT=${HEALTH_ENDPOINT:-http://localhost:8081/health}

# Function to check service health
check_health() {
    response=$(wget --quiet --tries=1 --spider --timeout=5 "$HEALTH_ENDPOINT" 2>&1)
    if [ $? -eq 0 ]; then
        echo "Service $SERVICE_NAME is healthy"
        exit 0
    else
        echo "Service $SERVICE_NAME is unhealthy: $response"
        exit 1
    fi
}

# Special handling for different services
case "$SERVICE_NAME" in
    "nats")
        nc -z localhost 4222
        exit $?
        ;;
    "redis")
        redis-cli ping > /dev/null 2>&1
        exit $?
        ;;
    "postgres")
        pg_isready -U mexoms > /dev/null 2>&1
        exit $?
        ;;
    *)
        check_health
        ;;
esac