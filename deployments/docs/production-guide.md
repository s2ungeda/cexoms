# Production Deployment Guide

This guide covers deploying the OMS to a production environment.

## Prerequisites

- Ubuntu 20.04+ or CentOS 8+
- 8+ CPU cores (physical cores recommended)
- 16GB+ RAM
- SSD storage with 100GB+ free space
- Root or sudo access
- Network access to cryptocurrency exchanges

## Installation Options

### 1. Bare Metal Installation (Recommended)

For lowest latency, deploy directly on bare metal servers.

```bash
# Clone repository
git clone https://github.com/mExOms/oms.git
cd oms

# Run installation script
sudo ./deployments/scripts/install.sh
```

The installation script will:
- Create OMS user and directories
- Install dependencies (NATS, Vault)
- Copy binaries and configurations
- Set up systemd services
- Configure log rotation
- Set up monitoring

### 2. Docker Deployment

For easier management, use Docker Compose:

```bash
# Build images
docker-compose -f deployments/docker/docker-compose.yml build

# Start services
docker-compose -f deployments/docker/docker-compose.yml up -d

# Check status
docker-compose -f deployments/docker/docker-compose.yml ps
```

## Configuration

### 1. API Keys

Add exchange API keys to Vault:

```bash
# Initialize Vault (first time only)
vault operator init
vault operator unseal

# Login to Vault
vault login

# Add Binance API keys
vault kv put secret/exchanges/binance_spot \
  api_key="your-api-key" \
  api_secret="your-api-secret"

vault kv put secret/exchanges/binance_futures \
  api_key="your-futures-api-key" \
  api_secret="your-futures-api-secret"
```

### 2. Update Configuration Files

Edit configuration files in `/opt/oms/configs/`:

```yaml
# /opt/oms/configs/config.yaml
exchanges:
  binance:
    testnet: false  # Change to false for production
    
risk:
  max_position_value: 100000  # Adjust based on your limits
  max_order_value: 10000
  daily_loss_limit: 5000

monitoring:
  alerts:
    email: ops@yourcompany.com
    webhook: https://your-webhook-url
```

### 3. TLS Certificates

Generate TLS certificates for gRPC:

```bash
# Generate self-signed certificate (for testing)
openssl req -x509 -newkey rsa:4096 -keyout server.key -out server.crt \
  -days 365 -nodes -subj "/CN=localhost"

# Copy to OMS directory
sudo cp server.{crt,key} /opt/oms/certs/
sudo chown oms:oms /opt/oms/certs/*
sudo chmod 600 /opt/oms/certs/server.key
```

For production, use certificates from a trusted CA.

## Starting Services

### Systemd Services

```bash
# Start core services in order
sudo systemctl start nats
sudo systemctl start oms-engine
sudo systemctl start oms-binance-spot
sudo systemctl start oms-binance-futures
sudo systemctl start oms-grpc-gateway

# Enable auto-start on boot
sudo systemctl enable nats oms-engine oms-binance-spot oms-binance-futures oms-grpc-gateway

# Check status
sudo systemctl status oms-*
```

### Docker Services

```bash
# Start all services
docker-compose -f deployments/docker/docker-compose.yml up -d

# View logs
docker-compose -f deployments/docker/docker-compose.yml logs -f

# Stop services
docker-compose -f deployments/docker/docker-compose.yml down
```

## Performance Tuning

### 1. CPU Affinity

Set CPU affinity for optimal performance:

```bash
# Edit service files to set CPU affinity
# /etc/systemd/system/oms-engine.service
[Service]
CPUAffinity=2 3  # Dedicate cores 2-3 to engine
```

See [CPU Allocation Guide](cpu-allocation.md) for detailed recommendations.

### 2. Kernel Parameters

Optimize kernel parameters:

```bash
# /etc/sysctl.d/oms.conf
# Network optimizations
net.core.rmem_max = 134217728
net.core.wmem_max = 134217728
net.ipv4.tcp_rmem = 4096 87380 134217728
net.ipv4.tcp_wmem = 4096 65536 134217728

# Reduce swappiness
vm.swappiness = 10

# Apply settings
sudo sysctl -p /etc/sysctl.d/oms.conf
```

### 3. Disable CPU Frequency Scaling

```bash
# Set performance governor
for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
    echo "performance" | sudo tee $cpu
done
```

## Monitoring

### 1. Health Checks

```bash
# Run health check
/opt/oms/bin/check-health.sh

# Monitor continuously
/opt/oms/bin/monitor.sh --continuous
```

### 2. Log Monitoring

```bash
# View engine logs
journalctl -u oms-engine -f

# View all OMS logs
journalctl -u oms-* -f

# Check for errors
journalctl -u oms-* --since "1 hour ago" | grep ERROR
```

### 3. Performance Metrics

Monitor key metrics:
- Order processing latency (target: < 100μs)
- Risk check latency (target: < 50μs)
- Message queue depth
- CPU and memory usage
- Network latency to exchanges

## Security

### 1. Firewall Rules

```bash
# Allow only necessary ports
sudo ufw allow 22/tcp    # SSH
sudo ufw allow 50051/tcp # gRPC (restrict source IPs)
sudo ufw enable
```

### 2. User Permissions

```bash
# Ensure OMS user has minimal permissions
sudo usermod -s /bin/false oms
sudo chmod 750 /opt/oms
```

### 3. API Key Security

- Use Vault for all API keys
- Enable Vault audit logging
- Rotate API keys every 30 days
- Use IP whitelisting on exchanges

## Backup and Recovery

### 1. Automated Backups

```bash
# Create backup script
cat > /opt/oms/bin/backup.sh << 'EOF'
#!/bin/bash
BACKUP_DIR="/backup/oms/$(date +%Y%m%d)"
mkdir -p "$BACKUP_DIR"

# Backup configurations
cp -r /opt/oms/configs "$BACKUP_DIR/"

# Backup data
tar -czf "$BACKUP_DIR/data.tar.gz" /opt/oms/data/

# Backup Vault
vault operator raft snapshot save "$BACKUP_DIR/vault.snap"

# Rotate old backups (keep 30 days)
find /backup/oms -type d -mtime +30 -exec rm -rf {} \;
EOF

chmod +x /opt/oms/bin/backup.sh

# Add to crontab
crontab -e
0 2 * * * /opt/oms/bin/backup.sh
```

### 2. Disaster Recovery

Document and test recovery procedures:
1. Install OMS on new server
2. Restore configurations
3. Restore Vault data
4. Restart services
5. Verify connectivity

## Troubleshooting

### Common Issues

1. **High Latency**
   - Check CPU affinity settings
   - Verify no CPU throttling
   - Check network latency to exchanges
   - Review system load

2. **Connection Issues**
   - Verify firewall rules
   - Check exchange API status
   - Validate API credentials
   - Review rate limits

3. **Service Failures**
   - Check logs: `journalctl -u <service>`
   - Verify dependencies are running
   - Check disk space
   - Review memory usage

### Debug Mode

Enable debug logging:

```yaml
# In config files
logging:
  level: debug
```

### Emergency Procedures

1. **Stop all trading**:
   ```bash
   sudo systemctl stop oms-binance-spot oms-binance-futures
   ```

2. **Cancel all orders**:
   ```bash
   /opt/oms/bin/emergency-cancel-all
   ```

3. **Full system restart**:
   ```bash
   sudo systemctl restart oms-*
   ```

## Maintenance

### Weekly Tasks
- Review performance metrics
- Check disk usage
- Verify backups
- Review error logs

### Monthly Tasks
- Rotate API keys
- Update system packages
- Review and optimize configurations
- Performance testing

### Quarterly Tasks
- Security audit
- Disaster recovery drill
- Capacity planning
- Software updates

## Support

For production support:
- Create issues on GitHub
- Check logs first
- Include system specifications
- Provide performance metrics