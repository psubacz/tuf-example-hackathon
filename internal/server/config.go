package server

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
	
	"tuf-golang-project/internal/logger"
)

// Config holds server configuration
type Config struct {
	// Server settings
	Port           int    `json:"port"`
	Host           string `json:"host"`
	RepositoryPath string `json:"repository_path"`
	
	// Timeouts
	ReadTimeout  int `json:"read_timeout"`
	WriteTimeout int `json:"write_timeout"`
	IdleTimeout  int `json:"idle_timeout"`
	
	// Logging
	LogLevel  string `json:"log_level"`
	LogFormat string `json:"log_format"` // "text" or "json"
	
	// Security
	TLS  TLSConfig  `json:"tls"`
	CORS CORSConfig `json:"cors"`
	
	// Rate limiting
	RateLimit RateLimitConfig `json:"rate_limit"`
	
	// Performance
	Compression CompressionConfig `json:"compression"`
	Cache       CacheConfig       `json:"cache"`
	
	// Monitoring
	Metrics MetricsConfig `json:"metrics"`
	
	// Authentication
	Auth AuthConfig `json:"auth"`
	
	// Storage backend
	Storage StorageConfig `json:"storage"`
	
	// OpenTelemetry tracing
	Tracing TracingConfig `json:"tracing"`
	
	// Request coalescing
	Coalescing CoalescingConfig `json:"coalescing"`
	
	// Circuit breaker
	CircuitBreaker CircuitBreakerConfig `json:"circuit_breaker"`
	
	// Retry configuration
	Retry RetryConfig `json:"retry"`
}

// StorageConfig holds storage backend configuration
type StorageConfig struct {
	Type       string                 `json:"type"`       // "filesystem", "s3", "gcs", "azure"
	Properties map[string]interface{} `json:"properties"` // Backend-specific properties
}

// AuthConfig holds authentication configuration
type AuthConfig struct {
	Enabled       bool              `json:"enabled"`
	JWTSecret     string            `json:"jwt_secret"`
	JWTExpiration string            `json:"jwt_expiration"` // e.g., "24h"
	APIKeys       map[string]string `json:"api_keys"`       // key -> description
	AdminUsers    []AdminUser       `json:"admin_users"`
}

// AdminUser represents an admin user
type AdminUser struct {
	Username string   `json:"username"`
	Password string   `json:"password"` // Should be hashed in production
	Role     string   `json:"role"`
	Permissions []string `json:"permissions"`
}

// TLSConfig holds TLS/HTTPS configuration
type TLSConfig struct {
	Enabled    bool   `json:"enabled"`
	CertFile   string `json:"cert_file"`
	KeyFile    string `json:"key_file"`
	AutoTLS    bool   `json:"auto_tls"`
	CACertFile string `json:"ca_cert_file"` // Custom CA certificate for client connections
}

// CORSConfig holds CORS configuration
type CORSConfig struct {
	Enabled          bool     `json:"enabled"`
	AllowedOrigins   []string `json:"allowed_origins"`
	AllowedMethods   []string `json:"allowed_methods"`
	AllowedHeaders   []string `json:"allowed_headers"`
	ExposedHeaders   []string `json:"exposed_headers"`
	AllowCredentials bool     `json:"allow_credentials"`
	MaxAge           int      `json:"max_age"`
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled     bool          `json:"enabled"`
	Requests    int           `json:"requests"`
	Window      time.Duration `json:"window"`
	BurstSize   int           `json:"burst_size"`
	IPWhitelist []string      `json:"ip_whitelist"`
	IPBlacklist []string      `json:"ip_blacklist"`
}

// CompressionConfig holds compression configuration
type CompressionConfig struct {
	Enabled   bool     `json:"enabled"`
	Level     int      `json:"level"` // 1-9 for gzip
	MinSize   int      `json:"min_size"`
	Types     []string `json:"types"`
}

// CacheConfig holds caching configuration
type CacheConfig struct {
	Enabled     bool          `json:"enabled"`
	TTL         time.Duration `json:"ttl"`
	MaxSize     int           `json:"max_size"`
	CleanupFreq time.Duration `json:"cleanup_freq"`
}

// MetricsConfig holds metrics configuration
type MetricsConfig struct {
	Enabled bool   `json:"enabled"`
	Path    string `json:"path"`
}

// TracingConfig holds OpenTelemetry tracing configuration
type TracingConfig struct {
	Enabled      bool              `json:"enabled"`
	ServiceName  string            `json:"service_name"`
	Endpoint     string            `json:"endpoint"`
	SamplingRate float64           `json:"sampling_rate"`
	Insecure     bool              `json:"insecure"`
	Headers      map[string]string `json:"headers"`
}

// CoalescingConfig holds request coalescing configuration
type CoalescingConfig struct {
	Enabled bool          `json:"enabled"`
	TTL     time.Duration `json:"ttl"`
	MaxWait time.Duration `json:"max_wait"`
}

// CircuitBreakerConfig holds circuit breaker configuration
type CircuitBreakerConfig struct {
	Enabled     bool          `json:"enabled"`
	MaxRequests uint32        `json:"max_requests"`
	Interval    time.Duration `json:"interval"`
	Timeout     time.Duration `json:"timeout"`
	Threshold   float64       `json:"threshold"`
	MinRequests uint32        `json:"min_requests"`
}

// RetryConfig holds retry configuration
type RetryConfig struct {
	Enabled        bool          `json:"enabled"`
	MaxRetries     int           `json:"max_retries"`
	InitialDelay   time.Duration `json:"initial_delay"`
	MaxDelay       time.Duration `json:"max_delay"`
	Multiplier     float64       `json:"multiplier"`
	JitterFraction float64       `json:"jitter_fraction"`
}

// DefaultConfig returns default server configuration
func DefaultConfig() *Config {
	return &Config{
		Port:           8080,
		Host:           "0.0.0.0",
		RepositoryPath: "./tuf-repository",
		ReadTimeout:    30,
		WriteTimeout:   30,
		IdleTimeout:    60,
		LogLevel:       "info",
		LogFormat:      "text",
		
		TLS: TLSConfig{
			Enabled: false,
		},
		
		CORS: CORSConfig{
			Enabled:        true,
			AllowedOrigins: []string{"*"},
			AllowedMethods: []string{"GET", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
			MaxAge:         3600,
		},
		
		RateLimit: RateLimitConfig{
			Enabled:   true,
			Requests:  1000,
			Window:    time.Hour,
			BurstSize: 100,
		},
		
		Compression: CompressionConfig{
			Enabled: true,
			Level:   6,
			MinSize: 1024,
			Types:   []string{"application/json", "text/html", "text/plain", "text/css", "application/javascript"},
		},
		
		Cache: CacheConfig{
			Enabled:     true,
			TTL:         5 * time.Minute,
			MaxSize:     100,
			CleanupFreq: 10 * time.Minute,
		},
		
		Metrics: MetricsConfig{
			Enabled: true,
			Path:    "/metrics",
		},
		
		Auth: AuthConfig{
			Enabled:       true, // Enable authentication by default
			JWTSecret:     "change-this-secret-in-production-use-at-least-32-chars",
			JWTExpiration: "24h",
			APIKeys: map[string]string{
				"admin-key-example": "Default admin API key - change in production",
			},
			AdminUsers: []AdminUser{
				{
					Username:    "admin",
					Password:    "changeme", // Change in production!
					Role:        "admin",
					Permissions: []string{"*"},
				},
			},
		},
		
		Storage: StorageConfig{
			Type: "filesystem",
			Properties: map[string]interface{}{
				"base_path": "./tuf-repository",
			},
		},
		
		Tracing: TracingConfig{
			Enabled:      false,
			ServiceName:  "tuf-server",
			Endpoint:     "localhost:4318",
			SamplingRate: 0.1,
			Insecure:     true,
			Headers:      make(map[string]string),
		},
		
		Coalescing: CoalescingConfig{
			Enabled: true, // Enable request coalescing for better performance
			TTL:     5 * time.Second,
			MaxWait: 30 * time.Second,
		},
		
		CircuitBreaker: CircuitBreakerConfig{
			Enabled:     true, // Enable circuit breakers for resilience
			MaxRequests: 3,
			Interval:    60 * time.Second,
			Timeout:     30 * time.Second,
			Threshold:   0.5,
			MinRequests: 5,
		},
		
		Retry: RetryConfig{
			Enabled:        true, // Enable retry logic for better resilience
			MaxRetries:     3,
			InitialDelay:   100 * time.Millisecond,
			MaxDelay:       10 * time.Second,
			Multiplier:     2.0,
			JitterFraction: 0.1,
		},
	}
}

// LoadConfig loads configuration from file or environment variables
func LoadConfig(configFile string) (*Config, error) {
	config := DefaultConfig()
	
	logger.Logger.Debug("LoadConfig called", "configFile", configFile)
	
	// Load from file if provided
	if configFile != "" {
		if err := config.loadFromFile(configFile); err != nil {
			return nil, fmt.Errorf("failed to load config file: %w", err)
		}
	}
	
	// Override with environment variables
	config.loadFromEnv()
	
	// Validate configuration
	if err := config.validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}
	
	return config, nil
}

// loadFromFile loads configuration from JSON file
func (c *Config) loadFromFile(filename string) error {
	logger.Logger.Debug("Loading config from file", "filename", filename)
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	logger.Logger.Debug("Config file read", "size", len(data))
	
	// First unmarshal into a temporary structure with string durations
	var temp struct {
		Port           int    `json:"port"`
		Host           string `json:"host"`
		RepositoryPath string `json:"repository_path"`
		ReadTimeout    int    `json:"read_timeout"`
		WriteTimeout   int    `json:"write_timeout"`
		IdleTimeout    int    `json:"idle_timeout"`
		LogLevel       string `json:"log_level"`
		LogFormat      string `json:"log_format"`
		
		TLS  TLSConfig  `json:"tls"`
		CORS CORSConfig `json:"cors"`
		
		RateLimit struct {
			Enabled     bool     `json:"enabled"`
			Requests    int      `json:"requests"`
			Window      string   `json:"window"`      // String instead of time.Duration
			BurstSize   int      `json:"burst_size"`
			IPWhitelist []string `json:"ip_whitelist"`
			IPBlacklist []string `json:"ip_blacklist"`
		} `json:"rate_limit"`
		
		Compression CompressionConfig `json:"compression"`
		
		Cache struct {
			Enabled     bool   `json:"enabled"`
			TTL         string `json:"ttl"`          // String instead of time.Duration
			MaxSize     int    `json:"max_size"`
			CleanupFreq string `json:"cleanup_freq"` // String instead of time.Duration
		} `json:"cache"`
		
		Metrics MetricsConfig `json:"metrics"`
		Auth    AuthConfig    `json:"auth"`
		Storage StorageConfig `json:"storage"`
		
		Tracing struct {
			Enabled      bool              `json:"enabled"`
			ServiceName  string            `json:"service_name"`
			Endpoint     string            `json:"endpoint"`
			SamplingRate float64           `json:"sampling_rate"`
			Insecure     bool              `json:"insecure"`
			Headers      map[string]string `json:"headers"`
		} `json:"tracing"`
		
		Coalescing struct {
			Enabled bool   `json:"enabled"`
			TTL     string `json:"ttl"`
			MaxWait string `json:"max_wait"`
		} `json:"coalescing"`
		
		CircuitBreaker struct {
			Enabled     bool    `json:"enabled"`
			MaxRequests uint32  `json:"max_requests"`
			Interval    string  `json:"interval"`
			Timeout     string  `json:"timeout"`
			Threshold   float64 `json:"threshold"`
			MinRequests uint32  `json:"min_requests"`
		} `json:"circuit_breaker"`
		
		Retry struct {
			Enabled        bool    `json:"enabled"`
			MaxRetries     int     `json:"max_retries"`
			InitialDelay   string  `json:"initial_delay"`
			MaxDelay       string  `json:"max_delay"`
			Multiplier     float64 `json:"multiplier"`
			JitterFraction float64 `json:"jitter_fraction"`
		} `json:"retry"`
	}
	
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	
	logger.Logger.Info("JSON unmarshaled", 
		"auth_enabled", temp.Auth.Enabled,
		"coalescing_enabled", temp.Coalescing.Enabled,
		"circuit_breaker_enabled", temp.CircuitBreaker.Enabled)
	
	// Copy simple fields
	c.Port = temp.Port
	c.Host = temp.Host
	c.RepositoryPath = temp.RepositoryPath
	c.ReadTimeout = temp.ReadTimeout
	c.WriteTimeout = temp.WriteTimeout
	c.IdleTimeout = temp.IdleTimeout
	c.LogLevel = temp.LogLevel
	c.LogFormat = temp.LogFormat
	c.TLS = temp.TLS
	c.CORS = temp.CORS
	c.Compression = temp.Compression
	c.Metrics = temp.Metrics
	c.Auth = temp.Auth
	c.Storage = temp.Storage
	
	// Parse duration strings
	c.RateLimit.Enabled = temp.RateLimit.Enabled
	c.RateLimit.Requests = temp.RateLimit.Requests
	c.RateLimit.BurstSize = temp.RateLimit.BurstSize
	c.RateLimit.IPWhitelist = temp.RateLimit.IPWhitelist
	c.RateLimit.IPBlacklist = temp.RateLimit.IPBlacklist
	
	if temp.RateLimit.Window != "" {
		if d, err := time.ParseDuration(temp.RateLimit.Window); err == nil {
			c.RateLimit.Window = d
		} else {
			return fmt.Errorf("invalid rate limit window duration: %s", temp.RateLimit.Window)
		}
	}
	
	c.Cache.Enabled = temp.Cache.Enabled
	c.Cache.MaxSize = temp.Cache.MaxSize
	
	if temp.Cache.TTL != "" {
		if d, err := time.ParseDuration(temp.Cache.TTL); err == nil {
			c.Cache.TTL = d
		} else {
			return fmt.Errorf("invalid cache TTL duration: %s", temp.Cache.TTL)
		}
	}
	
	if temp.Cache.CleanupFreq != "" {
		if d, err := time.ParseDuration(temp.Cache.CleanupFreq); err == nil {
			c.Cache.CleanupFreq = d
		} else {
			return fmt.Errorf("invalid cache cleanup frequency duration: %s", temp.Cache.CleanupFreq)
		}
	}
	
	// Parse tracing configuration
	c.Tracing.Enabled = temp.Tracing.Enabled
	c.Tracing.ServiceName = temp.Tracing.ServiceName
	c.Tracing.Endpoint = temp.Tracing.Endpoint
	c.Tracing.SamplingRate = temp.Tracing.SamplingRate
	c.Tracing.Insecure = temp.Tracing.Insecure
	if temp.Tracing.Headers != nil {
		c.Tracing.Headers = temp.Tracing.Headers
	}
	
	// Parse coalescing configuration
	c.Coalescing.Enabled = temp.Coalescing.Enabled
	if temp.Coalescing.TTL != "" {
		if d, err := time.ParseDuration(temp.Coalescing.TTL); err == nil {
			c.Coalescing.TTL = d
		} else {
			return fmt.Errorf("invalid coalescing TTL duration: %s", temp.Coalescing.TTL)
		}
	}
	if temp.Coalescing.MaxWait != "" {
		if d, err := time.ParseDuration(temp.Coalescing.MaxWait); err == nil {
			c.Coalescing.MaxWait = d
		} else {
			return fmt.Errorf("invalid coalescing max wait duration: %s", temp.Coalescing.MaxWait)
		}
	}
	
	// Parse circuit breaker configuration
	c.CircuitBreaker.Enabled = temp.CircuitBreaker.Enabled
	c.CircuitBreaker.MaxRequests = temp.CircuitBreaker.MaxRequests
	c.CircuitBreaker.Threshold = temp.CircuitBreaker.Threshold
	c.CircuitBreaker.MinRequests = temp.CircuitBreaker.MinRequests
	
	// Debug logging
	logger.Logger.Debug("Config file parsed",
		"auth_enabled", c.Auth.Enabled,
		"coalescing_enabled", c.Coalescing.Enabled,
		"circuit_breaker_enabled", c.CircuitBreaker.Enabled)
	
	if temp.CircuitBreaker.Interval != "" {
		if d, err := time.ParseDuration(temp.CircuitBreaker.Interval); err == nil {
			c.CircuitBreaker.Interval = d
		} else {
			return fmt.Errorf("invalid circuit breaker interval duration: %s", temp.CircuitBreaker.Interval)
		}
	}
	if temp.CircuitBreaker.Timeout != "" {
		if d, err := time.ParseDuration(temp.CircuitBreaker.Timeout); err == nil {
			c.CircuitBreaker.Timeout = d
		} else {
			return fmt.Errorf("invalid circuit breaker timeout duration: %s", temp.CircuitBreaker.Timeout)
		}
	}
	
	// Parse retry configuration
	c.Retry.Enabled = temp.Retry.Enabled
	c.Retry.MaxRetries = temp.Retry.MaxRetries
	c.Retry.Multiplier = temp.Retry.Multiplier
	c.Retry.JitterFraction = temp.Retry.JitterFraction
	
	if temp.Retry.InitialDelay != "" {
		if d, err := time.ParseDuration(temp.Retry.InitialDelay); err == nil {
			c.Retry.InitialDelay = d
		} else {
			return fmt.Errorf("invalid retry initial delay duration: %s", temp.Retry.InitialDelay)
		}
	}
	if temp.Retry.MaxDelay != "" {
		if d, err := time.ParseDuration(temp.Retry.MaxDelay); err == nil {
			c.Retry.MaxDelay = d
		} else {
			return fmt.Errorf("invalid retry max delay duration: %s", temp.Retry.MaxDelay)
		}
	}
	
	return nil
}

// loadFromEnv loads configuration from environment variables
func (c *Config) loadFromEnv() {
	if val := os.Getenv("TUF_PORT"); val != "" {
		if port, err := strconv.Atoi(val); err == nil {
			c.Port = port
		}
	}
	
	if val := os.Getenv("TUF_HOST"); val != "" {
		c.Host = val
	}
	
	if val := os.Getenv("TUF_REPOSITORY_PATH"); val != "" {
		c.RepositoryPath = val
	}
	
	if val := os.Getenv("TUF_LOG_LEVEL"); val != "" {
		c.LogLevel = val
	}
	
	if val := os.Getenv("TUF_LOG_FORMAT"); val != "" {
		c.LogFormat = val
	}
	
	// TLS configuration
	if val := os.Getenv("TUF_TLS_ENABLED"); val == "true" {
		c.TLS.Enabled = true
	}
	
	if val := os.Getenv("TUF_TLS_CERT_FILE"); val != "" {
		c.TLS.CertFile = val
	}
	
	if val := os.Getenv("TUF_TLS_KEY_FILE"); val != "" {
		c.TLS.KeyFile = val
	}
	
	if val := os.Getenv("TUF_TLS_CA_CERT_FILE"); val != "" {
		c.TLS.CACertFile = val
	}
	
	// CORS configuration
	if val := os.Getenv("TUF_CORS_ENABLED"); val == "false" {
		c.CORS.Enabled = false
	}
	
	if val := os.Getenv("TUF_CORS_ORIGINS"); val != "" {
		c.CORS.AllowedOrigins = strings.Split(val, ",")
	}
	
	// Rate limiting
	if val := os.Getenv("TUF_RATE_LIMIT_ENABLED"); val == "false" {
		c.RateLimit.Enabled = false
	}
	
	if val := os.Getenv("TUF_RATE_LIMIT_REQUESTS"); val != "" {
		if requests, err := strconv.Atoi(val); err == nil {
			c.RateLimit.Requests = requests
		}
	}
	
	// Compression
	if val := os.Getenv("TUF_COMPRESSION_ENABLED"); val == "false" {
		c.Compression.Enabled = false
	}
	
	// Cache
	if val := os.Getenv("TUF_CACHE_ENABLED"); val == "false" {
		c.Cache.Enabled = false
	}
	
	if val := os.Getenv("TUF_CACHE_TTL"); val != "" {
		if ttl, err := time.ParseDuration(val); err == nil {
			c.Cache.TTL = ttl
		}
	}
	
	// Metrics
	if val := os.Getenv("TUF_METRICS_ENABLED"); val == "false" {
		c.Metrics.Enabled = false
	}
}

// validate checks if configuration is valid
func (c *Config) validate() error {
	if c.Port <= 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	
	if c.RepositoryPath == "" {
		return fmt.Errorf("repository path cannot be empty")
	}
	
	validLogLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if !validLogLevels[strings.ToLower(c.LogLevel)] {
		return fmt.Errorf("invalid log level: %s", c.LogLevel)
	}
	
	validLogFormats := map[string]bool{"text": true, "json": true}
	if !validLogFormats[strings.ToLower(c.LogFormat)] {
		return fmt.Errorf("invalid log format: %s", c.LogFormat)
	}
	
	if c.TLS.Enabled {
		if c.TLS.CertFile == "" || c.TLS.KeyFile == "" {
			return fmt.Errorf("TLS enabled but cert or key file not specified")
		}
	}
	
	if c.Compression.Level < 1 || c.Compression.Level > 9 {
		c.Compression.Level = 6 // Default to reasonable compression level
	}
	
	return nil
}

// Save saves configuration to file
func (c *Config) Save(filename string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile(filename, data, 0644)
}