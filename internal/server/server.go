package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tuf-golang-project/internal/logger"
)

// Server represents the TUF repository server
type Server struct {
	config   *Config
	mux      *http.ServeMux
	server   *http.Server
	logger   *slog.Logger
	metrics  *Metrics
	shutdown chan struct{}
}

// New creates a new TUF server instance
func New(config *Config) *Server {
	s := &Server{
		config:   config,
		mux:      http.NewServeMux(),
		logger:   logger.Logger,
		metrics:  NewMetrics(),
		shutdown: make(chan struct{}),
	}

	s.setupRoutes()
	s.setupServer()

	return s
}

// setupRoutes configures all HTTP routes
func (s *Server) setupRoutes() {
	// Health and monitoring endpoints
	s.mux.HandleFunc("/health", s.withMiddleware(s.healthHandler))
	s.mux.HandleFunc("/metrics", s.withMiddleware(s.metricsHandler))
	s.mux.HandleFunc("/info", s.withMiddleware(s.infoHandler))

	// TUF metadata endpoints
	s.mux.HandleFunc("/metadata/", s.withMiddleware(s.metadataHandler))

	// TUF targets endpoints
	s.mux.HandleFunc("/targets/", s.withMiddleware(s.targetsHandler))

	// Root endpoint with dashboard
	s.mux.HandleFunc("/", s.withMiddleware(s.rootHandler))
}

// setupServer configures the HTTP server
func (s *Server) setupServer() {
	s.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.config.Port),
		Handler:      s.mux,
		ReadTimeout:  time.Duration(s.config.ReadTimeout) * time.Second,
		WriteTimeout: time.Duration(s.config.WriteTimeout) * time.Second,
		IdleTimeout:  time.Duration(s.config.IdleTimeout) * time.Second,
		ErrorLog:     log.New(os.Stderr, "[HTTP-ERROR] ", log.LstdFlags),
	}
}

// Start starts the server
func (s *Server) Start() error {
	// Check if repository exists
	if _, err := os.Stat(s.config.RepositoryPath); os.IsNotExist(err) {
		return fmt.Errorf("TUF repository not found at %s. Run 'go run cmd/tuf-demo' first", s.config.RepositoryPath)
	}

	s.logger.Info("TUF Repository Server starting")
	s.logger.Info("Server configuration",
		"repository", s.config.RepositoryPath,
		"server", fmt.Sprintf("http://localhost:%d", s.config.Port),
		"log_level", s.config.LogLevel,
		"cors_enabled", s.config.CORS.Enabled)

	if s.config.TLS.Enabled {
		s.logger.Info("TLS enabled", "cert_file", s.config.TLS.CertFile, "key_file", s.config.TLS.KeyFile)
		return s.server.ListenAndServeTLS(s.config.TLS.CertFile, s.config.TLS.KeyFile)
	}

	return s.server.ListenAndServe()
}

// Stop gracefully stops the server
func (s *Server) Stop(ctx context.Context) error {
	s.logger.Info("Gracefully shutting down server")

	// Signal shutdown to other goroutines
	close(s.shutdown)

	// Shutdown HTTP server
	if err := s.server.Shutdown(ctx); err != nil {
		s.logger.Error("Server shutdown error", "error", err)
		return err
	}

	s.logger.Info("Server stopped successfully")
	return nil
}

// withMiddleware applies middleware chain to handlers
func (s *Server) withMiddleware(handler http.HandlerFunc) http.HandlerFunc {
	return s.loggingMiddleware(
		s.metricsMiddleware(
			s.compressionMiddleware(
				s.securityMiddleware(
					s.corsMiddleware(
						s.rateLimitMiddleware(handler),
					),
				),
			),
		),
	)
}

// healthHandler provides health check endpoint
func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	health := s.checkHealth()

	status := http.StatusOK
	if health["status"] != "healthy" {
		status = http.StatusServiceUnavailable
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(health)
}

// checkHealth performs health checks
func (s *Server) checkHealth() map[string]interface{} {
	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "TUF Repository Server",
		"version":   "2.0.0",
		"uptime":    time.Since(s.metrics.StartTime).String(),
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

// infoHandler provides repository information
func (s *Server) infoHandler(w http.ResponseWriter, r *http.Request) {
	info := s.getRepositoryInfo()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(info)
}

// getRepositoryInfo gathers repository statistics
func (s *Server) getRepositoryInfo() map[string]interface{} {
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

	// Load targets metadata
	targetsMetadata := make(map[string]interface{})
	if data, err := os.ReadFile(filepath.Join(metadataDir, "targets.json")); err == nil {
		json.Unmarshal(data, &targetsMetadata)
	}

	return map[string]interface{}{
		"repository": map[string]interface{}{
			"path":           s.config.RepositoryPath,
			"metadata_files": len(metadataFiles),
			"target_files":   targetCount,
			"server_port":    s.config.Port,
		},
		"server": map[string]interface{}{
			"version":     "2.0.0",
			"uptime":      time.Since(s.metrics.StartTime).String(),
			"requests":    s.metrics.TotalRequests,
			"tls_enabled": s.config.TLS.Enabled,
		},
		"endpoints": map[string]string{
			"metadata": fmt.Sprintf("http://localhost:%d/metadata/", s.config.Port),
			"targets":  fmt.Sprintf("http://localhost:%d/targets/", s.config.Port),
			"health":   fmt.Sprintf("http://localhost:%d/health", s.config.Port),
			"metrics":  fmt.Sprintf("http://localhost:%d/metrics", s.config.Port),
		},
		"targets_metadata": targetsMetadata,
		"timestamp":        time.Now().Format(time.RFC3339),
	}
}
