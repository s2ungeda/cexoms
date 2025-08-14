package main

import (
	"context"
	"fmt"
	"log"

	omsv1 "github.com/mExOms/oms/pkg/proto/oms/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	fmt.Println("=== Testing gRPC API Gateway ===\n")

	// Connect to gRPC server
	conn, err := grpc.Dial("localhost:9090", grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatal("Failed to connect:", err)
	}
	defer conn.Close()

	// Test authentication
	authClient := omsv1.NewAuthServiceClient(conn)
	
	fmt.Println("Testing authentication...")
	authResp, err := authClient.Authenticate(context.Background(), &omsv1.AuthRequest{
		ApiKey: "demo-api-key",
		Secret: "demo-secret",
	})
	
	if err != nil {
		log.Fatal("Authentication failed:", err)
	}
	
	fmt.Println("✓ Authentication successful!")
	fmt.Printf("  Token: %s...\n", authResp.Token[:20])
	fmt.Printf("  Expires at: %v\n", authResp.ExpiresAt)
	fmt.Printf("  Permissions: %v\n", authResp.Permissions)
	
	fmt.Println("\n✓ gRPC API Gateway is working correctly!")
}