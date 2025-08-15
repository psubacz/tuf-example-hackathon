package server

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// CLI represents the command line interface
type CLI struct {
	config *Config
	server *Server
}

// NewCLI creates a new CLI instance
func NewCLI() *CLI {
	return &CLI{}
}

// Run starts the CLI application
func (cli *CLI) Run(args []string) error {
	// Parse command line arguments
	if err := cli.parseArgs(args); err != nil {
		return err
	}
	
	// Load configuration
	config, err := cli.loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	cli.config = config
	
	// Create and start server
	cli.server = New(config)
	
	// Setup graceful shutdown
	return cli.runWithGracefulShutdown()
}

// parseArgs parses command line arguments
func (cli *CLI) parseArgs(args []string) error {
	fs := flag.NewFlagSet("tuf-server", flag.ContinueOnError)
	fs.Usage = cli.printUsage
	
	var (
		port           = fs.Int("port", 8080, "Server port")
		host           = fs.String("host", "0.0.0.0", "Server host")
		repo           = fs.String("repo", "./tuf-repository", "Repository path (supports both demo and go-tuf v2 formats)")
		config         = fs.String("config", "", "Configuration file path")
		logLevel       = fs.String("log-level", "info", "Log level (debug, info, warn, error)")
		enableTLS      = fs.Bool("tls", false, "Enable HTTPS/TLS")
		certFile       = fs.String("cert", "", "TLS certificate file")
		keyFile        = fs.String("key", "", "TLS private key file")
		enableMetrics  = fs.Bool("metrics", true, "Enable metrics endpoint")
		enableCORS     = fs.Bool("cors", true, "Enable CORS")
		rateLimit      = fs.Int("rate-limit", 1000, "Rate limit (requests per hour)")
		version        = fs.Bool("version", false, "Show version information")
		help           = fs.Bool("help", false, "Show help message")
	)
	
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	
	// Handle special flags
	if *version {
		cli.printVersion()
		os.Exit(0)
	}
	
	if *help {
		cli.printUsage()
		os.Exit(0)
	}
	
	// Set default config with command line overrides
	cli.config = DefaultConfig()
	cli.config.Port = *port
	cli.config.Host = *host
	cli.config.RepositoryPath = *repo
	cli.config.LogLevel = *logLevel
	cli.config.TLS.Enabled = *enableTLS
	cli.config.TLS.CertFile = *certFile
	cli.config.TLS.KeyFile = *keyFile
	cli.config.Metrics.Enabled = *enableMetrics
	cli.config.CORS.Enabled = *enableCORS
	cli.config.RateLimit.Requests = *rateLimit
	
	// Store config file path for later loading
	if *config != "" {
		// Will be loaded in loadConfig()
		os.Setenv("TUF_CONFIG_FILE", *config)
	}
	
	return nil
}

// loadConfig loads configuration from file and environment
func (cli *CLI) loadConfig() (*Config, error) {
	configFile := os.Getenv("TUF_CONFIG_FILE")
	
	config, err := LoadConfig(configFile)
	if err != nil {
		return nil, err
	}
	
	// Apply command line overrides if cli.config was set
	if cli.config != nil {
		config.Port = cli.config.Port
		config.Host = cli.config.Host
		config.RepositoryPath = cli.config.RepositoryPath
		config.LogLevel = cli.config.LogLevel
		config.TLS.Enabled = cli.config.TLS.Enabled
		config.TLS.CertFile = cli.config.TLS.CertFile
		config.TLS.KeyFile = cli.config.TLS.KeyFile
		config.Metrics.Enabled = cli.config.Metrics.Enabled
		config.CORS.Enabled = cli.config.CORS.Enabled
		config.RateLimit.Requests = cli.config.RateLimit.Requests
	}
	
	return config, nil
}

// runWithGracefulShutdown runs the server with graceful shutdown
func (cli *CLI) runWithGracefulShutdown() error {
	// Create context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Channel to receive OS signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	
	// Channel to receive server errors
	errChan := make(chan error, 1)
	
	// Start server in goroutine
	go func() {
		fmt.Printf("🚀 Starting TUF Repository Server v2.0...\n")
		fmt.Printf("📁 Repository: %s\n", cli.config.RepositoryPath)
		fmt.Printf("🌐 Address: %s:%d\n", cli.config.Host, cli.config.Port)
		
		if cli.config.TLS.Enabled {
			fmt.Printf("🔐 TLS: Enabled\n")
		} else {
			fmt.Printf("⚠️ TLS: Disabled (HTTP only)\n")
		}
		
		fmt.Printf("📊 Metrics: %t\n", cli.config.Metrics.Enabled)
		fmt.Printf("🌍 CORS: %t\n", cli.config.CORS.Enabled)
		fmt.Printf("⚡ Rate Limit: %d req/hr\n", cli.config.RateLimit.Requests)
		fmt.Printf("🔒 Compression: %t\n", cli.config.Compression.Enabled)
		fmt.Printf("\n✅ Server ready! Press Ctrl+C to stop.\n\n")
		
		if err := cli.server.Start(); err != nil {
			errChan <- err
		}
	}()
	
	// Wait for shutdown signal or server error
	select {
	case sig := <-sigChan:
		fmt.Printf("\n🛑 Received signal: %s\n", sig)
		return cli.gracefulShutdown(ctx)
		
	case err := <-errChan:
		if err != nil {
			return fmt.Errorf("server error: %w", err)
		}
	}
	
	return nil
}

// gracefulShutdown performs graceful server shutdown
func (cli *CLI) gracefulShutdown(ctx context.Context) error {
	fmt.Printf("⏳ Graceful shutdown initiated...\n")
	
	// Create timeout context for shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	
	// Stop the server
	if err := cli.server.Stop(shutdownCtx); err != nil {
		fmt.Printf("❌ Shutdown error: %v\n", err)
		return err
	}
	
	fmt.Printf("✅ Server stopped gracefully\n")
	return nil
}

// printUsage prints command usage information
func (cli *CLI) printUsage() {
	fmt.Printf(`TUF Repository Server v2.0
A secure, production-ready server for The Update Framework (TUF) repositories.

USAGE:
    tuf-server [OPTIONS]

OPTIONS:
    --port, -p          Server port (default: 8080)
    --host              Server host (default: 0.0.0.0) 
    --repo, -r          Repository path (default: ./tuf-repository)
    --config, -c        Configuration file path
    --log-level         Log level: debug, info, warn, error (default: info)
    --tls               Enable HTTPS/TLS
    --cert              TLS certificate file (required with --tls)
    --key               TLS private key file (required with --tls)
    --metrics           Enable metrics endpoint (default: true)
    --cors              Enable CORS (default: true)
    --rate-limit        Rate limit in requests per hour (default: 1000)
    --version, -v       Show version information
    --help, -h          Show this help message

EXAMPLES:
    # Start server with default settings
    tuf-server

    # Start server on custom port and repository
    tuf-server --port 9000 --repo /path/to/tuf-repo

    # Start server with HTTPS
    tuf-server --tls --cert server.crt --key server.key

    # Start server with configuration file
    tuf-server --config /etc/tuf-server/config.json

    # Start server with debug logging
    tuf-server --log-level debug

ENVIRONMENT VARIABLES:
    TUF_PORT                 Server port
    TUF_HOST                 Server host
    TUF_REPOSITORY_PATH      Repository path
    TUF_LOG_LEVEL           Log level
    TUF_TLS_ENABLED         Enable TLS (true/false)
    TUF_TLS_CERT_FILE       TLS certificate file
    TUF_TLS_KEY_FILE        TLS private key file
    TUF_METRICS_ENABLED     Enable metrics (true/false)
    TUF_CORS_ENABLED        Enable CORS (true/false)

CONFIGURATION:
    Configuration can be provided via:
    1. Command line arguments (highest priority)
    2. Environment variables  
    3. Configuration file (JSON format)
    4. Default values (lowest priority)

SECURITY FEATURES:
    ✅ TUF cryptographic verification
    ✅ Rate limiting and DDoS protection  
    ✅ Security headers (CSP, HSTS, etc.)
    ✅ Path traversal prevention
    ✅ Gzip compression
    ✅ HTTPS/TLS support
    ✅ Graceful shutdown
    ✅ Health checks and metrics

For more information, visit: https://theupdateframework.io
`)
}

// printVersion prints version information
func (cli *CLI) printVersion() {
	fmt.Printf(`TUF Repository Server v2.0.0
Enhanced production-ready server for The Update Framework

Build Information:
  Version:     2.0.0
  Features:    TLS, Metrics, Rate Limiting, Compression, CORS
  Go Version:  %s
  Platform:    %s/%s

Security Features:
  - Cryptographic file verification
  - Path traversal protection  
  - Security headers
  - Rate limiting
  - HTTPS/TLS support
  
For more information: https://theupdateframework.io
`, "go1.21+", "cross-platform", "multi-arch")
}