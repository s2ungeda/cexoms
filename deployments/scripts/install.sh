#!/bin/bash
#
# OMS Production Installation Script
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
OMS_USER="oms"
OMS_GROUP="oms"
OMS_HOME="/opt/oms"
SYSTEMD_DIR="/etc/systemd/system"
NATS_VERSION="2.10.5"
VAULT_VERSION="1.15.2"

# Functions
log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

check_root() {
    if [[ $EUID -ne 0 ]]; then
        log_error "This script must be run as root"
        exit 1
    fi
}

create_user() {
    log_info "Creating OMS user and group..."
    
    if ! id -u ${OMS_USER} &>/dev/null; then
        useradd -r -s /bin/false -d ${OMS_HOME} -m ${OMS_USER}
        log_info "Created user: ${OMS_USER}"
    else
        log_warn "User ${OMS_USER} already exists"
    fi
}

create_directories() {
    log_info "Creating directory structure..."
    
    directories=(
        "${OMS_HOME}/bin"
        "${OMS_HOME}/configs"
        "${OMS_HOME}/data/logs"
        "${OMS_HOME}/data/snapshots"
        "${OMS_HOME}/data/reports"
        "${OMS_HOME}/certs"
        "/etc/nats"
        "/var/log/oms"
    )
    
    for dir in "${directories[@]}"; do
        mkdir -p "$dir"
        chown ${OMS_USER}:${OMS_GROUP} "$dir"
    done
    
    # Create shared memory directory
    mkdir -p /dev/shm/oms
    chown ${OMS_USER}:${OMS_GROUP} /dev/shm/oms
    chmod 750 /dev/shm/oms
}

install_dependencies() {
    log_info "Installing system dependencies..."
    
    apt-get update
    apt-get install -y \
        build-essential \
        cmake \
        libssl-dev \
        libprotobuf-dev \
        protobuf-compiler \
        jq \
        htop \
        iotop \
        sysstat
}

install_nats() {
    log_info "Installing NATS server..."
    
    if ! command -v nats-server &> /dev/null; then
        curl -L https://github.com/nats-io/nats-server/releases/download/v${NATS_VERSION}/nats-server-v${NATS_VERSION}-linux-amd64.tar.gz \
            -o /tmp/nats-server.tar.gz
        
        tar -xzf /tmp/nats-server.tar.gz -C /tmp
        mv /tmp/nats-server-v${NATS_VERSION}-linux-amd64/nats-server /usr/local/bin/
        chmod +x /usr/local/bin/nats-server
        
        # Create NATS user
        useradd -r -s /bin/false nats || true
        
        log_info "NATS server installed"
    else
        log_warn "NATS server already installed"
    fi
}

install_vault() {
    log_info "Installing HashiCorp Vault..."
    
    if ! command -v vault &> /dev/null; then
        curl -L https://releases.hashicorp.com/vault/${VAULT_VERSION}/vault_${VAULT_VERSION}_linux_amd64.zip \
            -o /tmp/vault.zip
        
        unzip -o /tmp/vault.zip -d /usr/local/bin
        chmod +x /usr/local/bin/vault
        
        # Set capabilities for mlock
        setcap cap_ipc_lock=+ep /usr/local/bin/vault
        
        log_info "Vault installed"
    else
        log_warn "Vault already installed"
    fi
}

copy_binaries() {
    log_info "Copying OMS binaries..."
    
    if [ ! -d "./bin" ]; then
        log_error "bin directory not found. Please build the project first."
        exit 1
    fi
    
    cp -f ./bin/* ${OMS_HOME}/bin/
    chown -R ${OMS_USER}:${OMS_GROUP} ${OMS_HOME}/bin/
    chmod +x ${OMS_HOME}/bin/*
}

copy_configs() {
    log_info "Copying configuration files..."
    
    # Copy main configs
    cp -r ./configs/* ${OMS_HOME}/configs/
    
    # Copy NATS config
    cat > /etc/nats/oms-nats.conf << EOF
# NATS Configuration for OMS
port: 4222
http_port: 8222

# JetStream
jetstream {
    store_dir: "/var/lib/nats/jetstream"
    max_memory_store: 1GB
    max_file_store: 10GB
}

# Logging
log_file: "/var/log/nats/oms-nats.log"
log_size_limit: 100MB
max_traced_msg_len: 10000

# Performance
max_payload: 8MB
max_connections: 10000
write_deadline: "10s"

# TLS (uncomment for production)
# tls {
#     cert_file: "/opt/oms/certs/server.crt"
#     key_file: "/opt/oms/certs/server.key"
# }
EOF
    
    chown -R ${OMS_USER}:${OMS_GROUP} ${OMS_HOME}/configs/
}

install_systemd_services() {
    log_info "Installing systemd services..."
    
    # Copy service files
    cp ./deployments/systemd/*.service ${SYSTEMD_DIR}/
    
    # Reload systemd
    systemctl daemon-reload
    
    # Enable services
    services=(
        "nats"
        "oms-engine"
        "oms-binance-spot"
        "oms-grpc-gateway"
    )
    
    for service in "${services[@]}"; do
        systemctl enable ${service}.service
        log_info "Enabled ${service}.service"
    done
}

setup_log_rotation() {
    log_info "Setting up log rotation..."
    
    cat > /etc/logrotate.d/oms << EOF
/var/log/oms/*.log {
    daily
    rotate 14
    compress
    delaycompress
    missingok
    notifempty
    create 0640 ${OMS_USER} ${OMS_GROUP}
    sharedscripts
    postrotate
        systemctl reload oms-engine || true
    endscript
}

${OMS_HOME}/data/logs/*.log {
    daily
    rotate 30
    compress
    delaycompress
    missingok
    notifempty
    create 0640 ${OMS_USER} ${OMS_GROUP}
}
EOF
}

setup_monitoring() {
    log_info "Setting up monitoring scripts..."
    
    # Create monitoring script
    cat > ${OMS_HOME}/bin/check-health.sh << 'EOF'
#!/bin/bash

# Check if all services are running
services=("nats" "oms-engine" "oms-binance-spot" "oms-grpc-gateway")
failed=0

for service in "${services[@]}"; do
    if ! systemctl is-active --quiet ${service}.service; then
        echo "CRITICAL: ${service} is not running"
        failed=$((failed + 1))
    fi
done

# Check NATS connectivity
if ! timeout 2 nc -z localhost 4222; then
    echo "CRITICAL: Cannot connect to NATS"
    failed=$((failed + 1))
fi

# Check gRPC gateway
if ! timeout 2 nc -z localhost 50051; then
    echo "WARNING: Cannot connect to gRPC gateway"
fi

if [ $failed -eq 0 ]; then
    echo "OK: All services running"
    exit 0
else
    exit 2
fi
EOF
    
    chmod +x ${OMS_HOME}/bin/check-health.sh
    chown ${OMS_USER}:${OMS_GROUP} ${OMS_HOME}/bin/check-health.sh
}

setup_firewall() {
    log_info "Setting up firewall rules..."
    
    if command -v ufw &> /dev/null; then
        # Allow SSH
        ufw allow 22/tcp
        
        # Allow gRPC
        ufw allow 50051/tcp
        
        # Allow NATS monitoring
        ufw allow from 10.0.0.0/8 to any port 8222
        
        # Enable firewall
        ufw --force enable
        
        log_info "Firewall configured"
    else
        log_warn "UFW not found, skipping firewall setup"
    fi
}

print_summary() {
    echo
    echo "========================================"
    echo "       OMS Installation Complete        "
    echo "========================================"
    echo
    echo "Installation directory: ${OMS_HOME}"
    echo "Configuration files: ${OMS_HOME}/configs/"
    echo "Log files: /var/log/oms/"
    echo
    echo "To start services:"
    echo "  systemctl start nats"
    echo "  systemctl start oms-engine"
    echo "  systemctl start oms-binance-spot"
    echo "  systemctl start oms-grpc-gateway"
    echo
    echo "To check status:"
    echo "  systemctl status oms-*"
    echo "  ${OMS_HOME}/bin/check-health.sh"
    echo
    echo "Next steps:"
    echo "1. Configure API keys in Vault"
    echo "2. Update configs in ${OMS_HOME}/configs/"
    echo "3. Generate TLS certificates"
    echo "4. Start the services"
    echo
}

# Main installation flow
main() {
    log_info "Starting OMS installation..."
    
    check_root
    create_user
    create_directories
    install_dependencies
    install_nats
    install_vault
    copy_binaries
    copy_configs
    install_systemd_services
    setup_log_rotation
    setup_monitoring
    setup_firewall
    
    print_summary
}

# Run main function
main "$@"