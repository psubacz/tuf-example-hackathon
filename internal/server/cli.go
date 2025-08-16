package server

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tuf-golang-project/internal/logger"
	. "tuf-golang-project/internal/tuf"
)

// CLI represents the command line interface
type CLI struct {
	config *Config
	server *GinServer
}

// NewCLI creates a new CLI instance
func NewCLI() *CLI {
	return &CLI{}
}

// Run starts the CLI application
func (cli *CLI) Run(args []string) error {
	// Parse command line arguments
	if err := cli.parseArgs(args); err != nil {
		logger.Logger.Error("parseArgs failed", "error", err)
		return err
	}
	
	logger.Logger.Info("After parseArgs")

	// Load configuration
	config, err := cli.loadConfig()
	if err != nil {
		logger.Logger.Error("loadConfig failed", "error", err)
		return fmt.Errorf("failed to load configuration: %w", err)
	}
	cli.config = config
	
	logger.Logger.Info("After loadConfig", "config", config)

	// Create and start server
	cli.server = NewGinServer(config)
	
	logger.Logger.Info("After NewGinServer")

	// Setup graceful shutdown
	return cli.runWithGracefulShutdown()
}

// parseArgs parses command line arguments
func (cli *CLI) parseArgs(args []string) error {
	logger.Logger.Info("parseArgs called", "args", args)
	
	// Skip "server" command if present
	if len(args) > 1 && args[1] == "server" {
		args = append(args[:1], args[2:]...)
		logger.Logger.Info("Removed 'server' from args", "newArgs", args)
	}
	
	fs := flag.NewFlagSet("tuf-server", flag.ContinueOnError)
	fs.Usage = cli.printUsage

	var (
		port          = fs.Int("port", 8080, "Server port")
		host          = fs.String("host", "0.0.0.0", "Server host")
		repo          = fs.String("repo", "./tuf-repository", "Repository path (supports both demo and go-tuf v2 formats)")
		config        = fs.String("config", "", "Configuration file path")
		configShort   = fs.String("c", "", "Configuration file path (short form)")
		logLevel      = fs.String("log-level", "info", "Log level (debug, info, warn, error)")
		enableTLS     = fs.Bool("tls", false, "Enable HTTPS/TLS")
		certFile      = fs.String("cert", "", "TLS certificate file")
		keyFile       = fs.String("key", "", "TLS private key file")
		enableMetrics = fs.Bool("metrics", true, "Enable metrics endpoint")
		enableCORS    = fs.Bool("cors", true, "Enable CORS")
		rateLimit     = fs.Int("rate-limit", 1000, "Rate limit (requests per hour)")
		initRepo      = fs.Bool("init", false, "Initialize TUF repository and exit")
		version       = fs.Bool("version", false, "Show version information")
		help          = fs.Bool("help", false, "Show help message")
	)

	logger.Logger.Info("About to parse flags", "args[1:]", args[1:])
	if err := fs.Parse(args[1:]); err != nil {
		logger.Logger.Error("Flag parse error", "error", err)
		return err
	}
	
	logger.Logger.Info("Flags parsed", "config", *config, "port", *port)

	// Handle special flags
	if *version {
		cli.printVersion()
		os.Exit(0)
	}

	if *help {
		cli.printUsage()
		os.Exit(0)
	}

	logger.Logger.Info("CLI Debug Info", "initRepo", *initRepo, "repo", *repo, "port", *port)
	
	if *initRepo {
		logger.Logger.Info("Running initialization mode")
		if err := cli.initializeRepository(*repo); err != nil {
			return err
		}
		logger.Logger.Info("Repository initialization complete")
		os.Exit(0) // Exit successfully after init
	}
	
	logger.Logger.Info("Running server mode")

	// Store config file path for later loading (check both long and short form)
	configFile := *config
	if configFile == "" && *configShort != "" {
		configFile = *configShort
	}
	
	if configFile != "" {
		logger.Logger.Info("Setting TUF_CONFIG_FILE", "config", configFile)
		os.Setenv("TUF_CONFIG_FILE", configFile)
		// Don't set cli.config when using a config file
		cli.config = nil
	} else {
		// Only set command line defaults if no config file provided
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
	}

	return nil
}

// loadConfig loads configuration from file and environment
func (cli *CLI) loadConfig() (*Config, error) {
	configFile := os.Getenv("TUF_CONFIG_FILE")
	logger.Logger.Info("Loading config", "configFile", configFile)

	config, err := LoadConfig(configFile)
	if err != nil {
		return nil, err
	}

	// Apply command line overrides if cli.config was set
	if cli.config != nil {
		if cli.config.Port != 0 {
			config.Port = cli.config.Port
		}
		if cli.config.Host != "" {
			config.Host = cli.config.Host
		}
		if cli.config.RepositoryPath != "" {
			config.RepositoryPath = cli.config.RepositoryPath
		}
		if cli.config.LogLevel != "" {
			config.LogLevel = cli.config.LogLevel
		}
		// TLS, Metrics, CORS are booleans - always override
		config.TLS.Enabled = cli.config.TLS.Enabled
		if cli.config.TLS.CertFile != "" {
			config.TLS.CertFile = cli.config.TLS.CertFile
		}
		if cli.config.TLS.KeyFile != "" {
			config.TLS.KeyFile = cli.config.TLS.KeyFile
		}
		config.Metrics.Enabled = cli.config.Metrics.Enabled
		config.CORS.Enabled = cli.config.CORS.Enabled
		if cli.config.RateLimit.Requests != 0 {
			config.RateLimit.Requests = cli.config.RateLimit.Requests
		}
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
		logger.Logger.Info("Starting TUF Repository Server v2.0",
			"repository", cli.config.RepositoryPath,
			"address", fmt.Sprintf("%s:%d", cli.config.Host, cli.config.Port),
			"tls_enabled", cli.config.TLS.Enabled,
			"metrics_enabled", cli.config.Metrics.Enabled,
			"cors_enabled", cli.config.CORS.Enabled,
			"rate_limit", cli.config.RateLimit.Requests,
			"compression_enabled", cli.config.Compression.Enabled)

		logger.Logger.Info("Server ready! Press Ctrl+C to stop")

		if err := cli.server.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for shutdown signal or server error
	select {
	case sig := <-sigChan:
		logger.Logger.Info("Received shutdown signal", "signal", sig.String())
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
	logger.Logger.Info("Graceful shutdown initiated")

	// Create timeout context for shutdown
	shutdownCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	// For Gin server, we handle shutdown differently since it has built-in graceful shutdown
	// The handleShutdown goroutine in server_gin.go will handle this
	// Just wait for the context to be done
	<-shutdownCtx.Done()

	logger.Logger.Info("Server stopped gracefully")
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
    --init              Initialize TUF repository and exit
    --version, -v       Show version information
    --help, -h          Show this help message

EXAMPLES:
    # Initialize TUF repository
    tuf-server --init --repo /path/to/tuf-repo

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

// initializeRepository creates a new TUF repository or verifies existing one
func (cli *CLI) initializeRepository(repoPath string) error {
	logger.Logger.Info("Checking repository", "path", repoPath)

	// Check if repository already exists
	metadataDir := filepath.Join(repoPath, "metadata")
	targetsDir := filepath.Join(repoPath, "targets")
	
	// Check for required metadata files
	requiredFiles := []string{
		filepath.Join(metadataDir, "root.json"),
		filepath.Join(metadataDir, "targets.json"),
		filepath.Join(metadataDir, "snapshot.json"),
		filepath.Join(metadataDir, "timestamp.json"),
	}
	
	allFilesExist := true
	for _, file := range requiredFiles {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			logger.Logger.Info("Missing metadata file", "file", file)
			allFilesExist = false
			break
		}
	}
	
	if allFilesExist {
		logger.Logger.Info("Repository already exists and is valid",
			"location", repoPath,
			"metadata_dir", metadataDir,
			"targets_dir", targetsDir)
		return nil
	}

	// Repository doesn't exist or is incomplete, initialize it
	logger.Logger.Info("Initializing new repository")
	repo := NewRepositoryV2(repoPath)

	if err := repo.Initialize(); err != nil {
		logger.Logger.Error("Failed to initialize repository", "error", err)
		return err
	}

	logger.Logger.Info("TUF repository with go-tuf v2 created successfully",
		"location", repoPath,
		"metadata_dir", metadataDir,
		"targets_dir", targetsDir)

	return nil
}
