package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"
)

// extractAPIKey extracts API key from gRPC metadata
func extractAPIKey(ctx context.Context, headerName string) (string, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return "", nil
	}

	// Check header
	if values := md.Get(headerName); len(values) > 0 {
		return values[0], nil
	}

	// Check authorization header
	if values := md.Get("authorization"); len(values) > 0 {
		auth := values[0]
		if strings.HasPrefix(auth, "Bearer ") {
			return strings.TrimPrefix(auth, "Bearer "), nil
		}
	}

	return "", nil
}

// getAPIKeyFromContext extracts API key from context
func getAPIKeyFromContext(ctx context.Context) string {
	if apiKey, ok := ctx.Value("api_key").(string); ok {
		return apiKey
	}
	return ""
}

// addOutgoingMetadata adds metadata to outgoing context
func addOutgoingMetadata(ctx context.Context, key, value string) context.Context {
	return metadata.AppendToOutgoingContext(ctx, key, value)
}