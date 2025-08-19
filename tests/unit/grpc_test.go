package unit

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestGRPCServerConnection(t *testing.T) {
	// Create a test gRPC server
	lis, err := net.Listen("tcp", ":0") // Random port
	require.NoError(t, err)

	srv := grpc.NewServer()
	
	// Start server in background
	go func() {
		_ = srv.Serve(lis)
	}()
	defer srv.Stop()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Try to connect
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, lis.Addr().String(), 
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock())
	
	assert.NoError(t, err)
	assert.NotNil(t, conn)
	
	if conn != nil {
		conn.Close()
	}
}

func TestMultipleConnections(t *testing.T) {
	// Test that server can handle multiple concurrent connections
	lis, err := net.Listen("tcp", ":0")
	require.NoError(t, err)

	srv := grpc.NewServer()
	go func() {
		_ = srv.Serve(lis)
	}()
	defer srv.Stop()

	time.Sleep(100 * time.Millisecond)

	// Create multiple connections
	conns := make([]*grpc.ClientConn, 5)
	for i := 0; i < 5; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		conn, err := grpc.DialContext(ctx, lis.Addr().String(),
			grpc.WithTransportCredentials(insecure.NewCredentials()),
			grpc.WithBlock())
		cancel()
		
		require.NoError(t, err)
		require.NotNil(t, conn)
		conns[i] = conn
	}

	// Close all connections
	for _, conn := range conns {
		conn.Close()
	}
}