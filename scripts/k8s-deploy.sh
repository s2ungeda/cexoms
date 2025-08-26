#!/bin/bash

# Kubernetes deployment script for mExOms

set -e

# Configuration
NAMESPACE=${NAMESPACE:-mexoms}
ENVIRONMENT=${ENVIRONMENT:-production}
ACTION=${1:-apply}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

# Logging functions
log() { echo -e "${GREEN}[K8S]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }
info() { echo -e "${BLUE}[INFO]${NC} $1"; }

# Check prerequisites
check_prerequisites() {
    log "Checking prerequisites..."
    
    if ! command -v kubectl &> /dev/null; then
        error "kubectl is not installed"
    fi
    
    if ! kubectl cluster-info &> /dev/null; then
        error "Not connected to a Kubernetes cluster"
    fi
    
    if ! command -v helm &> /dev/null; then
        warning "Helm is not installed, some features may not work"
    fi
    
    log "Prerequisites check passed"
}

# Create namespace
create_namespace() {
    log "Creating namespace..."
    kubectl apply -f k8s/namespace.yaml
}

# Deploy secrets
deploy_secrets() {
    log "Deploying secrets..."
    
    # Check if secrets already exist
    if kubectl get secret mexoms-secrets -n $NAMESPACE &> /dev/null; then
        warning "Secrets already exist, skipping creation"
    else
        info "Creating secrets from environment..."
        kubectl create secret generic mexoms-secrets \
            --from-literal=postgres-user=$POSTGRES_USER \
            --from-literal=postgres-password=$POSTGRES_PASSWORD \
            --from-literal=redis-password=$REDIS_PASSWORD \
            --from-literal=jwt-secret=$JWT_SECRET \
            --from-literal=vault-token=$VAULT_TOKEN \
            --from-literal=binance-api-key=$BINANCE_API_KEY \
            --from-literal=binance-secret-key=$BINANCE_SECRET_KEY \
            -n $NAMESPACE
    fi
}

# Deploy infrastructure
deploy_infrastructure() {
    log "Deploying infrastructure components..."
    
    # Deploy in order of dependencies
    kubectl apply -f k8s/configmap.yaml
    kubectl apply -f k8s/postgres.yaml
    kubectl apply -f k8s/redis.yaml
    kubectl apply -f k8s/nats.yaml
    kubectl apply -f k8s/vault.yaml
    
    # Wait for infrastructure to be ready
    log "Waiting for infrastructure to be ready..."
    kubectl wait --for=condition=ready pod -l app=postgres -n $NAMESPACE --timeout=300s
    kubectl wait --for=condition=ready pod -l app=redis -n $NAMESPACE --timeout=300s
    kubectl wait --for=condition=ready pod -l app=nats -n $NAMESPACE --timeout=300s
}

# Deploy applications
deploy_applications() {
    log "Deploying application components..."
    
    kubectl apply -f k8s/oms-server.yaml
    kubectl apply -f k8s/binance-connectors.yaml
    
    # Wait for applications to be ready
    log "Waiting for applications to be ready..."
    kubectl wait --for=condition=ready pod -l app=oms-server -n $NAMESPACE --timeout=300s
    kubectl wait --for=condition=ready pod -l app=binance-spot -n $NAMESPACE --timeout=300s
    kubectl wait --for=condition=ready pod -l app=binance-futures -n $NAMESPACE --timeout=300s
}

# Deploy monitoring
deploy_monitoring() {
    log "Deploying monitoring components..."
    
    kubectl apply -f k8s/monitoring.yaml
    
    # Wait for monitoring to be ready
    log "Waiting for monitoring to be ready..."
    kubectl wait --for=condition=ready pod -l app=prometheus -n $NAMESPACE --timeout=300s
    kubectl wait --for=condition=ready pod -l app=grafana -n $NAMESPACE --timeout=300s
}

# Deploy ingress
deploy_ingress() {
    log "Deploying ingress..."
    
    kubectl apply -f k8s/ingress.yaml
}

# Check deployment status
check_status() {
    log "Checking deployment status..."
    
    echo -e "\n${BLUE}Pods:${NC}"
    kubectl get pods -n $NAMESPACE
    
    echo -e "\n${BLUE}Services:${NC}"
    kubectl get services -n $NAMESPACE
    
    echo -e "\n${BLUE}Ingresses:${NC}"
    kubectl get ingress -n $NAMESPACE
    
    echo -e "\n${BLUE}PVCs:${NC}"
    kubectl get pvc -n $NAMESPACE
    
    echo -e "\n${BLUE}HPAs:${NC}"
    kubectl get hpa -n $NAMESPACE
}

# Port forward for local access
port_forward() {
    log "Setting up port forwarding..."
    
    info "OMS API: http://localhost:8080"
    kubectl port-forward -n $NAMESPACE service/oms-server-service 8080:8080 &
    
    info "Prometheus: http://localhost:9090"
    kubectl port-forward -n $NAMESPACE service/prometheus-service 9090:9090 &
    
    info "Grafana: http://localhost:3000"
    kubectl port-forward -n $NAMESPACE service/grafana-service 3000:3000 &
    
    info "Press Ctrl+C to stop port forwarding"
    wait
}

# Delete deployment
delete_deployment() {
    warning "Deleting deployment..."
    
    kubectl delete -f k8s/ingress.yaml || true
    kubectl delete -f k8s/monitoring.yaml || true
    kubectl delete -f k8s/binance-connectors.yaml || true
    kubectl delete -f k8s/oms-server.yaml || true
    kubectl delete -f k8s/vault.yaml || true
    kubectl delete -f k8s/nats.yaml || true
    kubectl delete -f k8s/redis.yaml || true
    kubectl delete -f k8s/postgres.yaml || true
    kubectl delete -f k8s/configmap.yaml || true
    kubectl delete -f k8s/secrets.yaml || true
    kubectl delete -f k8s/namespace.yaml || true
}

# Main deployment flow
main() {
    check_prerequisites
    
    case "$ACTION" in
        apply|deploy)
            log "Starting deployment to $ENVIRONMENT..."
            create_namespace
            deploy_secrets
            deploy_infrastructure
            deploy_applications
            deploy_monitoring
            deploy_ingress
            check_status
            log "Deployment completed successfully!"
            ;;
        status)
            check_status
            ;;
        port-forward)
            port_forward
            ;;
        delete)
            delete_deployment
            ;;
        *)
            echo "Usage: $0 {apply|deploy|status|port-forward|delete}"
            exit 1
            ;;
    esac
}

# Load environment variables
if [ -f ".env.$ENVIRONMENT" ]; then
    export $(cat ".env.$ENVIRONMENT" | xargs)
else
    warning "Environment file .env.$ENVIRONMENT not found"
fi

# Run main function
main