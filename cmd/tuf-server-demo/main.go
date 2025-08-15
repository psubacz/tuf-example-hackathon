package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// CORS middleware to allow cross-origin requests
func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		
		next.ServeHTTP(w, r)
	})
}

// Security headers middleware
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		next.ServeHTTP(w, r)
	})
}

// Logging middleware
func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		log.Printf("%s %s %s %v", r.Method, r.URL.Path, r.RemoteAddr, time.Since(start))
	})
}

// TUF Repository Server
type TUFServer struct {
	repoPath string
	port     int
}

func NewTUFServer(repoPath string, port int) *TUFServer {
	return &TUFServer{
		repoPath: repoPath,
		port:     port,
	}
}

func (s *TUFServer) Start() error {
	// Check if repository exists
	if _, err := os.Stat(s.repoPath); os.IsNotExist(err) {
		return fmt.Errorf("TUF repository not found at %s. Run 'go run cmd/tuf-demo' first", s.repoPath)
	}

	// Create HTTP server
	mux := http.NewServeMux()
	
	// Health check endpoint
	mux.HandleFunc("/health", s.healthHandler)
	
	// Repository info endpoint
	mux.HandleFunc("/info", s.infoHandler)
	
	// TUF metadata endpoints
	mux.HandleFunc("/metadata/", s.metadataHandler)
	
	// TUF targets endpoints
	mux.HandleFunc("/targets/", s.targetsHandler)
	
	// Root endpoint with repository information
	mux.HandleFunc("/", s.rootHandler)

	// Apply middleware
	handler := loggingMiddleware(securityHeaders(enableCORS(mux)))
	
	// Start server
	addr := fmt.Sprintf(":%d", s.port)
	server := &http.Server{
		Addr:         addr,
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	fmt.Printf("🚀 TUF Repository Server starting...\n")
	fmt.Printf("📁 Repository: %s\n", s.repoPath)
	fmt.Printf("🌐 Server: http://localhost%s\n", addr)
	fmt.Printf("📋 Endpoints:\n")
	fmt.Printf("   - http://localhost%s/health (health check)\n", addr)
	fmt.Printf("   - http://localhost%s/info (repository info)\n", addr)
	fmt.Printf("   - http://localhost%s/metadata/ (TUF metadata)\n", addr)
	fmt.Printf("   - http://localhost%s/targets/ (target files)\n", addr)
	fmt.Printf("\n✅ Ready for over-the-air updates!\n\n")

	return server.ListenAndServe()
}

func (s *TUFServer) healthHandler(w http.ResponseWriter, r *http.Request) {
	response := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"service":   "TUF Repository Server",
		"version":   "1.0.0",
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *TUFServer) infoHandler(w http.ResponseWriter, r *http.Request) {
	// Get repository statistics
	metadataDir := filepath.Join(s.repoPath, "metadata")
	targetsDir := filepath.Join(s.repoPath, "targets")
	
	// Count metadata files
	metadataFiles, _ := filepath.Glob(filepath.Join(metadataDir, "*.json"))
	
	// Count target files
	targetFiles, _ := filepath.Glob(filepath.Join(targetsDir, "*"))
	var targetCount int
	for _, file := range targetFiles {
		if info, err := os.Stat(file); err == nil && !info.IsDir() {
			targetCount++
		}
	}
	
	// Load targets metadata to get file list
	targetsMetadata := make(map[string]interface{})
	if data, err := os.ReadFile(filepath.Join(metadataDir, "targets.json")); err == nil {
		json.Unmarshal(data, &targetsMetadata)
	}
	
	response := map[string]interface{}{
		"repository": map[string]interface{}{
			"path":           s.repoPath,
			"metadata_files": len(metadataFiles),
			"target_files":   targetCount,
			"server_port":    s.port,
		},
		"endpoints": map[string]string{
			"metadata": fmt.Sprintf("http://localhost:%d/metadata/", s.port),
			"targets":  fmt.Sprintf("http://localhost:%d/targets/", s.port),
			"health":   fmt.Sprintf("http://localhost:%d/health", s.port),
		},
		"targets_metadata": targetsMetadata,
		"timestamp":        time.Now().Format(time.RFC3339),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *TUFServer) metadataHandler(w http.ResponseWriter, r *http.Request) {
	// Extract metadata file name from URL
	path := strings.TrimPrefix(r.URL.Path, "/metadata/")
	if path == "" {
		s.listMetadataFiles(w, r)
		return
	}
	
	// Security: only allow .json files and prevent path traversal
	if !strings.HasSuffix(path, ".json") || strings.Contains(path, "..") {
		http.Error(w, "Invalid metadata file", http.StatusBadRequest)
		return
	}
	
	// Serve metadata file
	filePath := filepath.Join(s.repoPath, "metadata", path)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Metadata file not found", http.StatusNotFound)
		return
	}
	
	w.Header().Set("Content-Type", "application/json")
	http.ServeFile(w, r, filePath)
}

func (s *TUFServer) targetsHandler(w http.ResponseWriter, r *http.Request) {
	// Extract target file name from URL
	path := strings.TrimPrefix(r.URL.Path, "/targets/")
	if path == "" {
		s.listTargetFiles(w, r)
		return
	}
	
	// Security: prevent path traversal
	if strings.Contains(path, "..") {
		http.Error(w, "Invalid target path", http.StatusBadRequest)
		return
	}
	
	// Serve target file
	filePath := filepath.Join(s.repoPath, "targets", path)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		http.Error(w, "Target file not found", http.StatusNotFound)
		return
	}
	
	// Set appropriate content type based on file extension
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		w.Header().Set("Content-Type", "application/json")
	case ".txt", ".md":
		w.Header().Set("Content-Type", "text/plain")
	case ".csv":
		w.Header().Set("Content-Type", "text/csv")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
	
	http.ServeFile(w, r, filePath)
}

func (s *TUFServer) listMetadataFiles(w http.ResponseWriter, r *http.Request) {
	metadataDir := filepath.Join(s.repoPath, "metadata")
	files, err := os.ReadDir(metadataDir)
	if err != nil {
		http.Error(w, "Could not read metadata directory", http.StatusInternalServerError)
		return
	}
	
	var metadataFiles []map[string]interface{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			info, _ := file.Info()
			metadataFiles = append(metadataFiles, map[string]interface{}{
				"name":         file.Name(),
				"size":         info.Size(),
				"modified":     info.ModTime().Format(time.RFC3339),
				"download_url": fmt.Sprintf("http://localhost:%d/metadata/%s", s.port, file.Name()),
			})
		}
	}
	
	response := map[string]interface{}{
		"metadata_files": metadataFiles,
		"count":          len(metadataFiles),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *TUFServer) listTargetFiles(w http.ResponseWriter, r *http.Request) {
	targetsDir := filepath.Join(s.repoPath, "targets")
	files, err := os.ReadDir(targetsDir)
	if err != nil {
		http.Error(w, "Could not read targets directory", http.StatusInternalServerError)
		return
	}
	
	var targetFiles []map[string]interface{}
	for _, file := range files {
		if !file.IsDir() {
			info, _ := file.Info()
			targetFiles = append(targetFiles, map[string]interface{}{
				"name":         file.Name(),
				"size":         info.Size(),
				"modified":     info.ModTime().Format(time.RFC3339),
				"download_url": fmt.Sprintf("http://localhost:%d/targets/%s", s.port, file.Name()),
			})
		}
	}
	
	response := map[string]interface{}{
		"target_files": targetFiles,
		"count":        len(targetFiles),
	}
	
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

func (s *TUFServer) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	
	html := fmt.Sprintf(`<!DOCTYPE html>
<html>
<head>
    <title>TUF Repository Server</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 40px; background: #f5f5f5; }
        .container { max-width: 800px; margin: 0 auto; background: white; padding: 30px; border-radius: 8px; box-shadow: 0 2px 10px rgba(0,0,0,0.1); }
        h1 { color: #2c3e50; margin-bottom: 20px; }
        .endpoint { background: #ecf0f1; padding: 15px; margin: 10px 0; border-radius: 5px; border-left: 4px solid #3498db; }
        .endpoint code { background: #34495e; color: white; padding: 2px 6px; border-radius: 3px; }
        .status { color: #27ae60; font-weight: bold; }
        a { color: #3498db; text-decoration: none; }
        a:hover { text-decoration: underline; }
        .security { background: #fff3cd; border: 1px solid #ffeaa7; padding: 15px; border-radius: 5px; margin: 20px 0; }
    </style>
</head>
<body>
    <div class="container">
        <h1>🔐 TUF Repository Server</h1>
        <p class="status">✅ Server is running and ready for over-the-air updates!</p>
        
        <h2>📋 Available Endpoints</h2>
        
        <div class="endpoint">
            <strong>Health Check:</strong><br>
            <code>GET <a href="/health">/health</a></code><br>
            Check server status and uptime
        </div>
        
        <div class="endpoint">
            <strong>Repository Info:</strong><br>
            <code>GET <a href="/info">/info</a></code><br>
            Get repository statistics and available files
        </div>
        
        <div class="endpoint">
            <strong>TUF Metadata:</strong><br>
            <code>GET <a href="/metadata/">/metadata/</a></code><br>
            <code>GET <a href="/metadata/root.json">/metadata/root.json</a></code><br>
            <code>GET <a href="/metadata/targets.json">/metadata/targets.json</a></code><br>
            <code>GET <a href="/metadata/snapshot.json">/metadata/snapshot.json</a></code><br>
            <code>GET <a href="/metadata/timestamp.json">/metadata/timestamp.json</a></code>
        </div>
        
        <div class="endpoint">
            <strong>Target Files:</strong><br>
            <code>GET <a href="/targets/">/targets/</a></code><br>
            <code>GET <a href="/targets/sample.txt">/targets/sample.txt</a></code><br>
            Download secured files with TUF verification
        </div>
        
        <div class="security">
            <strong>🛡️ Security Note:</strong> This server implements TUF (The Update Framework) which provides:
            <ul>
                <li>Cryptographic verification of file integrity</li>
                <li>Protection against rollback attacks</li>
                <li>Secure key distribution and rotation</li>
                <li>Freshness guarantees via timestamps</li>
            </ul>
        </div>
        
        <h2>🚀 For Production Deployment</h2>
        <p>For production use, consider:</p>
        <ul>
            <li>HTTPS/TLS encryption</li>
            <li>Rate limiting and DDoS protection</li>
            <li>CDN for global distribution</li>
            <li>Monitoring and logging</li>
            <li>Backup and disaster recovery</li>
        </ul>
        
        <p><small>Server running on port %d | Repository: %s</small></p>
    </div>
</body>
</html>`, s.port, s.repoPath)
	
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(html))
}

func main() {
	// Default values
	defaultRepo := "./tuf-repository"
	defaultPort := 8080
	
	// Parse command line arguments
	repoPath := defaultRepo
	port := defaultPort
	
	args := os.Args[1:]
	for i, arg := range args {
		switch arg {
		case "--repo", "-r":
			if i+1 < len(args) {
				repoPath = args[i+1]
			}
		case "--port", "-p":
			if i+1 < len(args) {
				if p, err := strconv.Atoi(args[i+1]); err == nil {
					port = p
				}
			}
		case "--help", "-h":
			fmt.Printf(`TUF Repository Server

Usage: go run cmd/tuf-server-demo [options]

Options:
  --repo, -r    Repository path (default: ./tuf-repository)
  --port, -p    Server port (default: 8080)
  --help, -h    Show this help message

Examples:
  go run cmd/tuf-server-demo
  go run cmd/tuf-server-demo --port 9000
  go run cmd/tuf-server-demo --repo /path/to/repo --port 8080

The server will serve TUF metadata and target files over HTTP for
over-the-air updates. Clients can securely download and verify files
using the TUF protocol.
`)
			return
		}
	}
	
	// Create and start server
	server := NewTUFServer(repoPath, port)
	if err := server.Start(); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}