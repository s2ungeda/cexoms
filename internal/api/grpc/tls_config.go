package grpc

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"

	"google.golang.org/grpc/credentials"
)

// TLSConfig represents TLS configuration options
type TLSConfig struct {
	// Server certificate and key
	CertFile string
	KeyFile  string
	
	// Client CA for mutual TLS
	ClientCAFile string
	
	// Server name for validation
	ServerName string
	
	// Enable mutual TLS
	RequireClientCert bool
	
	// Allowed cipher suites (if empty, uses secure defaults)
	CipherSuites []uint16
	
	// Min TLS version (defaults to TLS 1.3)
	MinVersion uint16
}

// LoadTLSCredentials loads TLS credentials for server
func LoadTLSCredentials(config *TLSConfig) (credentials.TransportCredentials, error) {
	// Load server certificate and key
	cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificates: %w", err)
	}
	
	// Create TLS configuration
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13, // Force TLS 1.3
	}
	
	// Set cipher suites for TLS 1.3
	if len(config.CipherSuites) > 0 {
		tlsConfig.CipherSuites = config.CipherSuites
	} else {
		// Use secure TLS 1.3 cipher suites
		tlsConfig.CipherSuites = []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		}
	}
	
	// Configure mutual TLS if client CA is provided
	if config.ClientCAFile != "" {
		certPool, err := loadCAPool(config.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client CA: %w", err)
		}
		
		tlsConfig.ClientCAs = certPool
		
		if config.RequireClientCert {
			tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
		} else {
			tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
		}
	}
	
	// Additional security settings
	tlsConfig.PreferServerCipherSuites = true
	
	return credentials.NewTLS(tlsConfig), nil
}

// LoadClientTLSCredentials loads TLS credentials for client
func LoadClientTLSCredentials(config *TLSConfig) (credentials.TransportCredentials, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS13, // Force TLS 1.3
	}
	
	// Set server name for certificate validation
	if config.ServerName != "" {
		tlsConfig.ServerName = config.ServerName
	}
	
	// Load client certificates if provided (for mutual TLS)
	if config.CertFile != "" && config.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificates: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}
	
	// Load CA for server verification
	if config.ClientCAFile != "" {
		certPool, err := loadCAPool(config.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load CA: %w", err)
		}
		tlsConfig.RootCAs = certPool
	}
	
	return credentials.NewTLS(tlsConfig), nil
}

// loadCAPool loads CA certificates from file
func loadCAPool(caFile string) (*x509.CertPool, error) {
	ca, err := ioutil.ReadFile(caFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA file: %w", err)
	}
	
	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(ca) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}
	
	return certPool, nil
}

// GenerateTLSConfig creates a secure TLS configuration
func GenerateTLSConfig() *tls.Config {
	return &tls.Config{
		MinVersion: tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.X25519,
			tls.CurveP256,
		},
		ClientSessionCache: tls.NewLRUClientSessionCache(100),
		SessionTicketsDisabled: false, // Enable for performance
	}
}

// VerifyPeerCertificate provides custom certificate verification
func VerifyPeerCertificate(rawCerts [][]byte, verifiedChains [][]*x509.Certificate) error {
	// Custom verification logic
	// For example, check certificate attributes, extensions, etc.
	
	if len(verifiedChains) == 0 {
		return fmt.Errorf("no verified chains")
	}
	
	// Get the peer certificate
	cert := verifiedChains[0][0]
	
	// Example: Check certificate validity
	if cert.NotAfter.Before(cert.NotBefore) {
		return fmt.Errorf("invalid certificate validity period")
	}
	
	// Example: Check key usage
	if cert.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
		return fmt.Errorf("certificate missing digital signature key usage")
	}
	
	return nil
}