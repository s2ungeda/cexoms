package performance

import (
	"context"
	"fmt"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

// NetworkOptimizer manages network performance optimizations
type NetworkOptimizer struct {
	config   NetworkConfig
	metrics  *NetworkMetrics
	connPool *ConnPool
	running  bool
	stopCh   chan struct{}
	mu       sync.RWMutex
}

type NetworkConfig struct {
	// TCP settings
	TCPNoDelay         bool
	TCPKeepAlive       bool
	TCPKeepAliveIdle   time.Duration
	TCPKeepAliveCount  int
	TCPKeepAliveIntvl  time.Duration
	
	// Socket settings
	SendBufferSize     int
	RecvBufferSize     int
	ReuseAddr          bool
	ReusePort          bool
	
	// Connection settings
	ConnectTimeout     time.Duration
	ReadTimeout        time.Duration
	WriteTimeout       time.Duration
	MaxConnections     int
	IdleTimeout        time.Duration
	
	// Performance optimizations
	EnableZeroCopy     bool
	EnableSendFile     bool
	EnableSplice       bool
	EnableBatching     bool
	BatchSize          int
	BatchTimeout       time.Duration
	
	// Advanced features
	EnableKernelBypass bool
	EnableTCPFastOpen  bool
	EnableMultipath    bool
}

func DefaultNetworkConfig() NetworkConfig {
	return NetworkConfig{
		TCPNoDelay:         true,
		TCPKeepAlive:       true,
		TCPKeepAliveIdle:   30 * time.Second,
		TCPKeepAliveCount:  3,
		TCPKeepAliveIntvl:  30 * time.Second,
		SendBufferSize:     64 * 1024, // 64KB
		RecvBufferSize:     64 * 1024, // 64KB
		ReuseAddr:          true,
		ReusePort:          true,
		ConnectTimeout:     5 * time.Second,
		ReadTimeout:        30 * time.Second,
		WriteTimeout:       30 * time.Second,
		MaxConnections:     1000,
		IdleTimeout:        60 * time.Second,
		EnableZeroCopy:     true,
		EnableSendFile:     true,
		EnableSplice:       true,
		EnableBatching:     true,
		BatchSize:          32,
		BatchTimeout:       1 * time.Millisecond,
		EnableKernelBypass: false, // Requires special setup
		EnableTCPFastOpen:  true,
		EnableMultipath:    false,
	}
}

func NewNetworkOptimizer(config NetworkConfig) *NetworkOptimizer {
	return &NetworkOptimizer{
		config:   config,
		metrics:  NewNetworkMetrics(),
		connPool: NewConnPool(config.MaxConnections),
		stopCh:   make(chan struct{}),
	}
}

func (n *NetworkOptimizer) Start() {
	n.mu.Lock()
	if n.running {
		n.mu.Unlock()
		return
	}
	n.running = true
	n.mu.Unlock()

	// Start metrics collection
	go n.collectMetrics()
}

func (n *NetworkOptimizer) Stop() {
	n.mu.Lock()
	if !n.running {
		n.mu.Unlock()
		return
	}
	n.running = false
	n.mu.Unlock()

	close(n.stopCh)
	n.connPool.Close()
}

func (n *NetworkOptimizer) collectMetrics() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			n.updateSystemMetrics()
		case <-n.stopCh:
			return
		}
	}
}

// OptimizedDial creates an optimized network connection
func (n *NetworkOptimizer) OptimizedDial(network, address string) (net.Conn, error) {
	start := time.Now()
	defer func() {
		n.metrics.RecordConnectionLatency(time.Since(start))
	}()

	// Create socket with optimizations
	conn, err := n.createOptimizedSocket(network, address)
	if err != nil {
		n.metrics.IncrementConnectionErrors()
		return nil, err
	}

	n.metrics.IncrementConnectionsCreated()
	return conn, nil
}

func (n *NetworkOptimizer) createOptimizedSocket(network, address string) (net.Conn, error) {
	// Parse address
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return nil, err
	}

	// Resolve address
	addr, err := net.ResolveTCPAddr(network, net.JoinHostPort(host, port))
	if err != nil {
		return nil, err
	}

	// Create socket
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	// Apply socket optimizations
	if err := n.applySocketOptions(fd); err != nil {
		syscall.Close(fd)
		return nil, err
	}

	// Connect with timeout
	if err := n.connectWithTimeout(fd, addr); err != nil {
		syscall.Close(fd)
		return nil, err
	}

	// Create net.Conn from file descriptor
	file := &syscallConn{fd: fd}
	return file, nil
}

func (n *NetworkOptimizer) applySocketOptions(fd int) error {
	// TCP_NODELAY - disable Nagle's algorithm
	if n.config.TCPNoDelay {
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_NODELAY, 1); err != nil {
			return err
		}
	}

	// SO_REUSEADDR - allow address reuse
	if n.config.ReuseAddr {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1); err != nil {
			return err
		}
	}

	// SO_REUSEPORT - allow port reuse (Linux-specific)
	if n.config.ReusePort {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, 0x0F /* SO_REUSEPORT */, 1); err != nil {
			// Ignore error if not supported
		}
	}

	// SO_SNDBUF - send buffer size
	if n.config.SendBufferSize > 0 {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_SNDBUF, n.config.SendBufferSize); err != nil {
			return err
		}
	}

	// SO_RCVBUF - receive buffer size
	if n.config.RecvBufferSize > 0 {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_RCVBUF, n.config.RecvBufferSize); err != nil {
			return err
		}
	}

	// SO_KEEPALIVE - enable keep-alive
	if n.config.TCPKeepAlive {
		if err := syscall.SetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_KEEPALIVE, 1); err != nil {
			return err
		}

		// TCP_KEEPIDLE - time before sending keep-alive probes
		idle := int(n.config.TCPKeepAliveIdle.Seconds())
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPIDLE, idle); err != nil {
			return err
		}

		// TCP_KEEPINTVL - interval between keep-alive probes
		intvl := int(n.config.TCPKeepAliveIntvl.Seconds())
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPINTVL, intvl); err != nil {
			return err
		}

		// TCP_KEEPCNT - number of keep-alive probes
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, syscall.TCP_KEEPCNT, n.config.TCPKeepAliveCount); err != nil {
			return err
		}
	}

	// TCP_FASTOPEN - enable TCP Fast Open (Linux-specific)
	if n.config.EnableTCPFastOpen {
		if err := syscall.SetsockoptInt(fd, syscall.IPPROTO_TCP, 23 /* TCP_FASTOPEN */, 1); err != nil {
			// Ignore error if not supported
		}
	}

	return nil
}

func (n *NetworkOptimizer) connectWithTimeout(fd int, addr *net.TCPAddr) error {
	// Convert to syscall address
	sockaddr := &syscall.SockaddrInet4{Port: addr.Port}
	copy(sockaddr.Addr[:], addr.IP.To4())

	// Set non-blocking mode
	if err := syscall.SetNonblock(fd, true); err != nil {
		return err
	}

	// Attempt connection
	err := syscall.Connect(fd, sockaddr)
	if err == syscall.EINPROGRESS {
		// Use select to wait for connection with timeout
		if err := n.waitForConnection(fd); err != nil {
			return err
		}
	} else if err != nil {
		return err
	}

	// Set blocking mode back
	if err := syscall.SetNonblock(fd, false); err != nil {
		return err
	}

	return nil
}

func (n *NetworkOptimizer) waitForConnection(fd int) error {
	// This is a simplified version - production code should use epoll/kqueue
	deadline := time.Now().Add(n.config.ConnectTimeout)
	for time.Now().Before(deadline) {
		// Check if socket is writable (connected)
		var errno syscall.Errno
		_, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_ERROR)
		if err != nil {
			errno = err.(syscall.Errno)
		}
		
		if errno == 0 {
			return nil // Connected successfully
		}
		
		if errno != syscall.EINPROGRESS {
			return errno
		}
		
		time.Sleep(time.Millisecond)
	}
	
	return syscall.ETIMEDOUT
}

// ZeroCopyWrite implements zero-copy write using sendfile/splice
func (n *NetworkOptimizer) ZeroCopyWrite(dst net.Conn, src *os.File, count int64) (int64, error) {
	if !n.config.EnableZeroCopy {
		return 0, fmt.Errorf("zero-copy disabled")
	}

	start := time.Now()
	defer func() {
		n.metrics.RecordWriteLatency(time.Since(start))
	}()

	// Get file descriptors
	dstFd := n.getConnFd(dst)
	srcFd := int(src.Fd())

	if dstFd < 0 {
		return 0, fmt.Errorf("cannot get destination file descriptor")
	}

	// Use sendfile for zero-copy transfer
	written, err := syscall.Sendfile(dstFd, srcFd, nil, int(count))
	if err != nil {
		n.metrics.IncrementWriteErrors()
		return 0, err
	}

	n.metrics.RecordBytesWritten(int64(written))
	return int64(written), nil
}

// BatchWrite performs batched write operations
func (n *NetworkOptimizer) BatchWrite(conn net.Conn, buffers [][]byte) (int64, error) {
	if !n.config.EnableBatching {
		// Fall back to individual writes
		var total int64
		for _, buf := range buffers {
			written, err := conn.Write(buf)
			total += int64(written)
			if err != nil {
				return total, err
			}
		}
		return total, nil
	}

	start := time.Now()
	defer func() {
		n.metrics.RecordWriteLatency(time.Since(start))
	}()

	// Use writev for vectored I/O (not directly available in Go)
	// This is a simplified implementation
	var total int64
	for _, buf := range buffers {
		written, err := conn.Write(buf)
		total += int64(written)
		if err != nil {
			n.metrics.IncrementWriteErrors()
			return total, err
		}
	}

	n.metrics.RecordBytesWritten(total)
	return total, nil
}

func (n *NetworkOptimizer) getConnFd(conn net.Conn) int {
	// This is a hack to get file descriptor from net.Conn
	// In production, use a proper interface or type assertion
	if tc, ok := conn.(*net.TCPConn); ok {
		if file, err := tc.File(); err == nil {
			defer file.Close()
			return int(file.Fd())
		}
	}
	return -1
}

// Simple syscall-based connection implementation
type syscallConn struct {
	fd int
	mu sync.Mutex
}

func (c *syscallConn) Read(b []byte) (n int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return syscall.Read(c.fd, b)
}

func (c *syscallConn) Write(b []byte) (n int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return syscall.Write(c.fd, b)
}

func (c *syscallConn) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return syscall.Close(c.fd)
}

func (c *syscallConn) LocalAddr() net.Addr {
	// Simplified implementation
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (c *syscallConn) RemoteAddr() net.Addr {
	// Simplified implementation
	return &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0}
}

func (c *syscallConn) SetDeadline(t time.Time) error {
	// Not implemented in this simplified version
	return nil
}

func (c *syscallConn) SetReadDeadline(t time.Time) error {
	// Not implemented in this simplified version
	return nil
}

func (c *syscallConn) SetWriteDeadline(t time.Time) error {
	// Not implemented in this simplified version
	return nil
}

// Connection Pool for reusing connections
type ConnPool struct {
	connections chan net.Conn
	maxSize     int
	metrics     *ConnPoolMetrics
	mu          sync.RWMutex
}

type ConnPoolMetrics struct {
	ActiveConns    int64
	IdleConns      int64
	CreatedConns   int64
	ClosedConns    int64
	ReuseCount     int64
}

func NewConnPool(maxSize int) *ConnPool {
	return &ConnPool{
		connections: make(chan net.Conn, maxSize),
		maxSize:     maxSize,
		metrics:     &ConnPoolMetrics{},
	}
}

func (p *ConnPool) Get() net.Conn {
	select {
	case conn := <-p.connections:
		atomic.AddInt64(&p.metrics.IdleConns, -1)
		atomic.AddInt64(&p.metrics.ActiveConns, 1)
		atomic.AddInt64(&p.metrics.ReuseCount, 1)
		return conn
	default:
		return nil
	}
}

func (p *ConnPool) Put(conn net.Conn) bool {
	select {
	case p.connections <- conn:
		atomic.AddInt64(&p.metrics.ActiveConns, -1)
		atomic.AddInt64(&p.metrics.IdleConns, 1)
		return true
	default:
		// Pool is full, close connection
		conn.Close()
		atomic.AddInt64(&p.metrics.ActiveConns, -1)
		atomic.AddInt64(&p.metrics.ClosedConns, 1)
		return false
	}
}

func (p *ConnPool) Close() {
	close(p.connections)
	for conn := range p.connections {
		conn.Close()
		atomic.AddInt64(&p.metrics.ClosedConns, 1)
	}
}

func (p *ConnPool) GetMetrics() ConnPoolMetrics {
	return ConnPoolMetrics{
		ActiveConns:  atomic.LoadInt64(&p.metrics.ActiveConns),
		IdleConns:    atomic.LoadInt64(&p.metrics.IdleConns),
		CreatedConns: atomic.LoadInt64(&p.metrics.CreatedConns),
		ClosedConns:  atomic.LoadInt64(&p.metrics.ClosedConns),
		ReuseCount:   atomic.LoadInt64(&p.metrics.ReuseCount),
	}
}

// Network Metrics
type NetworkMetrics struct {
	ConnectionsCreated    int64
	ConnectionsDestroyed  int64
	ConnectionErrors      int64
	ConnectionLatencySum  int64
	ConnectionLatencyMax  int64
	ConnectionCount       int64
	
	BytesRead           int64
	BytesWritten        int64
	ReadLatencySum      int64
	WriteLatencySum     int64
	ReadErrors          int64
	WriteErrors         int64
	ReadCount           int64
	WriteCount          int64
	
	// System-level metrics
	TCPRetransmits      int64
	TCPTimeouts         int64
	SocketErrors        int64
}

func NewNetworkMetrics() *NetworkMetrics {
	return &NetworkMetrics{}
}

func (m *NetworkMetrics) IncrementConnectionsCreated() {
	atomic.AddInt64(&m.ConnectionsCreated, 1)
}

func (m *NetworkMetrics) IncrementConnectionErrors() {
	atomic.AddInt64(&m.ConnectionErrors, 1)
}

func (m *NetworkMetrics) RecordConnectionLatency(latency time.Duration) {
	atomic.AddInt64(&m.ConnectionCount, 1)
	atomic.AddInt64(&m.ConnectionLatencySum, int64(latency))
	
	// Update max latency
	for {
		current := atomic.LoadInt64(&m.ConnectionLatencyMax)
		if int64(latency) <= current {
			break
		}
		if atomic.CompareAndSwapInt64(&m.ConnectionLatencyMax, current, int64(latency)) {
			break
		}
	}
}

func (m *NetworkMetrics) RecordBytesRead(bytes int64) {
	atomic.AddInt64(&m.BytesRead, bytes)
	atomic.AddInt64(&m.ReadCount, 1)
}

func (m *NetworkMetrics) RecordBytesWritten(bytes int64) {
	atomic.AddInt64(&m.BytesWritten, bytes)
	atomic.AddInt64(&m.WriteCount, 1)
}

func (m *NetworkMetrics) RecordReadLatency(latency time.Duration) {
	atomic.AddInt64(&m.ReadLatencySum, int64(latency))
}

func (m *NetworkMetrics) RecordWriteLatency(latency time.Duration) {
	atomic.AddInt64(&m.WriteLatencySum, int64(latency))
}

func (m *NetworkMetrics) IncrementReadErrors() {
	atomic.AddInt64(&m.ReadErrors, 1)
}

func (m *NetworkMetrics) IncrementWriteErrors() {
	atomic.AddInt64(&m.WriteErrors, 1)
}

func (m *NetworkMetrics) GetAverageConnectionLatency() time.Duration {
	count := atomic.LoadInt64(&m.ConnectionCount)
	if count == 0 {
		return 0
	}
	sum := atomic.LoadInt64(&m.ConnectionLatencySum)
	return time.Duration(sum / count)
}

func (m *NetworkMetrics) GetAverageReadLatency() time.Duration {
	count := atomic.LoadInt64(&m.ReadCount)
	if count == 0 {
		return 0
	}
	sum := atomic.LoadInt64(&m.ReadLatencySum)
	return time.Duration(sum / count)
}

func (m *NetworkMetrics) GetAverageWriteLatency() time.Duration {
	count := atomic.LoadInt64(&m.WriteCount)
	if count == 0 {
		return 0
	}
	sum := atomic.LoadInt64(&m.WriteLatencySum)
	return time.Duration(sum / count)
}

func (m *NetworkMetrics) GetThroughput() (readMBps, writeMBps float64) {
	// This would need time-based calculations in a real implementation
	bytesRead := atomic.LoadInt64(&m.BytesRead)
	bytesWritten := atomic.LoadInt64(&m.BytesWritten)
	
	// Simplified calculation (would need proper time tracking)
	readMBps = float64(bytesRead) / (1024 * 1024) // Convert to MB
	writeMBps = float64(bytesWritten) / (1024 * 1024)
	
	return readMBps, writeMBps
}

func (n *NetworkOptimizer) updateSystemMetrics() {
	// This would read from /proc/net/tcp, /proc/net/netstat, etc.
	// Simplified implementation - would need proper Linux networking stats parsing
}

func (n *NetworkOptimizer) GetMetrics() *NetworkMetrics {
	return n.metrics
}

func (n *NetworkOptimizer) GetConnPool() *ConnPool {
	return n.connPool
}