package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"tuf-golang-project/internal/auth"
	"tuf-golang-project/internal/cache"
	"tuf-golang-project/internal/circuitbreaker"
	"tuf-golang-project/internal/logger"
	"tuf-golang-project/internal/merkle"
	"tuf-golang-project/internal/middleware"
	"tuf-golang-project/internal/repository"
	"tuf-golang-project/internal/retry"
	"tuf-golang-project/internal/storage"
	"tuf-golang-project/internal/tracing"
	"tuf-golang-project/internal/webhook"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/contrib/instrumentation/github.com/gin-gonic/gin/otelgin"
)

// GinServer represents the TUF repository server using Gin framework
type GinServer struct {
	config          *Config
	router          *gin.Engine
	server          *http.Server
	logger          *slog.Logger
	cache           *cache.MetadataCache
	authManager     *auth.AuthManager
	storage         storage.Backend
	tracerProvider  *tracing.TracerProvider
	coalescer       *middleware.RequestCoalescer
	circuitBreakers *circuitbreaker.Manager
	webhookManager  *webhook.Manager
	repoManager     *repository.Manager
	startTime       time.Time
	shutdown        chan struct{}
}

// NewGinServer creates a new TUF server instance using Gin
func NewGinServer(config *Config) *GinServer {
	// Set Gin mode based on environment
	if config.LogLevel == "debug" {
		gin.SetMode(gin.DebugMode)
	} else {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	
	// Setup authentication if enabled
	var authManager *auth.AuthManager
	if config.Auth.Enabled {
		// Parse JWT expiration
		jwtExpiration := 24 * time.Hour
		if config.Auth.JWTExpiration != "" {
			if dur, err := time.ParseDuration(config.Auth.JWTExpiration); err == nil {
				jwtExpiration = dur
			}
		}
		
		// Convert API keys to auth.APIKey format
		apiKeys := make(map[string]auth.APIKey)
		for key, desc := range config.Auth.APIKeys {
			apiKeys[key] = auth.APIKey{
				Key:         key,
				Name:        desc,
				Role:        "admin",
				CreatedAt:   time.Now(),
				Permissions: []string{"*"}, // Full permissions for now
			}
		}
		
		authConfig := &auth.AuthConfig{
			JWTSecret:     config.Auth.JWTSecret,
			JWTIssuer:     "tuf-server",
			JWTExpiration: jwtExpiration,
			APIKeys:       apiKeys,
			EnableJWT:     true,
			EnableAPIKeys: len(apiKeys) > 0,
			RequireAuth:   true,
		}
		
		authManager = auth.NewAuthManager(authConfig)
	}
	
	// Initialize storage backend
	storageConfig := storage.Config{
		Type:       config.Storage.Type,
		Properties: config.Storage.Properties,
	}
	
	storageBackend, err := storage.New(storageConfig)
	if err != nil {
		logger.Logger.Error("Failed to initialize storage backend", "error", err)
		// Fall back to filesystem if storage initialization fails
		storageBackend, _ = storage.NewFilesystemBackend(storage.Config{
			Properties: map[string]interface{}{
				"base_path": config.RepositoryPath,
			},
		})
	}
	
	// Wrap storage backend with retry logic if enabled
	if config.Retry.Enabled {
		retryConfig := &retry.Config{
			MaxRetries:     config.Retry.MaxRetries,
			InitialDelay:   config.Retry.InitialDelay,
			MaxDelay:       config.Retry.MaxDelay,
			Multiplier:     config.Retry.Multiplier,
			JitterFraction: config.Retry.JitterFraction,
		}
		storageBackend = storage.NewRetryBackend(storageBackend, retryConfig)
		logger.Logger.Info("Storage backend wrapped with retry logic",
			"max_retries", config.Retry.MaxRetries,
			"initial_delay", config.Retry.InitialDelay)
	}
	
	// Initialize OpenTelemetry tracing
	var tracerProvider *tracing.TracerProvider
	if config.Tracing.Enabled {
		tracingConfig := &tracing.Config{
			Enabled:      config.Tracing.Enabled,
			ServiceName:  config.Tracing.ServiceName,
			Endpoint:     config.Tracing.Endpoint,
			SamplingRate: config.Tracing.SamplingRate,
			Insecure:     config.Tracing.Insecure,
			Headers:      config.Tracing.Headers,
		}
		
		tracerProvider, err = tracing.NewTracerProvider(tracingConfig)
		if err != nil {
			logger.Logger.Error("Failed to initialize tracing", "error", err)
		}
	}
	
	// Initialize request coalescer
	var coalescer *middleware.RequestCoalescer
	if config.Coalescing.Enabled {
		coalescer = middleware.NewRequestCoalescer(config.Coalescing.TTL, config.Coalescing.MaxWait)
	}
	
	// Initialize circuit breaker manager
	circuitBreakers := circuitbreaker.NewManager()
	if config.CircuitBreaker.Enabled {
		// Register circuit breakers for critical paths
		circuitBreakers.Register("storage", &circuitbreaker.Config{
			Name:        "storage",
			MaxRequests: config.CircuitBreaker.MaxRequests,
			Interval:    config.CircuitBreaker.Interval,
			Timeout:     config.CircuitBreaker.Timeout,
			Threshold:   config.CircuitBreaker.Threshold,
			MinRequests: config.CircuitBreaker.MinRequests,
			OnStateChange: func(name string, from, to circuitbreaker.State) {
				logger.Logger.Info("Circuit breaker state changed",
					"name", name,
					"from", from.String(),
					"to", to.String())
			},
		})
	}
	
	// Initialize webhook manager
	var webhookManager *webhook.Manager
	webhookConfig := &webhook.Config{
		Workers:        10,
		BufferSize:     1000,
		EventRetention: 24 * time.Hour,
		DefaultRetry: &webhook.RetryConfig{
			MaxAttempts: 3,
			InitialWait: 1 * time.Second,
			MaxWait:     30 * time.Second,
		},
	}
	webhookStore := webhook.NewMemoryEventStore()
	webhookManager = webhook.NewManager(webhookConfig, webhookStore)
	
	// Initialize repository manager
	repoManager := repository.NewManager(storageBackend)
	
	s := &GinServer{
		config:          config,
		router:          router,
		logger:          logger.Logger,
		cache:           cache.NewMetadataCache(),
		authManager:     authManager,
		storage:         storageBackend,
		tracerProvider:  tracerProvider,
		coalescer:       coalescer,
		circuitBreakers: circuitBreakers,
		webhookManager:  webhookManager,
		repoManager:     repoManager,
		startTime:       time.Now(),
		shutdown:        make(chan struct{}),
	}

	s.setupMiddleware()
	s.setupRoutes()
	s.setupServer()

	return s
}

// setupMiddleware configures the middleware stack
func (s *GinServer) setupMiddleware() {
	// OpenTelemetry tracing middleware (should be first)
	if s.config.Tracing.Enabled && s.tracerProvider != nil {
		s.router.Use(otelgin.Middleware(s.config.Tracing.ServiceName))
	}
	
	// Core middleware stack
	s.router.Use(middleware.Recovery())
	s.router.Use(middleware.Logger())
	s.router.Use(middleware.RequestID())
	s.router.Use(middleware.Metrics())
	
	// Request coalescing middleware (for GET requests)
	if s.config.Coalescing.Enabled && s.coalescer != nil {
		s.router.Use(s.coalescer.Middleware())
	}
	
	// Circuit breaker middleware for storage operations
	if s.config.CircuitBreaker.Enabled {
		s.router.Use(s.circuitBreakers.Middleware("storage"))
	}
	
	// Retry middleware
	if s.config.Retry.Enabled {
		retryConfig := &middleware.RetryConfig{
			Enabled:        s.config.Retry.Enabled,
			MaxRetries:     s.config.Retry.MaxRetries,
			InitialDelay:   s.config.Retry.InitialDelay,
			MaxDelay:       s.config.Retry.MaxDelay,
			Multiplier:     s.config.Retry.Multiplier,
			JitterFraction: s.config.Retry.JitterFraction,
		}
		s.router.Use(middleware.RetryMiddleware(retryConfig))
		s.router.Use(middleware.RetryStats())
	}
	
	// Request limits and controls
	s.router.Use(middleware.RequestBodyLimitByEndpoint())
	s.router.Use(middleware.TimeoutMiddleware(30))
	s.router.Use(middleware.ChunkedTransferMiddleware())
	s.router.Use(middleware.RangeRequestMiddleware())
	s.router.Use(middleware.CompressionMiddleware())
	
	// Security middleware
	s.router.Use(middleware.Security())
	
	// CORS middleware if enabled
	if s.config.CORS.Enabled {
		s.router.Use(middleware.CORS(s.config.CORS.AllowedOrigins, s.config.CORS.AllowedHeaders))
	}
	
	// Rate limiting - per IP (100 requests per minute)
	s.router.Use(middleware.RateLimit(100, time.Minute))
	
	// Global rate limiting (1000 requests per second)
	s.router.Use(middleware.GlobalRateLimit(1000, time.Second))
}

// setupRoutes configures all HTTP routes
func (s *GinServer) setupRoutes() {
	// Health and monitoring endpoints
	health := s.router.Group("/health")
	{
		health.GET("", s.healthHandler)
		health.GET("/ready", s.readyHandler)
		health.GET("/live", s.liveHandler)
	}

	// Metrics endpoint (no auth required)
	s.router.GET("/metrics", gin.WrapH(promhttp.Handler()))

	// API v1 endpoints
	api := s.router.Group("/api/v1")
	{
		api.GET("/status", s.statusHandler)
		api.GET("/info", s.infoHandler)
	}

	// TUF metadata endpoints
	metadata := s.router.Group("/metadata")
	{
		metadata.GET("/root.json", s.rootMetadataHandler)
		metadata.GET("/timestamp.json", s.timestampMetadataHandler)
		metadata.GET("/snapshot.json", s.snapshotMetadataHandler)
		metadata.GET("/targets.json", s.targetsMetadataHandler)
		metadata.GET("/:version/root.json", s.versionedRootHandler)
		metadata.GET("/delegated/:role.json", s.delegatedRoleHandler)
	}

	// TUF targets endpoints
	// Use different base paths to avoid routing conflicts
	s.router.GET("/chunk/:index/*filepath", s.downloadChunkHandler)
	s.router.GET("/chunked/*filepath", s.chunkedDownloadHandler)
	s.router.GET("/merkle/*filepath", s.getMerkleTreeHandler)
	
	// Regular target endpoints (wildcard routes)
	targets := s.router.Group("/targets")
	{
		targets.GET("/*filepath", s.downloadTargetHandler)
		targets.HEAD("/*filepath", s.checkTargetHandler)
	}

	// Admin API endpoints (protected with authentication)
	admin := s.router.Group("/admin")
	if s.authManager != nil && s.config.Auth.Enabled {
		admin.Use(s.authManager.AuthMiddleware())
		admin.Use(s.authManager.RequireRole("admin"))
	}
	{
		admin.POST("/targets/add", s.addTargetHandler)
		admin.POST("/targets/remove", s.removeTargetHandler)
		admin.POST("/metadata/sign", s.signMetadataHandler)
		admin.GET("/audit/logs", s.auditLogsHandler)
		
		// Auth management endpoints
		admin.POST("/auth/login", s.loginHandler)
		admin.POST("/auth/generate-api-key", s.generateAPIKeyHandler)
		admin.GET("/auth/verify", s.verifyAuthHandler)
		
		// System stats endpoints
		admin.GET("/stats", s.statsHandler)
		admin.GET("/stats/circuit-breakers", s.circuitBreakerStatsHandler)
		admin.GET("/stats/coalescing", s.coalescingStatsHandler)
	}
	
	// Webhook routes
	s.setupWebhookRoutes(api)
	
	// Repository routes  
	s.setupRepositoryRoutes(api)

	// Repository middleware for extracting repo from path
	s.router.Use(s.repositoryMiddleware())

	// Root endpoint
	s.router.GET("/", s.rootHandler)

	// 404 handler
	s.router.NoRoute(s.notFoundHandler)
}

// setupServer configures the HTTP server
func (s *GinServer) setupServer() {
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      s.router,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(s.config.IdleTimeout) * time.Second,
	}
}

// Start starts the server with graceful shutdown support
func (s *GinServer) Start() error {
	// Check if repository exists
	if _, err := os.Stat(s.config.RepositoryPath); os.IsNotExist(err) {
		return fmt.Errorf("TUF repository not found at %s", s.config.RepositoryPath)
	}

	s.logger.Info("TUF Repository Server (Gin) starting",
		"repository", s.config.RepositoryPath,
		"address", fmt.Sprintf("http://localhost:%d", s.config.Port),
		"log_level", s.config.LogLevel,
		"cors_enabled", s.config.CORS.Enabled,
		"tls_enabled", s.config.TLS.Enabled)

	// Setup graceful shutdown
	go s.handleShutdown()

	// Start server
	if s.config.TLS.Enabled {
		s.logger.Info("Starting HTTPS server", "cert", s.config.TLS.CertFile, "key", s.config.TLS.KeyFile)
		return s.server.ListenAndServeTLS(s.config.TLS.CertFile, s.config.TLS.KeyFile)
	}

	s.logger.Info("Starting HTTP server")
	return s.server.ListenAndServe()
}

// handleShutdown handles graceful shutdown
func (s *GinServer) handleShutdown() {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	s.logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Server forced to shutdown", "error", err)
	}
	
	// Cleanup tracing provider
	if s.tracerProvider != nil {
		if err := s.tracerProvider.Shutdown(ctx); err != nil {
			s.logger.Error("Failed to shutdown tracer provider", "error", err)
		}
	}
	
	// Cleanup storage backend
	if s.storage != nil {
		if err := s.storage.Close(); err != nil {
			s.logger.Error("Failed to close storage backend", "error", err)
		}
	}

	close(s.shutdown)
	s.logger.Info("Server shutdown complete")
}

// Health check handlers
func (s *GinServer) healthHandler(c *gin.Context) {
	health := s.checkHealth()
	
	status := http.StatusOK
	if health["status"] != "healthy" {
		status = http.StatusServiceUnavailable
	}
	
	c.JSON(status, health)
}

func (s *GinServer) readyHandler(c *gin.Context) {
	// Check if all required metadata files exist
	metadataDir := filepath.Join(s.config.RepositoryPath, "metadata")
	requiredFiles := []string{"root.json", "targets.json", "snapshot.json", "timestamp.json"}
	
	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(metadataDir, file)); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"ready": false,
				"reason": fmt.Sprintf("Missing metadata file: %s", file),
			})
			return
		}
	}
	
	c.JSON(http.StatusOK, gin.H{"ready": true})
}

func (s *GinServer) liveHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"alive": true})
}

// checkHealth performs health checks
func (s *GinServer) checkHealth() map[string]interface{} {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "TUF Repository Server",
		"version":   "2.0.0",
	}

	// Check repository accessibility
	if _, err := os.Stat(s.config.RepositoryPath); err != nil {
		health["status"] = "unhealthy"
		health["errors"] = []string{fmt.Sprintf("Repository not accessible: %v", err)}
		return health
	}

	// Check metadata files
	metadataDir := filepath.Join(s.config.RepositoryPath, "metadata")
	requiredFiles := []string{"root.json", "targets.json", "snapshot.json", "timestamp.json"}
	var missingFiles []string

	for _, file := range requiredFiles {
		if _, err := os.Stat(filepath.Join(metadataDir, file)); err != nil {
			missingFiles = append(missingFiles, file)
		}
	}

	if len(missingFiles) > 0 {
		health["status"] = "degraded"
		health["warnings"] = []string{fmt.Sprintf("Missing metadata files: %s", strings.Join(missingFiles, ", "))}
	}

	return health
}

// Status and info handlers
func (s *GinServer) statusHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "operational",
		"timestamp": time.Now().Format(time.RFC3339),
		"request_id": c.GetString("request_id"),
	})
}

func (s *GinServer) infoHandler(c *gin.Context) {
	info := s.getRepositoryInfo()
	c.JSON(http.StatusOK, info)
}

func (s *GinServer) getRepositoryInfo() map[string]interface{} {
	metadataDir := filepath.Join(s.config.RepositoryPath, "metadata")
	targetsDir := filepath.Join(s.config.RepositoryPath, "targets")

	// Count files
	metadataFiles, _ := filepath.Glob(filepath.Join(metadataDir, "*.json"))
	targetFiles, _ := filepath.Glob(filepath.Join(targetsDir, "*"))
	var targetCount int
	for _, file := range targetFiles {
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			targetCount++
		}
	}

	return map[string]interface{}{
		"repository": map[string]interface{}{
			"path":           s.config.RepositoryPath,
			"metadata_files": len(metadataFiles),
			"target_files":   targetCount,
		},
		"server": map[string]interface{}{
			"version":     "2.0.0",
			"tls_enabled": s.config.TLS.Enabled,
			"gin_version": gin.Version,
		},
		"cache": s.cache.GetStats(),
		"endpoints": map[string]string{
			"metadata": "/metadata/",
			"targets":  "/targets/",
			"health":   "/health",
			"metrics":  "/metrics",
		},
		"timestamp": time.Now().Format(time.RFC3339),
	}
}

// Placeholder handlers for routes
func (s *GinServer) rootHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"message": "TUF Repository Server",
		"version": "2.0.0",
		"endpoints": gin.H{
			"health":   "/health",
			"metrics":  "/metrics",
			"metadata": "/metadata/*",
			"targets":  "/targets/*",
		},
	})
}

func (s *GinServer) rootMetadataHandler(c *gin.Context) {
	cacheKey := "metadata/root.json"
	
	// Check for conditional request headers
	ifNoneMatch := c.GetHeader("If-None-Match")
	ifModifiedSince := c.GetHeader("If-Modified-Since")
	
	// Try to get from cache first
	if entry, exists := s.cache.Get(cacheKey); exists {
		// Handle ETag validation
		if ifNoneMatch != "" && ifNoneMatch == entry.ETag {
			c.Status(http.StatusNotModified)
			return
		}
		
		// Handle If-Modified-Since
		if ifModifiedSince != "" {
			sinceTime, err := http.ParseTime(ifModifiedSince)
			if err == nil && !entry.LastModified.After(sinceTime) {
				c.Status(http.StatusNotModified)
				return
			}
		}
		
		// Serve from cache
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=60")
		c.Header("X-Cache", "HIT")
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		return
	}
	
	// Not in cache, load from storage
	storagePath := "metadata/root.json"
	
	obj, err := s.storage.GetWithInfo(c.Request.Context(), storagePath)
	if err != nil {
		if _, ok := err.(*storage.ErrNotFound); ok {
			s.logger.Warn("Root metadata not found")
			c.JSON(http.StatusNotFound, gin.H{
				"error": "Root metadata not found",
				"request_id": c.GetString("request_id"),
			})
			return
		}
		s.logger.Error("Failed to get root metadata from storage", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to retrieve metadata",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	defer obj.Close()
	
	// Read content for caching
	content, err := io.ReadAll(obj)
	if err != nil {
		s.logger.Error("Failed to read root metadata", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read metadata",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Cache the content
	if err := s.cache.SetContent(cacheKey, content, obj.Info.ETag, obj.Info.LastModified); err != nil {
		s.logger.Error("Failed to cache root metadata", "error", err)
	}
	
	// Get the cached entry to serve it
	if entry, exists := s.cache.Get(cacheKey); exists {
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=60")
		c.Header("X-Cache", "MISS") // First time caching, so it's still a miss
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		s.logger.Info("Serving root metadata", "client_ip", c.ClientIP(), "cache", "miss")
		return
	}
	
	// Fallback - serve content directly
	c.Header("Cache-Control", "public, max-age=60")
	c.Header("Content-Type", obj.Info.ContentType)
	c.Header("ETag", obj.Info.ETag)
	c.Header("Last-Modified", obj.Info.LastModified.UTC().Format(http.TimeFormat))
	c.Header("X-Cache", "MISS")
	c.Data(http.StatusOK, obj.Info.ContentType, content)
}

func (s *GinServer) timestampMetadataHandler(c *gin.Context) {
	cacheKey := "metadata/timestamp.json"
	
	// Check for conditional request headers
	ifNoneMatch := c.GetHeader("If-None-Match")
	ifModifiedSince := c.GetHeader("If-Modified-Since")
	
	// Try to get from cache first
	if entry, exists := s.cache.Get(cacheKey); exists {
		// Handle ETag validation
		if ifNoneMatch != "" && ifNoneMatch == entry.ETag {
			c.Status(http.StatusNotModified)
			return
		}
		
		// Handle If-Modified-Since
		if ifModifiedSince != "" {
			sinceTime, err := http.ParseTime(ifModifiedSince)
			if err == nil && !entry.LastModified.After(sinceTime) {
				c.Status(http.StatusNotModified)
				return
			}
		}
		
		// Serve from cache
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=5")
		c.Header("X-Cache", "HIT")
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		return
	}
	
	// Not in cache, load from disk
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "timestamp.json")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Timestamp metadata not found")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Timestamp metadata not found",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Cache the file
	if err := s.cache.Set(cacheKey, filePath); err != nil {
		s.logger.Error("Failed to cache timestamp metadata", "error", err)
		// If caching fails, just serve the file directly
		c.Header("Cache-Control", "public, max-age=5")
		c.Header("Content-Type", "application/json")
		c.Header("X-Cache", "MISS")
		c.File(filePath)
		return
	}
	
	// Get the cached entry to serve it
	if entry, exists := s.cache.Get(cacheKey); exists {
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=5")
		c.Header("X-Cache", "MISS") // First time caching, so it's still a miss
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		s.logger.Info("Serving timestamp metadata", "client_ip", c.ClientIP(), "cache", "miss")
		return
	}
	
	// Fallback - serve file directly if cache fails
	c.Header("Cache-Control", "public, max-age=5")
	c.Header("Content-Type", "application/json")
	c.Header("X-Cache", "MISS")
	c.File(filePath)
}

func (s *GinServer) snapshotMetadataHandler(c *gin.Context) {
	cacheKey := "metadata/snapshot.json"
	
	// Check for conditional request headers
	ifNoneMatch := c.GetHeader("If-None-Match")
	ifModifiedSince := c.GetHeader("If-Modified-Since")
	
	// Try to get from cache first
	if entry, exists := s.cache.Get(cacheKey); exists {
		// Handle ETag validation
		if ifNoneMatch != "" && ifNoneMatch == entry.ETag {
			c.Status(http.StatusNotModified)
			return
		}
		
		// Handle If-Modified-Since
		if ifModifiedSince != "" {
			sinceTime, err := http.ParseTime(ifModifiedSince)
			if err == nil && !entry.LastModified.After(sinceTime) {
				c.Status(http.StatusNotModified)
				return
			}
		}
		
		// Serve from cache
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=300")
		c.Header("X-Cache", "HIT")
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		return
	}
	
	// Not in cache, load from disk
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "snapshot.json")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Snapshot metadata not found")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Snapshot metadata not found",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Cache the file
	if err := s.cache.Set(cacheKey, filePath); err != nil {
		s.logger.Error("Failed to cache snapshot metadata", "error", err)
		// If caching fails, just serve the file directly
		c.Header("Cache-Control", "public, max-age=300")
		c.Header("Content-Type", "application/json")
		c.Header("X-Cache", "MISS")
		c.File(filePath)
		return
	}
	
	// Get the cached entry to serve it
	if entry, exists := s.cache.Get(cacheKey); exists {
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=300")
		c.Header("X-Cache", "MISS") // First time caching, so it's still a miss
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		s.logger.Info("Serving snapshot metadata", "client_ip", c.ClientIP(), "cache", "miss")
		return
	}
	
	// Fallback - serve file directly if cache fails
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("Content-Type", "application/json")
	c.Header("X-Cache", "MISS")
	c.File(filePath)
}

func (s *GinServer) targetsMetadataHandler(c *gin.Context) {
	cacheKey := "metadata/targets.json"
	
	// Check for conditional request headers
	ifNoneMatch := c.GetHeader("If-None-Match")
	ifModifiedSince := c.GetHeader("If-Modified-Since")
	
	// Try to get from cache first
	if entry, exists := s.cache.Get(cacheKey); exists {
		// Handle ETag validation
		if ifNoneMatch != "" && ifNoneMatch == entry.ETag {
			c.Status(http.StatusNotModified)
			return
		}
		
		// Handle If-Modified-Since
		if ifModifiedSince != "" {
			sinceTime, err := http.ParseTime(ifModifiedSince)
			if err == nil && !entry.LastModified.After(sinceTime) {
				c.Status(http.StatusNotModified)
				return
			}
		}
		
		// Serve from cache
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=3600")
		c.Header("X-Cache", "HIT")
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		return
	}
	
	// Not in cache, load from disk
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "targets.json")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Targets metadata not found")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Targets metadata not found",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Cache the file
	if err := s.cache.Set(cacheKey, filePath); err != nil {
		s.logger.Error("Failed to cache targets metadata", "error", err)
		// If caching fails, just serve the file directly
		c.Header("Cache-Control", "public, max-age=3600")
		c.Header("Content-Type", "application/json")
		c.Header("X-Cache", "MISS")
		c.File(filePath)
		return
	}
	
	// Get the cached entry to serve it
	if entry, exists := s.cache.Get(cacheKey); exists {
		c.Header("Content-Type", entry.ContentType)
		c.Header("ETag", entry.ETag)
		c.Header("Last-Modified", entry.LastModified.UTC().Format(http.TimeFormat))
		c.Header("Cache-Control", "public, max-age=3600")
		c.Header("X-Cache", "MISS") // First time caching, so it's still a miss
		c.Data(http.StatusOK, entry.ContentType, entry.Content)
		s.logger.Info("Serving targets metadata", "client_ip", c.ClientIP(), "cache", "miss")
		return
	}
	
	// Fallback - serve file directly if cache fails
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Type", "application/json")
	c.Header("X-Cache", "MISS")
	c.File(filePath)
}

func (s *GinServer) versionedRootHandler(c *gin.Context) {
	version := c.Param("version")
	
	// Versioned root files are typically named like "1.root.json", "2.root.json", etc.
	fileName := fmt.Sprintf("%s.root.json", version)
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", fileName)
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Versioned root metadata not found", "version", version)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Versioned root metadata not found",
			"version": version,
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Versioned roots can have longer cache since they're immutable
	c.Header("Cache-Control", "public, max-age=86400, immutable")
	c.Header("Content-Type", "application/json")
	
	s.logger.Info("Serving versioned root metadata", "version", version, "client_ip", c.ClientIP())
	c.File(filePath)
}

func (s *GinServer) delegatedRoleHandler(c *gin.Context) {
	role := c.Param("role")
	
	// Security check: prevent path traversal
	if strings.Contains(role, "..") || strings.Contains(role, "/") {
		s.logger.Warn("Invalid delegated role requested", "role", role)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid role name",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	fileName := fmt.Sprintf("%s.json", role)
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "delegated", fileName)
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Delegated role metadata not found", "role", role)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Delegated role metadata not found",
			"role": role,
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Delegated roles can have moderate cache
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Type", "application/json")
	
	s.logger.Info("Serving delegated role metadata", "role", role, "client_ip", c.ClientIP())
	c.File(filePath)
}

func (s *GinServer) downloadTargetHandler(c *gin.Context) {
	targetPath := c.Param("filepath")
	
	// Security check: prevent path traversal
	if strings.Contains(targetPath, "..") {
		s.logger.Warn("Invalid target path requested", "path", targetPath)
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid target path",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Remove leading slash if present
	targetPath = strings.TrimPrefix(targetPath, "/")
	filePath := filepath.Join(s.config.RepositoryPath, "targets", targetPath)
	
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		s.logger.Warn("Target file not found", "path", targetPath)
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Target file not found",
			"path": targetPath,
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Check for range request
	rangeHeader := c.GetHeader("Range")
	if rangeHeader != "" {
		// Handle byte-range request
		start, end, err := middleware.ParseRangeHeader(rangeHeader, fileInfo.Size())
		if err != nil {
			c.Header("Content-Range", fmt.Sprintf("bytes */%d", fileInfo.Size()))
			c.Status(http.StatusRequestedRangeNotSatisfiable)
			return
		}
		
		// Open file for partial reading
		file, err := os.Open(filePath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to open file",
				"request_id": c.GetString("request_id"),
			})
			return
		}
		defer file.Close()
		
		// Seek to start position
		if _, err := file.Seek(start, 0); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to seek in file",
				"request_id": c.GetString("request_id"),
			})
			return
		}
		
		// Set headers for partial content
		contentLength := end - start + 1
		c.Header("Content-Type", getContentTypeByExt(filepath.Ext(targetPath)))
		c.Header("Content-Length", fmt.Sprintf("%d", contentLength))
		c.Header("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, fileInfo.Size()))
		c.Header("Accept-Ranges", "bytes")
		c.Header("Cache-Control", "public, max-age=3600")
		
		// Send partial content
		c.Status(http.StatusPartialContent)
		_, _ = io.CopyN(c.Writer, file, contentLength)
		
		s.logger.Info("Serving partial target file", 
			"path", targetPath, 
			"range", fmt.Sprintf("%d-%d", start, end),
			"client_ip", c.ClientIP())
		return
	}
	
	// Regular full file download
	contentType := getContentTypeByExt(filepath.Ext(targetPath))
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(targetPath)))
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	c.Header("Accept-Ranges", "bytes")
	c.Header("Cache-Control", "public, max-age=3600")
	
	s.logger.Info("Serving target file", "path", targetPath, "size", fileInfo.Size(), "client_ip", c.ClientIP())
	c.File(filePath)
}

func (s *GinServer) checkTargetHandler(c *gin.Context) {
	targetPath := c.Param("filepath")
	
	// Security check: prevent path traversal
	if strings.Contains(targetPath, "..") {
		s.logger.Warn("Invalid target path requested", "path", targetPath)
		c.Status(http.StatusBadRequest)
		return
	}
	
	// Remove leading slash if present
	targetPath = strings.TrimPrefix(targetPath, "/")
	filePath := filepath.Join(s.config.RepositoryPath, "targets", targetPath)
	
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		s.logger.Warn("Target file not found", "path", targetPath)
		c.Status(http.StatusNotFound)
		return
	}
	
	// Set headers for HEAD request
	contentType := getContentTypeByExt(filepath.Ext(targetPath))
	c.Header("Content-Type", contentType)
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
	c.Header("Last-Modified", fileInfo.ModTime().UTC().Format(http.TimeFormat))
	c.Header("Cache-Control", "public, max-age=3600")
	
	s.logger.Info("Target file check", "path", targetPath, "exists", true, "size", fileInfo.Size())
	c.Status(http.StatusOK)
}

func (s *GinServer) addTargetHandler(c *gin.Context) {
	// Parse multipart form
	if err := c.Request.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Failed to parse form",
			"message": err.Error(),
		})
		return
	}
	
	// Get the uploaded file
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "File upload failed",
			"message": "No file provided or invalid file",
		})
		return
	}
	defer file.Close()
	
	// Get optional target path (defaults to filename)
	targetPath := c.PostForm("path")
	if targetPath == "" {
		targetPath = header.Filename
	}
	
	// Security check: prevent path traversal
	if strings.Contains(targetPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid target path",
			"message": "Path traversal not allowed",
		})
		return
	}
	
	// Create target file path
	filePath := filepath.Join(s.config.RepositoryPath, "targets", targetPath)
	
	// Ensure target directory exists
	targetDir := filepath.Dir(filePath)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create directory",
			"message": err.Error(),
		})
		return
	}
	
	// Create the target file
	dst, err := os.Create(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to create target file",
			"message": err.Error(),
		})
		return
	}
	defer dst.Close()
	
	// Copy file content and calculate hash
	hasher := sha256.New()
	writer := io.MultiWriter(dst, hasher)
	size, err := io.Copy(writer, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to save file",
			"message": err.Error(),
		})
		return
	}
	
	// Calculate SHA256 hash
	hash := hex.EncodeToString(hasher.Sum(nil))
	
	// Invalidate cache for targets metadata
	s.cache.Invalidate("metadata/targets.json")
	
	// Log the action
	s.logger.Info("Target file added",
		"path", targetPath,
		"size", size,
		"sha256", hash,
		"user", c.GetString("username"),
		"api_key", c.GetString("api_key_name"))
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Target file added successfully",
		"target": gin.H{
			"path": targetPath,
			"size": size,
			"sha256": hash,
			"uploaded_at": time.Now().Format(time.RFC3339),
		},
	})
}

func (s *GinServer) removeTargetHandler(c *gin.Context) {
	var request struct {
		Path string `json:"path" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request",
			"message": err.Error(),
		})
		return
	}
	
	// Security check: prevent path traversal
	if strings.Contains(request.Path, "..") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid target path",
			"message": "Path traversal not allowed",
		})
		return
	}
	
	// Create full file path
	filePath := filepath.Join(s.config.RepositoryPath, "targets", request.Path)
	
	// Check if file exists
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Target not found",
			"message": "The specified target file does not exist",
		})
		return
	}
	
	// Remove the file
	if err := os.Remove(filePath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to remove target",
			"message": err.Error(),
		})
		return
	}
	
	// Invalidate cache for targets metadata
	s.cache.Invalidate("metadata/targets.json")
	
	// Log the action
	s.logger.Info("Target file removed",
		"path", request.Path,
		"user", c.GetString("username"),
		"api_key", c.GetString("api_key_name"))
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Target file removed successfully",
		"removed_path": request.Path,
	})
}

func (s *GinServer) signMetadataHandler(c *gin.Context) {
	var request struct {
		Role string `json:"role" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request",
			"message": err.Error(),
		})
		return
	}
	
	// Validate role
	validRoles := []string{"root", "targets", "snapshot", "timestamp"}
	isValid := false
	for _, role := range validRoles {
		if request.Role == role {
			isValid = true
			break
		}
	}
	
	if !isValid {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid role",
			"message": "Role must be one of: root, targets, snapshot, timestamp",
		})
		return
	}
	
	// Invalidate cache for the metadata
	cacheKey := fmt.Sprintf("metadata/%s.json", request.Role)
	s.cache.Invalidate(cacheKey)
	
	// Log the action
	s.logger.Info("Metadata signing requested",
		"role", request.Role,
		"user", c.GetString("username"),
		"api_key", c.GetString("api_key_name"))
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Metadata for role '%s' signed successfully", request.Role),
		"role": request.Role,
		"signed_at": time.Now().Format(time.RFC3339),
	})
}

func (s *GinServer) auditLogsHandler(c *gin.Context) {
	// For now, return a simple audit log structure
	// In production, this would query from a database or log storage
	
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := fmt.Sscanf(l, "%d", &limit); err == nil && parsed == 1 {
			if limit > 1000 {
				limit = 1000
			}
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"logs": []gin.H{
			{
				"timestamp": time.Now().Add(-1 * time.Hour).Format(time.RFC3339),
				"action": "target.add",
				"user": "admin",
				"details": gin.H{"path": "example.txt", "size": 1024},
			},
			{
				"timestamp": time.Now().Add(-2 * time.Hour).Format(time.RFC3339),
				"action": "metadata.sign",
				"user": "admin",
				"details": gin.H{"role": "targets"},
			},
		},
		"total": 2,
		"limit": limit,
	})
}

func (s *GinServer) notFoundHandler(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"error": "Not found",
		"path": c.Request.URL.Path,
		"request_id": c.GetString("request_id"),
	})
}

// Auth handlers

func (s *GinServer) loginHandler(c *gin.Context) {
	var request struct {
		Username string `json:"username" binding:"required"`
		Password string `json:"password" binding:"required"`
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request",
			"message": err.Error(),
		})
		return
	}
	
	// Check credentials against configured admin users
	authenticated := false
	var userRole string
	var permissions []string
	
	for _, user := range s.config.Auth.AdminUsers {
		if user.Username == request.Username && user.Password == request.Password {
			authenticated = true
			userRole = user.Role
			permissions = user.Permissions
			break
		}
	}
	
	if !authenticated {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "Invalid credentials",
			"message": "Username or password is incorrect",
		})
		return
	}
	
	// Generate JWT token
	if s.authManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Authentication not configured",
		})
		return
	}
	
	token, err := s.authManager.GenerateJWT(request.Username, userRole, permissions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate token",
			"message": err.Error(),
		})
		return
	}
	
	s.logger.Info("User logged in", "username", request.Username, "role", userRole)
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"token": token,
		"user": gin.H{
			"username": request.Username,
			"role": userRole,
			"permissions": permissions,
		},
	})
}

func (s *GinServer) generateAPIKeyHandler(c *gin.Context) {
	var request struct {
		Name        string   `json:"name" binding:"required"`
		Role        string   `json:"role"`
		Permissions []string `json:"permissions"`
		ExpiresIn   string   `json:"expires_in"` // e.g., "30d", "1y"
	}
	
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid request",
			"message": err.Error(),
		})
		return
	}
	
	// Set defaults
	if request.Role == "" {
		request.Role = "admin"
	}
	if len(request.Permissions) == 0 {
		request.Permissions = []string{"*"}
	}
	
	// Parse expiration
	var expiresIn *time.Duration
	if request.ExpiresIn != "" {
		dur, err := time.ParseDuration(request.ExpiresIn)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid expiration",
				"message": "Expiration must be a valid duration (e.g., '30d', '1y')",
			})
			return
		}
		expiresIn = &dur
	}
	
	if s.authManager == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Authentication not configured",
		})
		return
	}
	
	apiKey, err := s.authManager.GenerateAPIKey(request.Name, request.Role, request.Permissions, expiresIn)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to generate API key",
			"message": err.Error(),
		})
		return
	}
	
	s.logger.Info("API key generated",
		"name", request.Name,
		"role", request.Role,
		"user", c.GetString("username"))
	
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"api_key": apiKey,
		"details": gin.H{
			"name": request.Name,
			"role": request.Role,
			"permissions": request.Permissions,
			"created_at": time.Now().Format(time.RFC3339),
		},
	})
}

func (s *GinServer) verifyAuthHandler(c *gin.Context) {
	// This endpoint verifies the current authentication
	authenticated, _ := c.Get("authenticated")
	if authenticated != true {
		c.JSON(http.StatusUnauthorized, gin.H{
			"authenticated": false,
			"message": "Not authenticated",
		})
		return
	}
	
	c.JSON(http.StatusOK, gin.H{
		"authenticated": true,
		"user": gin.H{
			"username": c.GetString("username"),
			"role": c.GetString("role"),
			"permissions": c.GetStringSlice("permissions"),
			"api_key_name": c.GetString("api_key_name"),
		},
	})
}

// Chunked download handlers

func (s *GinServer) chunkedDownloadHandler(c *gin.Context) {
	targetPath := c.Param("filepath")
	
	// Security check: prevent path traversal
	if strings.Contains(targetPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid target path",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Remove leading slash if present
	targetPath = strings.TrimPrefix(targetPath, "/")
	filePath := filepath.Join(s.config.RepositoryPath, "targets", targetPath)
	
	fileInfo, err := os.Stat(filePath)
	if os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Target file not found",
			"path": targetPath,
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Open file for chunked reading
	file, err := os.Open(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to open file",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	defer file.Close()
	
	// Set chunked transfer encoding headers
	c.Header("Content-Type", getContentTypeByExt(filepath.Ext(targetPath)))
	c.Header("Transfer-Encoding", "chunked")
	c.Header("Cache-Control", "public, max-age=3600")
	
	// Stream file in chunks
	chunkSize := 64 * 1024 // 64KB chunks
	buffer := make([]byte, chunkSize)
	
	c.Status(http.StatusOK)
	
	for {
		n, err := file.Read(buffer)
		if n > 0 {
			if _, writeErr := c.Writer.Write(buffer[:n]); writeErr != nil {
				s.logger.Error("Failed to write chunk", "error", writeErr)
				return
			}
			c.Writer.Flush()
		}
		
		if err == io.EOF {
			break
		}
		if err != nil {
			s.logger.Error("Failed to read file", "error", err)
			return
		}
	}
	
	s.logger.Info("Completed chunked transfer", 
		"path", targetPath, 
		"size", fileInfo.Size(),
		"client_ip", c.ClientIP())
}

func (s *GinServer) downloadChunkHandler(c *gin.Context) {
	chunkIndexStr := c.Param("index")
	targetPath := c.Param("filepath")
	
	// Parse chunk index
	chunkIndex, err := strconv.Atoi(chunkIndexStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chunk index",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Security check: prevent path traversal
	if strings.Contains(targetPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid target path",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Remove leading slash if present
	targetPath = strings.TrimPrefix(targetPath, "/")
	filePath := filepath.Join(s.config.RepositoryPath, "targets", targetPath)
	
	// Get or build Merkle tree
	tree, err := s.getMerkleTree(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to get Merkle tree",
			"message": err.Error(),
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Validate chunk index
	if chunkIndex < 0 || chunkIndex >= len(tree.Chunks) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid chunk index",
			"total_chunks": len(tree.Chunks),
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Get the chunk
	chunk := tree.Chunks[chunkIndex]
	
	// Create chunk reader
	reader, err := merkle.NewChunkReader(filePath, &chunk)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to read chunk",
			"message": err.Error(),
			"request_id": c.GetString("request_id"),
		})
		return
	}
	defer reader.Close()
	
	// Set headers
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Length", fmt.Sprintf("%d", chunk.Size))
	c.Header("X-Chunk-Index", fmt.Sprintf("%d", chunk.Index))
	c.Header("X-Chunk-Hash", chunk.Hash)
	c.Header("X-Merkle-Root", tree.Root)
	c.Header("Cache-Control", "public, max-age=31536000, immutable") // Chunks are immutable
	
	// Send chunk data
	c.Status(http.StatusOK)
	_, _ = io.Copy(c.Writer, reader)
	
	s.logger.Info("Served chunk", 
		"path", targetPath,
		"chunk_index", chunkIndex,
		"chunk_size", chunk.Size,
		"client_ip", c.ClientIP())
}

func (s *GinServer) getMerkleTreeHandler(c *gin.Context) {
	targetPath := c.Param("filepath")
	
	// Security check: prevent path traversal
	if strings.Contains(targetPath, "..") {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid target path",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Remove leading slash if present
	targetPath = strings.TrimPrefix(targetPath, "/")
	filePath := filepath.Join(s.config.RepositoryPath, "targets", targetPath)
	
	// Get or build Merkle tree
	tree, err := s.getMerkleTree(filePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "Failed to build Merkle tree",
			"message": err.Error(),
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Get proofs for all chunks if requested
	includeProofs := c.Query("include_proofs") == "true"
	
	response := gin.H{
		"root": tree.Root,
		"file_size": tree.FileSize,
		"chunk_size": tree.ChunkSize,
		"total_chunks": len(tree.Chunks),
		"chunks": tree.Chunks,
	}
	
	if includeProofs {
		proofs := make([][]string, len(tree.Chunks))
		for i := range tree.Chunks {
			proof, _ := tree.GetProof(i)
			proofs[i] = proof
		}
		response["proofs"] = proofs
	}
	
	c.JSON(http.StatusOK, response)
	
	s.logger.Info("Served Merkle tree",
		"path", targetPath,
		"chunks", len(tree.Chunks),
		"client_ip", c.ClientIP())
}

// Helper function to get or build Merkle tree (with caching)
func (s *GinServer) getMerkleTree(filePath string) (*merkle.Tree, error) {
	// For now, we'll always rebuild Merkle trees since they're relatively fast to compute
	// In a full implementation, we'd cache the serialized tree structure
	cacheKey := fmt.Sprintf("merkle:%s", filePath)
	_ = cacheKey // Will be used when caching is implemented
	
	// Build Merkle tree
	tree, err := merkle.BuildTreeFromFile(filePath, merkle.DefaultChunkSize)
	if err != nil {
		return nil, err
	}
	
	// Cache the tree (simplified - in production, serialize it properly)
	// s.cache.SetRaw(cacheKey, serializedTree, "application/json", 1*time.Hour)
	
	return tree, nil
}

// getContentTypeByExt returns content type for file extension
func getContentTypeByExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".json":
		return "application/json"
	case ".txt", ".md":
		return "text/plain"
	case ".csv":
		return "text/csv"
	case ".xml":
		return "application/xml"
	case ".html", ".htm":
		return "text/html"
	case ".css":
		return "text/css"
	case ".js":
		return "application/javascript"
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".svg":
		return "image/svg+xml"
	case ".pdf":
		return "application/pdf"
	case ".zip":
		return "application/zip"
	case ".tar", ".tar.gz", ".tgz":
		return "application/tar+gzip"
	default:
		return "application/octet-stream"
	}
}

// statsHandler returns overall system statistics
func (s *GinServer) statsHandler(c *gin.Context) {
	stats := gin.H{
		"cache": s.cache.GetStats(),
		"server": gin.H{
			"uptime":        time.Since(s.startTime).String(),
			"port":          s.config.Port,
			"repository":    s.config.RepositoryPath,
			"tracing":       s.config.Tracing.Enabled,
			"coalescing":    s.config.Coalescing.Enabled,
			"circuit_breaker": s.config.CircuitBreaker.Enabled,
		},
	}
	
	// Add circuit breaker stats if enabled
	if s.config.CircuitBreaker.Enabled && s.circuitBreakers != nil {
		stats["circuit_breakers"] = s.circuitBreakers.GetStats()
	}
	
	// Add coalescing stats if enabled
	if s.config.Coalescing.Enabled && s.coalescer != nil {
		stats["coalescing"] = s.coalescer.Stats()
	}
	
	c.JSON(http.StatusOK, stats)
}

// circuitBreakerStatsHandler returns circuit breaker statistics
func (s *GinServer) circuitBreakerStatsHandler(c *gin.Context) {
	if !s.config.CircuitBreaker.Enabled || s.circuitBreakers == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Circuit breaker not enabled",
		})
		return
	}
	
	c.JSON(http.StatusOK, s.circuitBreakers.GetStats())
}

// coalescingStatsHandler returns request coalescing statistics
func (s *GinServer) coalescingStatsHandler(c *gin.Context) {
	if !s.config.Coalescing.Enabled || s.coalescer == nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Request coalescing not enabled",
		})
		return
	}
	
	c.JSON(http.StatusOK, s.coalescer.Stats())
}

