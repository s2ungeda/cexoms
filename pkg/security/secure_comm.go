package security

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"sync"
	"time"
)

// SecureCommunicationManager manages secure communication with TLS/mTLS
type SecureCommunicationManager struct {
	mu              sync.RWMutex
	certStore       *CertificateStore
	tlsConfigs      map[string]*tls.Config
	trustedCAs      *x509.CertPool
	clientCerts     map[string]tls.Certificate
	renewalSchedule map[string]time.Time
}

// CertificateStore stores certificates
type CertificateStore struct {
	mu          sync.RWMutex
	certs       map[string]*x509.Certificate
	privateKeys map[string]*rsa.PrivateKey
}

// NewSecureCommunicationManager creates a new secure communication manager
func NewSecureCommunicationManager() *SecureCommunicationManager {
	return &SecureCommunicationManager{
		certStore:       NewCertificateStore(),
		tlsConfigs:      make(map[string]*tls.Config),
		trustedCAs:      x509.NewCertPool(),
		clientCerts:     make(map[string]tls.Certificate),
		renewalSchedule: make(map[string]time.Time),
	}
}

// NewCertificateStore creates a new certificate store
func NewCertificateStore() *CertificateStore {
	return &CertificateStore{
		certs:       make(map[string]*x509.Certificate),
		privateKeys: make(map[string]*rsa.PrivateKey),
	}
}

// CreateServerTLSConfig creates TLS config for server
func (scm *SecureCommunicationManager) CreateServerTLSConfig(certPath, keyPath string, requireClientAuth bool) (*tls.Config, error) {
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load certificate: %w", err)
	}
	
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
		},
		PreferServerCipherSuites: true,
		CurvePreferences: []tls.CurveID{
			tls.CurveP384,
			tls.CurveP256,
		},
	}
	
	if requireClientAuth {
		config.ClientAuth = tls.RequireAndVerifyClientCert
		config.ClientCAs = scm.trustedCAs
	}
	
	return config, nil
}

// CreateClientTLSConfig creates TLS config for client
func (scm *SecureCommunicationManager) CreateClientTLSConfig(serverName string, clientCertPath, clientKeyPath string) (*tls.Config, error) {
	config := &tls.Config{
		ServerName: serverName,
		MinVersion: tls.VersionTLS13,
		RootCAs:    scm.trustedCAs,
	}
	
	// Load client certificate if provided
	if clientCertPath != "" && clientKeyPath != "" {
		cert, err := tls.LoadX509KeyPair(clientCertPath, clientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		config.Certificates = []tls.Certificate{cert}
	}
	
	return config, nil
}

// GenerateSelfSignedCert generates a self-signed certificate
func (scm *SecureCommunicationManager) GenerateSelfSignedCert(hosts []string, validFor time.Duration) (tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to generate private key: %w", err)
	}
	
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"mExOms"},
			Country:       []string{"US"},
			Province:      []string{""},
			Locality:      []string{""},
			StreetAddress: []string{""},
			PostalCode:    []string{""},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}
	
	// Add hosts
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}
	
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return tls.Certificate{}, fmt.Errorf("failed to create certificate: %w", err)
	}
	
	cert := tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  priv,
	}
	
	return cert, nil
}

// GenerateCA generates a Certificate Authority
func (scm *SecureCommunicationManager) GenerateCA(commonName string, validFor time.Duration) (*x509.Certificate, *rsa.PrivateKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization:  []string{"mExOms CA"},
			Country:       []string{"US"},
			CommonName:    commonName,
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(validFor),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            2,
	}
	
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}
	
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	
	return cert, priv, nil
}

// IssueCertificate issues a certificate signed by CA
func (scm *SecureCommunicationManager) IssueCertificate(ca *x509.Certificate, caPrivKey *rsa.PrivateKey, commonName string, hosts []string, validFor time.Duration) (*x509.Certificate, *rsa.PrivateKey, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate private key: %w", err)
	}
	
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate serial number: %w", err)
	}
	
	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"mExOms"},
			CommonName:   commonName,
		},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(validFor),
		KeyUsage:     x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		SubjectKeyId: []byte{1, 2, 3, 4, 6},
	}
	
	// Add hosts
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			template.IPAddresses = append(template.IPAddresses, ip)
		} else {
			template.DNSNames = append(template.DNSNames, h)
		}
	}
	
	certDER, err := x509.CreateCertificate(rand.Reader, &template, ca, &priv.PublicKey, caPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create certificate: %w", err)
	}
	
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to parse certificate: %w", err)
	}
	
	return cert, priv, nil
}

// AddTrustedCA adds a trusted CA certificate
func (scm *SecureCommunicationManager) AddTrustedCA(caCert *x509.Certificate) {
	scm.mu.Lock()
	defer scm.mu.Unlock()
	
	scm.trustedCAs.AddCert(caCert)
}

// AddTrustedCAPEM adds a trusted CA from PEM
func (scm *SecureCommunicationManager) AddTrustedCAPEM(pemData []byte) error {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return fmt.Errorf("failed to decode PEM")
	}
	
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return fmt.Errorf("failed to parse certificate: %w", err)
	}
	
	scm.AddTrustedCA(cert)
	return nil
}

// SaveCertificate saves a certificate to store
func (scm *SecureCommunicationManager) SaveCertificate(name string, cert *x509.Certificate, privKey *rsa.PrivateKey) error {
	scm.mu.Lock()
	defer scm.mu.Unlock()
	
	scm.certStore.mu.Lock()
	defer scm.certStore.mu.Unlock()
	
	scm.certStore.certs[name] = cert
	scm.certStore.privateKeys[name] = privKey
	
	// Schedule renewal
	renewalTime := cert.NotAfter.Add(-30 * 24 * time.Hour) // 30 days before expiry
	scm.renewalSchedule[name] = renewalTime
	
	return nil
}

// GetCertificate retrieves a certificate from store
func (scm *SecureCommunicationManager) GetCertificate(name string) (*x509.Certificate, *rsa.PrivateKey, error) {
	scm.certStore.mu.RLock()
	defer scm.certStore.mu.RUnlock()
	
	cert, certExists := scm.certStore.certs[name]
	privKey, keyExists := scm.certStore.privateKeys[name]
	
	if !certExists || !keyExists {
		return nil, nil, fmt.Errorf("certificate not found: %s", name)
	}
	
	return cert, privKey, nil
}

// ExportCertificatePEM exports certificate as PEM
func (scm *SecureCommunicationManager) ExportCertificatePEM(cert *x509.Certificate) ([]byte, error) {
	pemBlock := &pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	}
	
	return pem.EncodeToMemory(pemBlock), nil
}

// ExportPrivateKeyPEM exports private key as PEM
func (scm *SecureCommunicationManager) ExportPrivateKeyPEM(privKey *rsa.PrivateKey) ([]byte, error) {
	privKeyBytes, err := x509.MarshalPKCS8PrivateKey(privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	
	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privKeyBytes,
	}
	
	return pem.EncodeToMemory(pemBlock), nil
}

// VerifyCertificateChain verifies a certificate chain
func (scm *SecureCommunicationManager) VerifyCertificateChain(cert *x509.Certificate, intermediates []*x509.Certificate) error {
	opts := x509.VerifyOptions{
		Roots:         scm.trustedCAs,
		Intermediates: x509.NewCertPool(),
	}
	
	for _, inter := range intermediates {
		opts.Intermediates.AddCert(inter)
	}
	
	_, err := cert.Verify(opts)
	return err
}

// CheckCertificateExpiry checks if certificate is about to expire
func (scm *SecureCommunicationManager) CheckCertificateExpiry(cert *x509.Certificate, warningDays int) bool {
	warningTime := time.Now().Add(time.Duration(warningDays) * 24 * time.Hour)
	return cert.NotAfter.Before(warningTime)
}

// GetCertificatesNeedingRenewal returns certificates that need renewal
func (scm *SecureCommunicationManager) GetCertificatesNeedingRenewal() []string {
	scm.mu.RLock()
	defer scm.mu.RUnlock()
	
	var needsRenewal []string
	now := time.Now()
	
	for name, renewalTime := range scm.renewalSchedule {
		if now.After(renewalTime) {
			needsRenewal = append(needsRenewal, name)
		}
	}
	
	return needsRenewal
}

// CreateMutualTLSConfig creates mTLS configuration
func (scm *SecureCommunicationManager) CreateMutualTLSConfig(serverCert, serverKey, clientCA string) (*tls.Config, error) {
	// Load server certificate
	cert, err := tls.LoadX509KeyPair(serverCert, serverKey)
	if err != nil {
		return nil, fmt.Errorf("failed to load server certificate: %w", err)
	}
	
	// Load client CA
	caCert, err := os.ReadFile(clientCA)
	if err != nil {
		return nil, fmt.Errorf("failed to read client CA: %w", err)
	}
	
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse client CA")
	}
	
	config := &tls.Config{
		Certificates: []tls.Certificate{cert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caCertPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}
	
	return config, nil
}

// ValidateTLSConnection validates TLS connection parameters
func (scm *SecureCommunicationManager) ValidateTLSConnection(conn *tls.Conn) error {
	state := conn.ConnectionState()
	
	// Check TLS version
	if state.Version < tls.VersionTLS12 {
		return fmt.Errorf("TLS version too old: %x", state.Version)
	}
	
	// Check cipher suite
	validCiphers := map[uint16]bool{
		tls.TLS_AES_256_GCM_SHA384:         true,
		tls.TLS_AES_128_GCM_SHA256:         true,
		tls.TLS_CHACHA20_POLY1305_SHA256:  true,
		tls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384:   true,
		tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256:   true,
		tls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384: true,
		tls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256: true,
	}
	
	if !validCiphers[state.CipherSuite] {
		return fmt.Errorf("weak cipher suite: %x", state.CipherSuite)
	}
	
	// Verify peer certificates
	if len(state.PeerCertificates) == 0 {
		return fmt.Errorf("no peer certificates")
	}
	
	return nil
}