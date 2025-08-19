package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mExOms/internal/exchange"
	grpcSvc "github.com/mExOms/internal/grpc"
	"github.com/mExOms/internal/position"
	"github.com/mExOms/internal/risk"
	"github.com/mExOms/internal/router"
	omsv1 "github.com/mExOms/pkg/proto/oms/v1"
	"github.com/shopspring/decimal"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/keepalive"
	"google.golang.org/grpc/reflection"
)

var (
	port       = flag.Int("port", 9090, "gRPC server port")
	tlsCert    = flag.String("tls-cert", "", "TLS certificate file")
	tlsKey     = flag.String("tls-key", "", "TLS key file")
	enableTLS  = flag.Bool("enable-tls", false, "Enable TLS")
	rateLimit  = flag.Int("rate-limit", 100, "Rate limit per second per user")
	burstLimit = flag.Int("burst-limit", 200, "Burst limit per user")
)

func main() {
	flag.Parse()

	// Create core components
	exchangeFactory, err := createExchangeFactory()
	if err != nil {
		log.Fatal("Failed to create exchange factory:", err)
	}

	riskEngine := risk.NewRiskEngine()
	configureRiskEngine(riskEngine)

	smartRouter := router.NewSmartRouter(exchangeFactory.GetAvailableExchanges())

	positionManager, err := position.NewPositionManager("./data/snapshots")
	if err != nil {
		log.Fatal("Failed to create position manager:", err)
	}
	defer positionManager.Close()

	// Create gRPC services
	authService := grpcSvc.NewAuthService()
	orderService := grpcSvc.NewOrderService(exchangeFactory, riskEngine, smartRouter)
	positionService := grpcSvc.NewPositionService(positionManager)

	// Create interceptors
	authInterceptor := grpcSvc.NewAuthInterceptor(authService)
	rateLimiter := grpcSvc.NewRateLimiter(*rateLimit, *burstLimit)

	// Configure gRPC server options
	serverOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(grpc.ChainUnaryInterceptor(
			authInterceptor.Unary(),
			rateLimiter.Unary(),
		)),
		grpc.StreamInterceptor(grpc.ChainStreamInterceptor(
			authInterceptor.Stream(),
			rateLimiter.Stream(),
		)),
		grpc.KeepaliveParams(keepalive.ServerParameters{
			Time:    60 * time.Second,
			Timeout: 20 * time.Second,
		}),
		grpc.KeepaliveEnforcementPolicy(keepalive.EnforcementPolicy{
			MinTime:             5 * time.Second,
			PermitWithoutStream: true,
		}),
	}

	// Configure TLS if enabled
	if *enableTLS {
		creds, err := loadTLSCredentials()
		if err != nil {
			log.Fatal("Failed to load TLS credentials:", err)
		}
		serverOpts = append(serverOpts, grpc.Creds(creds))
	}

	// Create gRPC server
	grpcServer := grpc.NewServer(serverOpts...)

	// Register services
	omsv1.RegisterAuthServiceServer(grpcServer, authService)
	omsv1.RegisterOrderServiceServer(grpcServer, orderService)
	omsv1.RegisterPositionServiceServer(grpcServer, positionService)

	// Enable reflection for grpcurl
	reflection.Register(grpcServer)

	// Create demo API key
	createDemoAPIKey(authService)

	// Start server
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Fatal("Failed to listen:", err)
	}

	// Handle graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go handleShutdown(ctx, grpcServer)

	// Start serving
	protocol := "gRPC"
	if *enableTLS {
		protocol = "gRPC/TLS"
	}
	
	log.Printf("Starting %s server on port %d", protocol, *port)
	log.Println("=== gRPC API Gateway Started ===")
	log.Println("Services:")
	log.Println("  - AuthService")
	log.Println("  - OrderService")
	log.Println("  - PositionService")
	log.Println("  - MarketDataService (coming soon)")
	log.Println()
	log.Println("Security features:")
	log.Println("  - JWT authentication")
	log.Println("  - API key authentication")
	log.Printf("  - Rate limiting: %d req/s (burst: %d)", *rateLimit, *burstLimit)
	if *enableTLS {
		log.Println("  - TLS 1.3 enabled")
	}
	log.Println()
	log.Println("Demo API key created:")
	log.Println("  API Key: demo-api-key")
	log.Println("  Secret: demo-secret")
	log.Println()
	log.Println("Test with grpcurl:")
	log.Printf("  grpcurl -plaintext localhost:%d list", *port)
	log.Println()

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatal("Failed to serve:", err)
	}
}

func createExchangeFactory() (*exchange.Factory, error) {
	factory := exchange.NewFactory()

	// Register exchanges
	factory.RegisterExchange("binance", func(config map[string]interface{}) (interface{}, error) {
		// In production, load from config
		return nil, fmt.Errorf("binance client not implemented in demo")
	})

	return factory, nil
}

func configureRiskEngine(engine *risk.RiskEngine) {
	// Configure risk limits
	engine.SetMaxPositionSize(decimal.NewFromFloat(100000))  // $100k max position
	engine.SetMaxLeverage(20)                                 // 20x max leverage
	engine.SetMaxOrderValue(decimal.NewFromFloat(50000))     // $50k max order
	engine.SetMaxDailyLoss(decimal.NewFromFloat(10000))      // $10k max daily loss
	engine.SetMaxExposure(decimal.NewFromFloat(500000))      // $500k max exposure
}

func loadTLSCredentials() (credentials.TransportCredentials, error) {
	if *tlsCert == "" || *tlsKey == "" {
		// Generate self-signed certificate for demo
		return generateSelfSignedTLS()
	}

	// Load certificate and key
	cert, err := tls.LoadX509KeyPair(*tlsCert, *tlsKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
	}

	// Configure TLS
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13, // Enforce TLS 1.3
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}

	return credentials.NewTLS(config), nil
}

func generateSelfSignedTLS() (credentials.TransportCredentials, error) {
	// In production, use proper certificates
	// This is just for demo purposes
	config := &tls.Config{
		InsecureSkipVerify: true,
		MinVersion:         tls.VersionTLS13,
	}
	return credentials.NewTLS(config), nil
}

func createDemoAPIKey(authService *grpcSvc.AuthService) {
	// Create a demo API key for testing
	ctx := context.Background()
	req := &omsv1.CreateAPIKeyRequest{
		Name: "Demo API Key",
		Permissions: []omsv1.Permission{
			omsv1.Permission_PERMISSION_READ_ORDERS,
			omsv1.Permission_PERMISSION_WRITE_ORDERS,
			omsv1.Permission_PERMISSION_READ_POSITIONS,
			omsv1.Permission_PERMISSION_READ_MARKET_DATA,
		},
	}

	resp, err := authService.CreateAPIKey(ctx, req)
	if err != nil {
		log.Printf("Failed to create demo API key: %v", err)
		return
	}

	// For demo purposes, set known values
	authService.ApiKeys.Store("demo-api-key", &grpcSvc.APIKeyData{
		ID:          "demo-api-key",
		Name:        "Demo API Key",
		Secret:      "demo-secret",
		Permissions: req.Permissions,
		CreatedAt:   time.Now(),
		IsActive:    true,
	})
}

func handleShutdown(ctx context.Context, grpcServer *grpc.Server) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case <-sigChan:
		log.Println("Shutdown signal received, gracefully stopping...")
		grpcServer.GracefulStop()
	case <-ctx.Done():
		grpcServer.Stop()
	}
}