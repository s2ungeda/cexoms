# Administrator Guide

Comprehensive guide for system administrators deploying and managing mExOms infrastructure.

## Table of Contents

1. [System Requirements](#system-requirements)
2. [Installation](#installation)
3. [Configuration](#configuration)
4. [Deployment](#deployment)
5. [Monitoring](#monitoring)
6. [Security](#security)
7. [User Management](#user-management)
8. [Performance Tuning](#performance-tuning)
9. [Backup & Recovery](#backup--recovery)
10. [Troubleshooting](#troubleshooting)

## System Requirements

### Minimum Requirements
```yaml
CPU: 8 cores (Intel/AMD x64)
RAM: 16GB
Storage: 500GB SSD
Network: 1Gbps
OS: Ubuntu 20.04+ / CentOS 8+ / RHEL 8+
```

### Recommended Production
```yaml
CPU: 32 cores (Intel/AMD x64) 
RAM: 128GB
Storage: 2TB NVMe SSD
Network: 10Gbps
OS: Ubuntu 22.04 LTS
Additional: Hardware security module (HSM)
```

### Dependencies
```bash
# Core dependencies
Docker >= 20.10
Docker Compose >= 2.0
Kubernetes >= 1.25 (optional)
PostgreSQL >= 14
Redis >= 6.2
NATS >= 2.8
HashiCorp Vault >= 1.12
```

## Installation

### Docker Installation (Recommended)

#### 1. Clone Repository
```bash
git clone https://github.com/mexoms/mexoms.git
cd mexoms
```

#### 2. Environment Setup
```bash
# Copy environment template
cp .env.example .env

# Generate secure secrets
./scripts/generate-secrets.sh

# Edit configuration
vim .env
```

#### 3. Deploy Infrastructure
```bash
# Start infrastructure services
docker-compose -f docker-compose.infrastructure.yml up -d

# Wait for services to be ready
./scripts/wait-for-services.sh

# Start mExOms services
docker-compose up -d
```

#### 4. Initialize System
```bash
# Run database migrations
./scripts/migrate-database.sh

# Create admin user
./scripts/create-admin-user.sh

# Load exchange configurations
./scripts/load-exchange-configs.sh
```

### Kubernetes Installation

#### 1. Prepare Kubernetes Cluster
```bash
# Create namespace
kubectl create namespace mexoms

# Apply RBAC
kubectl apply -f k8s/rbac.yaml

# Create secrets
kubectl create secret generic mexoms-secrets \
  --from-env-file=.env \
  --namespace=mexoms
```

#### 2. Deploy with Helm
```bash
# Add Helm repository
helm repo add mexoms https://charts.mexoms.com

# Install mExOms
helm install mexoms mexoms/mexoms \
  --namespace mexoms \
  --values values-production.yaml
```

#### 3. Verify Deployment
```bash
# Check pod status
kubectl get pods -n mexoms

# Check services
kubectl get services -n mexoms

# View logs
kubectl logs -f deployment/mexoms-server -n mexoms
```

## Configuration

### Core Configuration

#### Database Configuration
```yaml
# config/database.yaml
database:
  host: postgres
  port: 5432
  name: mexoms
  username: mexoms_user
  password: ${DB_PASSWORD}
  ssl_mode: require
  max_connections: 100
  connection_timeout: 30s
```

#### Cache Configuration
```yaml
# config/cache.yaml  
cache:
  redis:
    host: redis
    port: 6379
    password: ${REDIS_PASSWORD}
    db: 0
    pool_size: 10
    max_retries: 3
```

#### Message Queue Configuration
```yaml
# config/messaging.yaml
messaging:
  nats:
    urls: 
      - nats://nats:4222
    cluster_id: mexoms
    client_id: mexoms-server
    max_reconnects: -1
    reconnect_wait: 2s
```

### Exchange Configuration

#### Binance Configuration
```yaml
# config/exchanges/binance.yaml
binance:
  name: "binance"
  enabled: true
  endpoints:
    spot_api: "https://api.binance.com"
    futures_api: "https://fapi.binance.com" 
    websocket: "wss://stream.binance.com:9443/ws"
  rate_limits:
    requests_per_second: 10
    weight_per_minute: 1200
  features:
    spot_trading: true
    futures_trading: true
    margin_trading: false
```

### Security Configuration

#### Vault Configuration
```yaml
# config/vault.yaml
vault:
  address: "https://vault:8200"
  token: ${VAULT_TOKEN}
  mount_path: "secret"
  key_rotation_interval: "30d"
  auto_unseal: true
```

#### TLS Configuration
```yaml
# config/tls.yaml
tls:
  enabled: true
  cert_file: "/etc/ssl/certs/mexoms.crt"
  key_file: "/etc/ssl/private/mexoms.key"
  ca_file: "/etc/ssl/certs/ca.crt"
  min_version: "1.3"
  cipher_suites:
    - "TLS_AES_256_GCM_SHA384"
    - "TLS_CHACHA20_POLY1305_SHA256"
```

## Deployment

### Production Deployment

#### 1. Infrastructure Preparation
```bash
# Setup load balancer
./scripts/setup-loadbalancer.sh

# Configure firewall
ufw allow 22/tcp      # SSH
ufw allow 443/tcp     # HTTPS
ufw allow 8080/tcp    # gRPC
ufw enable

# Setup monitoring
./scripts/setup-monitoring.sh
```

#### 2. Blue-Green Deployment
```bash
# Deploy to green environment
./scripts/deploy-green.sh

# Run health checks
./scripts/health-check.sh green

# Switch traffic to green
./scripts/switch-traffic.sh green

# Cleanup blue environment
./scripts/cleanup-blue.sh
```

#### 3. Canary Deployment
```bash
# Deploy canary version (5% traffic)
./scripts/deploy-canary.sh --traffic-percent=5

# Monitor metrics
./scripts/monitor-canary.sh

# Promote to full deployment
./scripts/promote-canary.sh
```

### High Availability Setup

#### 1. Multi-Region Deployment
```yaml
# k8s/multi-region.yaml
regions:
  primary:
    name: "us-east-1"
    replicas: 3
    zones:
      - "us-east-1a"
      - "us-east-1b" 
      - "us-east-1c"
  secondary:
    name: "us-west-2"
    replicas: 2
    zones:
      - "us-west-2a"
      - "us-west-2b"
```

#### 2. Database Clustering
```yaml
# config/postgres-cluster.yaml
postgresql:
  cluster:
    enabled: true
    primary:
      host: postgres-primary
      port: 5432
    replicas:
      - host: postgres-replica-1
        port: 5432
      - host: postgres-replica-2
        port: 5432
  failover:
    enabled: true
    check_interval: 10s
    timeout: 30s
```

## Monitoring

### Metrics Collection

#### Prometheus Configuration
```yaml
# monitoring/prometheus.yml
global:
  scrape_interval: 15s
  evaluation_interval: 15s

rule_files:
  - "rules/*.yml"

scrape_configs:
  - job_name: 'mexoms'
    static_configs:
      - targets: ['mexoms:9090']
    metrics_path: '/metrics'
    scrape_interval: 5s
    
  - job_name: 'postgres'
    static_configs:
      - targets: ['postgres-exporter:9187']
```

#### Custom Metrics
```go
// Example: Custom order metrics
var (
    ordersProcessed = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "mexoms_orders_processed_total",
            Help: "Total number of orders processed",
        },
        []string{"exchange", "symbol", "side"},
    )
    
    orderLatency = prometheus.NewHistogramVec(
        prometheus.HistogramOpts{
            Name: "mexoms_order_processing_duration_seconds",
            Help: "Order processing latency",
            Buckets: prometheus.ExponentialBuckets(0.0001, 2, 15),
        },
        []string{"exchange", "type"},
    )
)
```

### Dashboard Setup

#### Grafana Dashboards
```bash
# Import predefined dashboards
./scripts/import-dashboards.sh

# Available dashboards:
# - System Overview
# - Trading Performance  
# - Risk Management
# - Exchange Connectivity
# - Performance Metrics
```

#### Key Monitoring Panels

1. **System Health**
   - CPU/Memory usage
   - Disk I/O
   - Network traffic
   - Service uptime

2. **Trading Metrics** 
   - Orders per second
   - Order processing latency
   - Fill rates
   - P&L tracking

3. **Risk Metrics**
   - Position exposure
   - VaR calculations
   - Drawdown monitoring
   - Correlation analysis

### Alerting

#### AlertManager Configuration
```yaml
# monitoring/alertmanager.yml
global:
  smtp_smarthost: 'localhost:587'
  smtp_from: 'alerts@mexoms.com'

route:
  group_by: ['alertname']
  group_wait: 10s
  group_interval: 10s
  repeat_interval: 1h
  receiver: 'web.hook'

receivers:
  - name: 'web.hook'
    email_configs:
      - to: 'admin@mexoms.com'
        subject: 'mExOms Alert: {{ .GroupLabels.alertname }}'
        body: |
          {{ range .Alerts }}
          Alert: {{ .Annotations.summary }}
          Description: {{ .Annotations.description }}
          {{ end }}
    
    slack_configs:
      - api_url: '${SLACK_API_URL}'
        channel: '#mexoms-alerts'
        title: 'mExOms Alert'
        text: '{{ range .Alerts }}{{ .Annotations.summary }}{{ end }}'
```

#### Alert Rules
```yaml
# monitoring/rules/system.yml
groups:
  - name: system
    rules:
      - alert: HighMemoryUsage
        expr: (node_memory_MemTotal_bytes - node_memory_MemAvailable_bytes) / node_memory_MemTotal_bytes > 0.9
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "High memory usage detected"
          description: "Memory usage is above 90%"
      
      - alert: OrderProcessingLatencyHigh
        expr: histogram_quantile(0.95, mexoms_order_processing_duration_seconds) > 0.1
        for: 2m
        labels:
          severity: critical
        annotations:
          summary: "Order processing latency is high"
          description: "95th percentile latency is above 100ms"
```

## Security

### Authentication & Authorization

#### User Management
```bash
# Create user
./scripts/create-user.sh \
  --username trader1 \
  --email trader1@company.com \
  --role trader

# Set permissions
./scripts/set-permissions.sh \
  --user trader1 \
  --permissions trading,read_positions

# Enable MFA
./scripts/enable-mfa.sh --user trader1
```

#### API Key Management
```bash
# Create API key
./scripts/create-api-key.sh \
  --user trader1 \
  --name "Trading Bot Key" \
  --scopes trading,read_orders \
  --ip-whitelist 203.0.113.0/24

# Rotate keys (automated)
crontab -e
0 0 1 * * /opt/mexoms/scripts/rotate-api-keys.sh
```

### Network Security

#### Firewall Configuration
```bash
# iptables rules
iptables -A INPUT -p tcp --dport 22 -j ACCEPT    # SSH
iptables -A INPUT -p tcp --dport 443 -j ACCEPT   # HTTPS
iptables -A INPUT -p tcp --dport 8080 -j ACCEPT  # gRPC
iptables -A INPUT -j DROP                        # Drop all other traffic

# Save rules
iptables-save > /etc/iptables/rules.v4
```

#### VPN Setup
```bash
# WireGuard VPN for admin access
wg genkey | tee privatekey | wg pubkey > publickey

# Server configuration
cat > /etc/wireguard/wg0.conf << EOF
[Interface]
PrivateKey = $(cat privatekey)
Address = 10.0.0.1/24
ListenPort = 51820

[Peer]
PublicKey = CLIENT_PUBLIC_KEY
AllowedIPs = 10.0.0.2/32
EOF

systemctl enable wg-quick@wg0
systemctl start wg-quick@wg0
```

### Audit Logging

#### Enable Comprehensive Logging
```yaml
# config/logging.yaml
logging:
  level: info
  audit:
    enabled: true
    destinations:
      - file: /var/log/mexoms/audit.log
      - syslog: true
      - elasticsearch:
          url: https://elasticsearch:9200
          index: mexoms-audit
    events:
      - user_login
      - user_logout  
      - order_create
      - order_cancel
      - api_key_create
      - permission_change
```

## User Management

### Role-Based Access Control

#### Define Roles
```yaml
# config/rbac.yaml
roles:
  admin:
    permissions:
      - "*"
    description: "Full system access"
  
  trader:
    permissions:
      - "orders:create"
      - "orders:cancel"
      - "orders:read"
      - "positions:read"
      - "market_data:read"
    description: "Standard trader access"
  
  analyst:
    permissions:
      - "orders:read"
      - "positions:read" 
      - "market_data:read"
      - "analytics:read"
    description: "Read-only analyst access"
```

#### User Provisioning
```bash
#!/bin/bash
# scripts/provision-user.sh

USERNAME=$1
EMAIL=$2
ROLE=$3

# Create user in database
psql -d mexoms -c "
INSERT INTO users (username, email, role, created_at) 
VALUES ('$USERNAME', '$EMAIL', '$ROLE', NOW());
"

# Send welcome email
./scripts/send-welcome-email.sh $EMAIL

# Generate temporary password
TEMP_PASSWORD=$(openssl rand -base64 12)
./scripts/set-password.sh $USERNAME $TEMP_PASSWORD

echo "User $USERNAME created with temporary password: $TEMP_PASSWORD"
```

### Multi-Tenant Management

#### Tenant Isolation
```yaml
# config/tenants.yaml
tenants:
  tenant1:
    name: "Hedge Fund A"
    database_suffix: "_hfa"
    resource_limits:
      max_accounts: 10
      max_orders_per_minute: 1000
    features:
      - multi_exchange
      - advanced_analytics
  
  tenant2:
    name: "Retail Traders"
    database_suffix: "_retail"
    resource_limits:
      max_accounts: 1
      max_orders_per_minute: 100
    features:
      - basic_trading
```

## Performance Tuning

### Database Optimization

#### PostgreSQL Tuning
```sql
-- postgresql.conf optimizations
shared_buffers = 8GB                    # 25% of RAM
effective_cache_size = 24GB             # 75% of RAM  
random_page_cost = 1.1                  # SSD optimization
wal_buffers = 64MB
checkpoint_completion_target = 0.9
max_connections = 200

-- Create indexes for performance
CREATE INDEX CONCURRENTLY idx_orders_symbol_created 
  ON orders(symbol, created_at);
  
CREATE INDEX CONCURRENTLY idx_positions_account_symbol
  ON positions(account_id, symbol);
```

#### Connection Pooling
```yaml
# config/pgbouncer.ini
[databases]
mexoms = host=postgres port=5432 dbname=mexoms

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 50
server_round_robin = 1
```

### Application Performance

#### Go Service Optimization
```yaml
# config/performance.yaml
performance:
  gomaxprocs: 32                    # Match CPU cores
  gc_target_percentage: 100        # GC tuning
  memory_limit: "32GB"
  goroutine_pool_size: 10000
  
  order_processing:
    batch_size: 1000
    flush_interval: "10ms"
    worker_count: 50
    
  market_data:
    buffer_size: 100000
    compression: true
    batch_updates: true
```

#### C++ Core Optimization
```cpp
// Core engine performance settings
#define RING_BUFFER_SIZE 1048576      // 1M entries
#define MAX_ACCOUNTS 1000
#define MAX_SYMBOLS 10000

// CPU affinity for critical threads  
void setCPUAffinity(int core) {
    cpu_set_t cpuset;
    CPU_ZERO(&cpuset);
    CPU_SET(core, &cpuset);
    pthread_setaffinity_np(pthread_self(), sizeof(cpuset), &cpuset);
}

// Dedicated cores for:
// Core 0-3: Order processing
// Core 4-7: Risk management  
// Core 8-11: Market data
// Core 12+: General purpose
```

### Network Optimization

#### Load Balancer Configuration
```nginx
# nginx.conf
upstream mexoms_backend {
    least_conn;
    server mexoms-1:8080 max_fails=3 fail_timeout=30s;
    server mexoms-2:8080 max_fails=3 fail_timeout=30s;
    server mexoms-3:8080 max_fails=3 fail_timeout=30s;
}

server {
    listen 443 ssl http2;
    server_name api.mexoms.com;
    
    # SSL configuration
    ssl_certificate /etc/ssl/certs/mexoms.crt;
    ssl_certificate_key /etc/ssl/private/mexoms.key;
    ssl_protocols TLSv1.3;
    
    # Performance optimizations
    keepalive_timeout 65;
    keepalive_requests 10000;
    
    location / {
        grpc_pass grpc://mexoms_backend;
        grpc_set_header Host $host;
        grpc_set_header X-Real-IP $remote_addr;
    }
}
```

## Backup & Recovery

### Database Backup Strategy

#### Automated Backups
```bash
#!/bin/bash
# scripts/backup-database.sh

BACKUP_DIR="/backup/postgres"
RETENTION_DAYS=30

# Full backup daily
pg_dump mexoms | gzip > $BACKUP_DIR/mexoms_$(date +%Y%m%d).sql.gz

# WAL archiving for point-in-time recovery
rsync -av /var/lib/postgresql/13/main/pg_wal/ $BACKUP_DIR/wal/

# Cleanup old backups
find $BACKUP_DIR -name "*.sql.gz" -mtime +$RETENTION_DAYS -delete

# Upload to S3
aws s3 sync $BACKUP_DIR s3://mexoms-backups/
```

#### Point-in-Time Recovery
```bash
#!/bin/bash
# scripts/restore-database.sh

RESTORE_TIME="2024-01-15 14:30:00"
BACKUP_FILE="/backup/postgres/mexoms_20240115.sql.gz"

# Stop service
systemctl stop mexoms

# Restore base backup
gunzip -c $BACKUP_FILE | psql mexoms

# Apply WAL files until restore time
pg_ctl start -D /var/lib/postgresql/13/main
psql -d mexoms -c "SELECT pg_is_in_recovery();"

# Start service when recovery complete
systemctl start mexoms
```

### Configuration Backup

#### Vault Backup
```bash
# Backup Vault data
vault operator raft snapshot save backup.snap

# Restore Vault data  
vault operator raft snapshot restore backup.snap
```

#### Kubernetes Backup
```bash
# Backup entire namespace
kubectl get all,pv,pvc,secrets,configmaps -o yaml -n mexoms > mexoms-backup.yaml

# Restore from backup
kubectl apply -f mexoms-backup.yaml
```

## Troubleshooting

### Common Issues

#### 1. High Memory Usage
```bash
# Check memory usage
free -h
ps aux --sort=-%mem | head -10

# Analyze heap dumps (Go)
go tool pprof http://localhost:6060/debug/pprof/heap

# Check for memory leaks
valgrind --leak-check=full ./mexoms-core
```

#### 2. Database Connection Issues
```bash
# Check connection pool
psql -d mexoms -c "
SELECT state, count(*) 
FROM pg_stat_activity 
GROUP BY state;
"

# Monitor long-running queries
psql -d mexoms -c "
SELECT pid, now() - pg_stat_activity.query_start AS duration, query 
FROM pg_stat_activity 
WHERE (now() - pg_stat_activity.query_start) > interval '5 minutes';
"

# Kill problematic connections
SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE state = 'idle in transaction';
```

#### 3. Network Connectivity
```bash
# Test exchange connectivity
curl -I https://api.binance.com/api/v3/ping
nc -zv stream.binance.com 9443

# Check DNS resolution
nslookup api.binance.com
dig api.binance.com

# Monitor network traffic
netstat -tulpn | grep :8080
ss -tulpn | grep :8080
```

### Performance Debugging

#### 1. Order Processing Latency
```bash
# Enable detailed tracing
export MEXOMS_TRACE=order_processing

# Analyze traces
jaeger-query --query.base-path=/jaeger

# Profile CPU usage
go tool pprof http://localhost:6060/debug/pprof/profile?seconds=30
```

#### 2. Memory Profiling
```bash
# Go memory profiling
go tool pprof http://localhost:6060/debug/pprof/heap

# C++ memory profiling
valgrind --tool=massif ./mexoms-core
massif-visualizer massif.out.*
```

### Emergency Procedures

#### 1. Circuit Breaker Activation
```bash
# Stop all trading immediately
./scripts/emergency-stop.sh

# Check system status
./scripts/health-check.sh

# Resume trading when safe
./scripts/resume-trading.sh
```

#### 2. Disaster Recovery
```bash
# Failover to backup region
./scripts/failover-to-backup.sh

# Restore from backup
./scripts/restore-from-backup.sh --timestamp="2024-01-15T14:30:00Z"

# Verify system integrity
./scripts/verify-integrity.sh
```

### Log Analysis

#### Log Aggregation
```bash
# Centralized logging with ELK stack
filebeat -e -c filebeat.yml

# Search logs
curl -X GET "elasticsearch:9200/mexoms-logs/_search" -H 'Content-Type: application/json' -d'
{
  "query": {
    "bool": {
      "must": [
        {"range": {"@timestamp": {"gte": "now-1h"}}},
        {"term": {"level": "error"}}
      ]
    }
  }
}
'
```

## Support Resources

- **Documentation**: [docs.mexoms.com](https://docs.mexoms.com)
- **Status Page**: [status.mexoms.com](https://status.mexoms.com)  
- **Support**: support@mexoms.com
- **Emergency**: +1-555-MEXOMS

---

*Keep your systems running smoothly! 🚀*