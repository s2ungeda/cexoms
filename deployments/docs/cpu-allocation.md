# CPU Core Allocation Guide

This document describes the recommended CPU core allocation for optimal OMS performance.

## Overview

The OMS uses CPU affinity to ensure consistent low-latency performance by binding processes to specific CPU cores. This prevents context switching and improves cache locality.

## Recommended CPU Allocation

### Minimum Requirements
- 8 CPU cores (physical cores recommended)
- Hyper-threading disabled for critical cores
- CPU frequency scaling disabled (performance governor)

### Core Assignment

```
Core 0-1: NATS Messaging
  - NATS server process
  - High priority, real-time scheduling
  - Handles all inter-service communication

Core 2-3: C++ Core Engine
  - Order processing engine
  - Risk management
  - Lock-free data structures
  - Highest priority (-10 nice)
  - Real-time I/O scheduling

Core 4: Exchange Connectors (Primary)
  - Binance Spot connector
  - Primary exchange connection
  - WebSocket and REST API handling

Core 5: Exchange Connectors (Secondary)
  - Binance Futures connector
  - Additional exchange connectors
  - Load balanced with Core 4

Core 6: API Gateway
  - gRPC server
  - Client connections
  - Rate limiting
  - Authentication

Core 7: System/Monitoring
  - OS processes
  - Monitoring agents
  - Log collection
  - Backup processes
```

## Configuration

### Setting CPU Affinity in systemd

Each service file includes CPU affinity settings:

```ini
[Service]
CPUAffinity=2 3  # Cores 2 and 3 for engine
```

### Manual CPU Affinity

Set CPU affinity for running processes:

```bash
# Set process to run on cores 2-3
taskset -cp 2,3 <pid>

# Launch process on specific cores
taskset -c 2,3 /opt/oms/bin/oms-engine
```

### Verify CPU Affinity

```bash
# Check CPU affinity for a process
taskset -cp <pid>

# Monitor CPU usage per core
htop  # Press F2 → Display options → Show custom thread names

# Check interrupt affinity
cat /proc/interrupts
```

## Performance Tuning

### Disable CPU Frequency Scaling

```bash
# Set performance governor
for cpu in /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor; do
    echo "performance" > $cpu
done

# Verify settings
cat /sys/devices/system/cpu/cpu*/cpufreq/scaling_governor
```

### Disable Hyper-Threading (Optional)

For cores 2-3 (C++ engine), consider disabling hyper-threading:

```bash
# Disable sibling threads
echo 0 > /sys/devices/system/cpu/cpu11/online  # Sibling of cpu3
echo 0 > /sys/devices/system/cpu/cpu10/online  # Sibling of cpu2
```

### IRQ Affinity

Move network interrupts away from critical cores:

```bash
# Find network interface interrupts
grep eth0 /proc/interrupts

# Set IRQ affinity to core 7
echo 80 > /proc/irq/24/smp_affinity  # 0x80 = core 7
```

### Memory and NUMA

For NUMA systems:

```bash
# Check NUMA topology
numactl --hardware

# Bind process to NUMA node 0
numactl --cpunodebind=0 --membind=0 /opt/oms/bin/oms-engine
```

## Monitoring

### Real-time Monitoring Script

```bash
#!/bin/bash
# /opt/oms/bin/monitor-cpu.sh

while true; do
    clear
    echo "=== OMS CPU Usage ==="
    echo
    
    # Show CPU usage for OMS processes
    ps -eLo pid,tid,psr,pcpu,comm | grep -E "oms-|nats" | \
        awk '{cpu[$3]+=$4; count[$3]++} 
             END {for (i in cpu) printf "Core %d: %.1f%% (%d threads)\n", i, cpu[i], count[i]}' | \
        sort -n
    
    echo
    echo "=== Core Frequencies ==="
    grep "cpu MHz" /proc/cpuinfo | nl -v 0
    
    sleep 2
done
```

### Performance Metrics

Monitor these metrics for optimal performance:

1. **CPU Usage**: Cores 2-3 should be 60-80% utilized
2. **Context Switches**: Should be minimal on cores 2-3
3. **Cache Misses**: Monitor L1/L2 cache performance
4. **Interrupt Count**: Should be low on critical cores

## Troubleshooting

### High Latency Issues

1. Check CPU affinity is properly set
2. Verify no other processes on critical cores
3. Check for thermal throttling
4. Verify C-states are disabled

### CPU Contention

```bash
# Find processes on specific core
ps -eLo pid,tid,psr,pcpu,comm | awk '$3==2' | sort -k4 -nr
```

### Adjust Priorities

```bash
# Increase process priority
renice -n -10 -p $(pidof oms-engine)

# Set real-time priority
chrt -f -p 50 $(pidof oms-engine)
```

## Best Practices

1. **Isolate Critical Cores**: Use kernel boot parameter `isolcpus=2,3`
2. **Disable Power Management**: Set BIOS to maximum performance
3. **Monitor Temperature**: Ensure adequate cooling
4. **Regular Testing**: Benchmark latency weekly
5. **Document Changes**: Log all CPU allocation modifications

## Alternative Configurations

### 4-Core System

```
Core 0: NATS + System
Core 1: C++ Engine
Core 2: Exchange Connectors
Core 3: API Gateway + Monitoring
```

### 16-Core System

```
Core 0-1: NATS (dedicated)
Core 2-5: C++ Engine (4 cores)
Core 6-9: Exchange Connectors (1 per exchange)
Core 10-11: API Gateway (load balanced)
Core 12-13: Risk Management (dedicated)
Core 14-15: System/Monitoring/Backup
```

## Validation

After configuration, validate the setup:

```bash
# Run CPU allocation test
/opt/oms/bin/test-cpu-allocation

# Expected output:
# ✓ NATS on cores 0-1
# ✓ Engine on cores 2-3
# ✓ No competing processes on critical cores
# ✓ CPU frequency at maximum
# ✓ IRQ affinity optimized
```