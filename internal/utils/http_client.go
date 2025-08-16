package utils

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"time"
)

// HTTPClientConfig represents configuration for creating HTTP clients with custom TLS
type HTTPClientConfig struct {
	CACertFile string
}

// CreateHTTPClient creates an HTTP client with optional custom CA certificate
func CreateHTTPClient(config HTTPClientConfig, timeout time.Duration) (*http.Client, error) {
	client := &http.Client{
		Timeout: timeout,
	}

	// If CA cert file is specified, configure custom TLS
	if config.CACertFile != "" {
		tlsConfig, err := createTLSConfig(config.CACertFile)
		if err != nil {
			return nil, fmt.Errorf("failed to create TLS config: %w", err)
		}

		transport := &http.Transport{
			TLSClientConfig: tlsConfig,
		}
		client.Transport = transport
	}

	return client, nil
}

// createTLSConfig creates a TLS config with custom CA certificate
func createTLSConfig(caCertFile string) (*tls.Config, error) {
	// Load CA certificate
	caCert, err := os.ReadFile(caCertFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate file %s: %w", caCertFile, err)
	}

	// Create cert pool and add CA cert
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to parse CA certificate from %s", caCertFile)
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		RootCAs: caCertPool,
	}

	return tlsConfig, nil
}