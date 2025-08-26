#!/bin/bash

# Security hardening script for mExOms containers

set -e

# Configuration
SCAN_IMAGES=${1:-true}
APPLY_POLICIES=${2:-true}

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

log() { echo -e "${GREEN}[SECURITY]${NC} $1"; }
error() { echo -e "${RED}[ERROR]${NC} $1"; exit 1; }
warning() { echo -e "${YELLOW}[WARNING]${NC} $1"; }

# Check prerequisites
check_tools() {
    log "Checking security tools..."
    
    # Install trivy for vulnerability scanning
    if ! command -v trivy &> /dev/null; then
        warning "Trivy not found, installing..."
        wget -qO - https://aquasecurity.github.io/trivy-repo/deb/public.key | sudo apt-key add -
        echo "deb https://aquasecurity.github.io/trivy-repo/deb $(lsb_release -sc) main" | sudo tee -a /etc/apt/sources.list.d/trivy.list
        sudo apt-get update && sudo apt-get install trivy
    fi
    
    # Check docker-bench-security
    if [ ! -f "/tmp/docker-bench-security.sh" ]; then
        warning "Downloading docker-bench-security..."
        wget -q https://raw.githubusercontent.com/docker/docker-bench-security/master/docker-bench-security.sh -O /tmp/docker-bench-security.sh
        chmod +x /tmp/docker-bench-security.sh
    fi
}

# Scan images for vulnerabilities
scan_images() {
    log "Scanning Docker images for vulnerabilities..."
    
    images=$(docker images --format "{{.Repository}}:{{.Tag}}" | grep mexoms)
    
    for image in $images; do
        log "Scanning $image..."
        trivy image --severity HIGH,CRITICAL "$image" || warning "Vulnerabilities found in $image"
    done
}

# Apply security policies
apply_security_policies() {
    log "Applying security policies..."
    
    # Create security policy file
    cat > /tmp/mexoms-security-policy.json <<EOF
{
  "policies": [
    {
      "name": "no-root-user",
      "description": "Containers should not run as root",
      "rule": "user != 'root'"
    },
    {
      "name": "read-only-root-filesystem",
      "description": "Root filesystem should be read-only",
      "rule": "readOnlyRootFilesystem == true"
    },
    {
      "name": "no-privileged",
      "description": "Containers should not run in privileged mode",
      "rule": "privileged == false"
    },
    {
      "name": "drop-capabilities",
      "description": "Drop all capabilities by default",
      "rule": "capabilities.drop contains 'ALL'"
    }
  ]
}
EOF
}

# Harden running containers
harden_containers() {
    log "Hardening running containers..."
    
    containers=$(docker ps --format "{{.Names}}" | grep mexoms)
    
    for container in $containers; do
        log "Checking $container..."
        
        # Check if running as root
        user=$(docker exec "$container" id -u 2>/dev/null || echo "0")
        if [ "$user" = "0" ]; then
            warning "$container is running as root"
        fi
        
        # Check capabilities
        caps=$(docker inspect "$container" | jq -r '.[0].HostConfig.CapDrop[]' 2>/dev/null)
        if [[ ! "$caps" =~ "ALL" ]]; then
            warning "$container has not dropped all capabilities"
        fi
        
        # Check read-only filesystem
        readonly=$(docker inspect "$container" | jq -r '.[0].HostConfig.ReadonlyRootfs' 2>/dev/null)
        if [ "$readonly" != "true" ]; then
            warning "$container does not have read-only root filesystem"
        fi
    done
}

# Set up AppArmor profiles
setup_apparmor() {
    log "Setting up AppArmor profiles..."
    
    # Create AppArmor profile for mExOms
    sudo tee /etc/apparmor.d/docker-mexoms > /dev/null <<EOF
#include <tunables/global>

profile docker-mexoms flags=(attach_disconnected,mediate_deleted) {
  #include <abstractions/base>
  
  # Deny all by default
  deny /** rwklx,
  
  # Allow necessary operations
  /app/** r,
  /app/bin/* ix,
  /app/logs/** rw,
  /app/data/** rw,
  /tmp/** rw,
  /dev/null rw,
  /dev/urandom r,
  /proc/sys/net/** r,
  
  # Network access
  network tcp,
  network udp,
  
  # Capabilities
  capability net_bind_service,
  capability setuid,
  capability setgid,
  
  # Deny dangerous operations
  deny /proc/sys/** w,
  deny /sys/** w,
  deny /boot/** rwx,
  deny /lib/** wx,
}
EOF
    
    # Load profile
    sudo apparmor_parser -r /etc/apparmor.d/docker-mexoms
}

# Create seccomp profile
create_seccomp_profile() {
    log "Creating seccomp security profile..."
    
    cat > configs/seccomp-profile.json <<EOF
{
  "defaultAction": "SCMP_ACT_ERRNO",
  "architectures": [
    "SCMP_ARCH_X86_64",
    "SCMP_ARCH_X86",
    "SCMP_ARCH_X32"
  ],
  "syscalls": [
    {
      "names": [
        "accept", "accept4", "access", "alarm", "arch_prctl",
        "bind", "brk", "capget", "capset", "chdir", "chmod",
        "chown", "chown32", "clock_getres", "clock_gettime",
        "clock_nanosleep", "close", "connect", "copy_file_range",
        "creat", "dup", "dup2", "dup3", "epoll_create",
        "epoll_create1", "epoll_ctl", "epoll_ctl_old",
        "epoll_pwait", "epoll_wait", "epoll_wait_old",
        "eventfd", "eventfd2", "execve", "execveat",
        "exit", "exit_group", "faccessat", "fadvise64",
        "fadvise64_64", "fallocate", "fanotify_mark",
        "fchdir", "fchmod", "fchmodat", "fchown",
        "fchown32", "fchownat", "fcntl", "fcntl64",
        "fdatasync", "fgetxattr", "flistxattr", "flock",
        "fork", "fremovexattr", "fsetxattr", "fstat",
        "fstat64", "fstatat64", "fstatfs", "fstatfs64",
        "fsync", "ftruncate", "ftruncate64", "futex",
        "futimesat", "getcpu", "getcwd", "getdents",
        "getdents64", "getegid", "getegid32", "geteuid",
        "geteuid32", "getgid", "getgid32", "getgroups",
        "getgroups32", "getitimer", "getpeername", "getpgid",
        "getpgrp", "getpid", "getppid", "getpriority",
        "getrandom", "getresgid", "getresgid32", "getresuid",
        "getresuid32", "getrlimit", "get_robust_list",
        "getrusage", "getsid", "getsockname", "getsockopt",
        "get_thread_area", "gettid", "gettimeofday", "getuid",
        "getuid32", "getxattr", "inotify_add_watch",
        "inotify_init", "inotify_init1", "inotify_rm_watch",
        "io_cancel", "ioctl", "io_destroy", "io_getevents",
        "ioprio_get", "ioprio_set", "io_setup", "io_submit",
        "ipc", "kill", "lchown", "lchown32", "lgetxattr",
        "link", "linkat", "listen", "listxattr", "llistxattr",
        "lremovexattr", "lseek", "lsetxattr", "lstat",
        "lstat64", "madvise", "memfd_create", "mincore",
        "mkdir", "mkdirat", "mknod", "mknodat", "mlock",
        "mlock2", "mlockall", "mmap", "mmap2", "mprotect",
        "mq_getsetattr", "mq_notify", "mq_open", "mq_timedreceive",
        "mq_timedsend", "mq_unlink", "mremap", "msgctl",
        "msgget", "msgrcv", "msgsnd", "msync", "munlock",
        "munlockall", "munmap", "nanosleep", "newfstatat",
        "open", "openat", "pause", "pipe", "pipe2", "poll",
        "ppoll", "prctl", "pread64", "preadv", "prlimit64",
        "pselect6", "pwrite64", "pwritev", "read", "readahead",
        "readlink", "readlinkat", "readv", "recv", "recvfrom",
        "recvmmsg", "recvmsg", "remap_file_pages", "removexattr",
        "rename", "renameat", "renameat2", "restart_syscall",
        "rmdir", "rt_sigaction", "rt_sigpending", "rt_sigprocmask",
        "rt_sigqueueinfo", "rt_sigreturn", "rt_sigsuspend",
        "rt_sigtimedwait", "rt_tgsigqueueinfo", "sched_getaffinity",
        "sched_getattr", "sched_getparam", "sched_get_priority_max",
        "sched_get_priority_min", "sched_getscheduler",
        "sched_rr_get_interval", "sched_setaffinity",
        "sched_setattr", "sched_setparam", "sched_setscheduler",
        "sched_yield", "seccomp", "select", "semctl", "semget",
        "semop", "semtimedop", "send", "sendfile", "sendfile64",
        "sendmmsg", "sendmsg", "sendto", "setfsgid", "setfsgid32",
        "setfsuid", "setfsuid32", "setgid", "setgid32",
        "setgroups", "setgroups32", "setitimer", "setpgid",
        "setpriority", "setregid", "setregid32", "setresgid",
        "setresgid32", "setresuid", "setresuid32", "setreuid",
        "setreuid32", "setrlimit", "set_robust_list", "setsid",
        "setsockopt", "set_thread_area", "set_tid_address",
        "setuid", "setuid32", "setxattr", "shmat", "shmctl",
        "shmdt", "shmget", "shutdown", "sigaltstack", "signalfd",
        "signalfd4", "sigreturn", "socket", "socketcall",
        "socketpair", "splice", "stat", "stat64", "statfs",
        "statfs64", "statx", "symlink", "symlinkat", "sync",
        "sync_file_range", "syncfs", "sysinfo", "syslog",
        "tee", "tgkill", "time", "timer_create", "timer_delete",
        "timerfd_create", "timerfd_gettime", "timerfd_settime",
        "timer_getoverrun", "timer_gettime", "timer_settime",
        "times", "tkill", "truncate", "truncate64", "ugetrlimit",
        "umask", "uname", "unlink", "unlinkat", "utime",
        "utimensat", "utimes", "vfork", "vmsplice", "wait4",
        "waitid", "waitpid", "write", "writev"
      ],
      "action": "SCMP_ACT_ALLOW"
    }
  ]
}
EOF
}

# Network security
setup_network_security() {
    log "Setting up network security..."
    
    # Create custom Docker network with encryption
    docker network create \
        --driver bridge \
        --opt encrypted=true \
        --opt com.docker.network.bridge.name=br-mexoms \
        --subnet=172.20.0.0/16 \
        mexoms-secure || warning "Network already exists"
    
    # Set up iptables rules
    sudo iptables -A DOCKER-USER -i eth0 -j DROP
    sudo iptables -A DOCKER-USER -i eth0 -p tcp --dport 8080 -j ACCEPT
    sudo iptables -A DOCKER-USER -i eth0 -p tcp --dport 9090 -j ACCEPT
    sudo iptables -A DOCKER-USER -m conntrack --ctstate ESTABLISHED,RELATED -j ACCEPT
}

# Run Docker bench security
run_docker_bench() {
    log "Running Docker security benchmark..."
    
    sudo /tmp/docker-bench-security.sh | tee security-report.txt
}

# Generate security report
generate_report() {
    log "Generating security report..."
    
    cat > security-summary.md <<EOF
# mExOms Security Report
Generated: $(date)

## Image Vulnerability Scan
$(trivy image --format table mexoms/oms-server:latest 2>/dev/null || echo "No scan results")

## Container Security Status
| Container | User | Read-only FS | Capabilities Dropped |
|-----------|------|--------------|---------------------|
EOF
    
    containers=$(docker ps --format "{{.Names}}" | grep mexoms)
    for container in $containers; do
        user=$(docker exec "$container" whoami 2>/dev/null || echo "unknown")
        readonly=$(docker inspect "$container" | jq -r '.[0].HostConfig.ReadonlyRootfs' 2>/dev/null)
        caps=$(docker inspect "$container" | jq -r '.[0].HostConfig.CapDrop[]' 2>/dev/null | grep -c "ALL")
        
        echo "| $container | $user | $readonly | $([[ $caps -gt 0 ]] && echo "Yes" || echo "No") |" >> security-summary.md
    done
    
    echo -e "\n## Recommendations" >> security-summary.md
    echo "1. Run containers as non-root user" >> security-summary.md
    echo "2. Enable read-only root filesystem where possible" >> security-summary.md
    echo "3. Drop all capabilities and add only required ones" >> security-summary.md
    echo "4. Use seccomp and AppArmor profiles" >> security-summary.md
    echo "5. Regularly scan images for vulnerabilities" >> security-summary.md
    
    log "Security report saved to security-summary.md"
}

# Main execution
main() {
    log "Starting security hardening..."
    
    check_tools
    
    if [ "$SCAN_IMAGES" = "true" ]; then
        scan_images
    fi
    
    if [ "$APPLY_POLICIES" = "true" ]; then
        apply_security_policies
        setup_apparmor
        create_seccomp_profile
        setup_network_security
    fi
    
    harden_containers
    run_docker_bench
    generate_report
    
    log "Security hardening completed"
}

# Run main
main