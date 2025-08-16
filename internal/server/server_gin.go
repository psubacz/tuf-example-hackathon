package server

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tuf-golang-project/internal/logger"
	"tuf-golang-project/internal/middleware"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// GinServer represents the TUF repository server using Gin framework
type GinServer struct {
	config   *Config
	router   *gin.Engine
	server   *http.Server
	logger   *slog.Logger
	shutdown chan struct{}
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
	
	s := &GinServer{
		config:   config,
		router:   router,
		logger:   logger.Logger,
		shutdown: make(chan struct{}),
	}

	s.setupMiddleware()
	s.setupRoutes()
	s.setupServer()

	return s
}

// setupMiddleware configures the middleware stack
func (s *GinServer) setupMiddleware() {
	// Core middleware stack
	s.router.Use(middleware.Recovery())
	s.router.Use(middleware.Logger())
	s.router.Use(middleware.RequestID())
	s.router.Use(middleware.Metrics())
	
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
	targets := s.router.Group("/targets")
	{
		targets.GET("/*filepath", s.downloadTargetHandler)
		targets.HEAD("/*filepath", s.checkTargetHandler)
	}

	// Admin API endpoints (will add auth middleware later)
	admin := s.router.Group("/admin")
	{
		admin.POST("/targets/add", s.addTargetHandler)
		admin.POST("/targets/remove", s.removeTargetHandler)
		admin.POST("/metadata/sign", s.signMetadataHandler)
		admin.GET("/audit/logs", s.auditLogsHandler)
	}

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
		"endpoints": map[string]string{
			"metadata": fmt.Sprintf("/metadata/"),
			"targets":  fmt.Sprintf("/targets/"),
			"health":   fmt.Sprintf("/health"),
			"metrics":  fmt.Sprintf("/metrics"),
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
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "root.json")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Root metadata not found")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Root metadata not found",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Set caching headers for root metadata (short TTL)
	c.Header("Cache-Control", "public, max-age=60")
	c.Header("Content-Type", "application/json")
	
	s.logger.Info("Serving root metadata", "client_ip", c.ClientIP())
	c.File(filePath)
}

func (s *GinServer) timestampMetadataHandler(c *gin.Context) {
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "timestamp.json")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Timestamp metadata not found")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Timestamp metadata not found",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Timestamp should have very short cache (5 seconds) for freshness
	c.Header("Cache-Control", "public, max-age=5")
	c.Header("Content-Type", "application/json")
	
	s.logger.Info("Serving timestamp metadata", "client_ip", c.ClientIP())
	c.File(filePath)
}

func (s *GinServer) snapshotMetadataHandler(c *gin.Context) {
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "snapshot.json")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Snapshot metadata not found")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Snapshot metadata not found",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Snapshot can have moderate cache (5 minutes)
	c.Header("Cache-Control", "public, max-age=300")
	c.Header("Content-Type", "application/json")
	
	s.logger.Info("Serving snapshot metadata", "client_ip", c.ClientIP())
	c.File(filePath)
}

func (s *GinServer) targetsMetadataHandler(c *gin.Context) {
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", "targets.json")
	
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Targets metadata not found")
		c.JSON(http.StatusNotFound, gin.H{
			"error": "Targets metadata not found",
			"request_id": c.GetString("request_id"),
		})
		return
	}
	
	// Targets can have longer cache (1 hour)
	c.Header("Cache-Control", "public, max-age=3600")
	c.Header("Content-Type", "application/json")
	
	s.logger.Info("Serving targets metadata", "client_ip", c.ClientIP())
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
	
	// Set appropriate content type
	contentType := getContentTypeByExt(filepath.Ext(targetPath))
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filepath.Base(targetPath)))
	c.Header("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))
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
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Not implemented yet"})
}

func (s *GinServer) removeTargetHandler(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Not implemented yet"})
}

func (s *GinServer) signMetadataHandler(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Not implemented yet"})
}

func (s *GinServer) auditLogsHandler(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"error": "Not implemented yet"})
}

func (s *GinServer) notFoundHandler(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{
		"error": "Not found",
		"path": c.Request.URL.Path,
		"request_id": c.GetString("request_id"),
	})
}