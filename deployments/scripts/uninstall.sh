#!/bin/bash
#
# OMS Uninstallation Script
#

set -euo pipefail

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Configuration
OMS_USER="oms"
OMS_HOME="/opt/oms"
SYSTEMD_DIR="/etc/systemd/system"

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

confirm_uninstall() {
    echo
    log_warn "This will completely remove OMS from your system!"
    log_warn "The following will be deleted:"
    echo "  - All OMS services"
    echo "  - User: ${OMS_USER}"
    echo "  - Directory: ${OMS_HOME}"
    echo "  - Log files: /var/log/oms"
    echo "  - Systemd service files"
    echo
    read -p "Are you sure you want to continue? (yes/no): " confirm
    
    if [ "$confirm" != "yes" ]; then
        log_info "Uninstallation cancelled"
        exit 0
    fi
}

stop_services() {
    log_info "Stopping OMS services..."
    
    services=(
        "oms-engine"
        "oms-binance-spot"
        "oms-binance-futures"
        "oms-grpc-gateway"
        "oms-monitor"
    )
    
    for service in "${services[@]}"; do
        if systemctl is-active --quiet ${service}.service; then
            systemctl stop ${service}.service
            log_info "Stopped ${service}"
        fi
    done
    
    # Stop NATS if no other services depend on it
    if systemctl is-active --quiet nats.service; then
        read -p "Stop NATS server? Other services might depend on it (y/n): " stop_nats
        if [ "$stop_nats" = "y" ]; then
            systemctl stop nats.service
            log_info "Stopped NATS"
        fi
    fi
}

disable_services() {
    log_info "Disabling OMS services..."
    
    services=(
        "oms-engine"
        "oms-binance-spot"
        "oms-binance-futures"
        "oms-grpc-gateway"
        "oms-monitor"
    )
    
    for service in "${services[@]}"; do
        if systemctl is-enabled --quiet ${service}.service 2>/dev/null; then
            systemctl disable ${service}.service
            log_info "Disabled ${service}"
        fi
    done
}

remove_service_files() {
    log_info "Removing systemd service files..."
    
    rm -f ${SYSTEMD_DIR}/oms-*.service
    systemctl daemon-reload
}

backup_data() {
    log_info "Creating backup of data files..."
    
    backup_dir="/tmp/oms-backup-$(date +%Y%m%d-%H%M%S)"
    
    if [ -d "${OMS_HOME}/data" ]; then
        mkdir -p "$backup_dir"
        cp -r ${OMS_HOME}/data "$backup_dir/"
        cp -r ${OMS_HOME}/configs "$backup_dir/"
        
        log_info "Backup created at: $backup_dir"
        log_info "You can restore data from this backup if needed"
    fi
}

remove_files() {
    log_info "Removing OMS files..."
    
    # Remove installation directory
    if [ -d "$OMS_HOME" ]; then
        rm -rf "$OMS_HOME"
        log_info "Removed $OMS_HOME"
    fi
    
    # Remove log files
    if [ -d "/var/log/oms" ]; then
        rm -rf "/var/log/oms"
        log_info "Removed /var/log/oms"
    fi
    
    # Remove shared memory
    if [ -d "/dev/shm/oms" ]; then
        rm -rf "/dev/shm/oms"
        log_info "Removed /dev/shm/oms"
    fi
    
    # Remove logrotate config
    rm -f /etc/logrotate.d/oms
    
    # Remove NATS config (if confirmed)
    if [ -f "/etc/nats/oms-nats.conf" ]; then
        read -p "Remove NATS configuration? (y/n): " remove_nats
        if [ "$remove_nats" = "y" ]; then
            rm -f /etc/nats/oms-nats.conf
            log_info "Removed NATS configuration"
        fi
    fi
}

remove_user() {
    log_info "Removing OMS user..."
    
    if id -u ${OMS_USER} &>/dev/null; then
        userdel ${OMS_USER}
        log_info "Removed user: ${OMS_USER}"
    fi
}

print_summary() {
    echo
    echo "========================================"
    echo "      OMS Uninstallation Complete       "
    echo "========================================"
    echo
    echo "The following have been removed:"
    echo "  - OMS services and binaries"
    echo "  - User: ${OMS_USER}"
    echo "  - Directory: ${OMS_HOME}"
    echo "  - Log files and configurations"
    echo
    if [ -n "${backup_dir:-}" ] && [ -d "$backup_dir" ]; then
        echo "Data backup saved at: $backup_dir"
        echo
    fi
    echo "Note: NATS and Vault were not uninstalled as they"
    echo "      might be used by other applications."
    echo
}

# Main uninstallation flow
main() {
    log_info "Starting OMS uninstallation..."
    
    check_root
    confirm_uninstall
    stop_services
    disable_services
    remove_service_files
    backup_data
    remove_files
    remove_user
    
    print_summary
}

# Run main function
main "$@"