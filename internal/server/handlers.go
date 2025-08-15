package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// metadataHandler serves TUF metadata files
func (s *Server) metadataHandler(w http.ResponseWriter, r *http.Request) {
	// Extract metadata file name from URL
	path := strings.TrimPrefix(r.URL.Path, "/metadata/")
	if path == "" {
		s.listMetadataFiles(w, r)
		return
	}

	// Security: only allow .json files and prevent path traversal
	if !strings.HasSuffix(path, ".json") || strings.Contains(path, "..") {
		s.logger.Warn("Invalid metadata file requested: %s from %s", path, getClientIP(r))
		http.Error(w, "Invalid metadata file", http.StatusBadRequest)
		return
	}

	// Serve metadata file
	filePath := filepath.Join(s.config.RepositoryPath, "metadata", path)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Metadata file not found: %s", path)
		http.Error(w, "Metadata file not found", http.StatusNotFound)
		return
	}

	// Set appropriate headers for TUF metadata
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(s.config.Cache.TTL.Seconds())))

	s.logger.Warn("Serving metadata: %s to %s", path, getClientIP(r))
	http.ServeFile(w, r, filePath)
}

// targetsHandler serves TUF target files
func (s *Server) targetsHandler(w http.ResponseWriter, r *http.Request) {
	// Extract target file name from URL
	path := strings.TrimPrefix(r.URL.Path, "/targets/")
	if path == "" {
		s.listTargetFiles(w, r)
		return
	}

	// Security: prevent path traversal
	if strings.Contains(path, "..") {
		s.logger.Warn("Invalid target path requested: %s from %s", path, getClientIP(r))
		http.Error(w, "Invalid target path", http.StatusBadRequest)
		return
	}

	// Serve target file
	filePath := filepath.Join(s.config.RepositoryPath, "targets", path)
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		s.logger.Warn("Target file not found: %s", path)
		http.Error(w, "Target file not found", http.StatusNotFound)
		return
	}

	// Set appropriate content type and headers
	s.setContentType(w, filepath.Ext(path))
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(s.config.Cache.TTL.Seconds())))

	s.logger.Warn("Serving target: %s to %s", path, getClientIP(r))
	http.ServeFile(w, r, filePath)
}

// setContentType sets appropriate content type based on file extension
func (s *Server) setContentType(w http.ResponseWriter, ext string) {
	switch strings.ToLower(ext) {
	case ".json":
		w.Header().Set("Content-Type", "application/json")
	case ".txt", ".md":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	case ".csv":
		w.Header().Set("Content-Type", "text/csv")
	case ".xml":
		w.Header().Set("Content-Type", "application/xml")
	case ".html", ".htm":
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
	case ".css":
		w.Header().Set("Content-Type", "text/css")
	case ".js":
		w.Header().Set("Content-Type", "application/javascript")
	case ".png":
		w.Header().Set("Content-Type", "image/png")
	case ".jpg", ".jpeg":
		w.Header().Set("Content-Type", "image/jpeg")
	case ".gif":
		w.Header().Set("Content-Type", "image/gif")
	case ".svg":
		w.Header().Set("Content-Type", "image/svg+xml")
	case ".pdf":
		w.Header().Set("Content-Type", "application/pdf")
	case ".zip":
		w.Header().Set("Content-Type", "application/zip")
	case ".tar", ".tar.gz", ".tgz":
		w.Header().Set("Content-Type", "application/tar+gzip")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
	}
}

// listMetadataFiles lists available metadata files
func (s *Server) listMetadataFiles(w http.ResponseWriter, r *http.Request) {
	metadataDir := filepath.Join(s.config.RepositoryPath, "metadata")
	files, err := os.ReadDir(metadataDir)
	if err != nil {
		s.logger.Warn("Could not read metadata directory: %v", err)
		http.Error(w, "Could not read metadata directory", http.StatusInternalServerError)
		return
	}

	var metadataFiles []map[string]interface{}
	for _, file := range files {
		if !file.IsDir() && strings.HasSuffix(file.Name(), ".json") {
			info, _ := file.Info()

			// Calculate file checksum for integrity
			checksum := ""
			if filePath := filepath.Join(metadataDir, file.Name()); filePath != "" {
				if hash, err := calculateSHA256(filePath); err == nil {
					checksum = hash
				}
			}

			metadataFiles = append(metadataFiles, map[string]interface{}{
				"name":         file.Name(),
				"size":         info.Size(),
				"modified":     info.ModTime().Format(time.RFC3339),
				"checksum":     checksum,
				"download_url": fmt.Sprintf("http://localhost:%d/metadata/%s", s.config.Port, file.Name()),
			})
		}
	}

	response := map[string]interface{}{
		"metadata_files": metadataFiles,
		"count":          len(metadataFiles),
		"timestamp":      time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(s.config.Cache.TTL.Seconds())))
	json.NewEncoder(w).Encode(response)
}

// listTargetFiles lists available target files
func (s *Server) listTargetFiles(w http.ResponseWriter, r *http.Request) {
	targetsDir := filepath.Join(s.config.RepositoryPath, "targets")
	files, err := os.ReadDir(targetsDir)
	if err != nil {
		s.logger.Warn("Could not read targets directory: %v", err)
		http.Error(w, "Could not read targets directory", http.StatusInternalServerError)
		return
	}

	var targetFiles []map[string]interface{}
	for _, file := range files {
		if !file.IsDir() {
			info, _ := file.Info()

			// Calculate file checksum for integrity
			checksum := ""
			if filePath := filepath.Join(targetsDir, file.Name()); filePath != "" {
				if hash, err := calculateSHA256(filePath); err == nil {
					checksum = hash
				}
			}

			targetFiles = append(targetFiles, map[string]interface{}{
				"name":         file.Name(),
				"size":         info.Size(),
				"modified":     info.ModTime().Format(time.RFC3339),
				"checksum":     checksum,
				"content_type": getContentTypeByExt(filepath.Ext(file.Name())),
				"download_url": fmt.Sprintf("http://localhost:%d/targets/%s", s.config.Port, file.Name()),
			})
		}
	}

	response := map[string]interface{}{
		"target_files": targetFiles,
		"count":        len(targetFiles),
		"timestamp":    time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", fmt.Sprintf("public, max-age=%d", int(s.config.Cache.TTL.Seconds())))
	json.NewEncoder(w).Encode(response)
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

// rootHandler serves the HTML dashboard
func (s *Server) rootHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Get repository statistics for the dashboard
	info := s.getRepositoryInfo()
	repoInfo, ok := info["repository"].(map[string]interface{})
	if !ok {
		repoInfo = make(map[string]interface{})
	}

	serverInfo, ok := info["server"].(map[string]interface{})
	if !ok {
		serverInfo = make(map[string]interface{})
	}

	html := s.generateDashboardHTML(repoInfo, serverInfo)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write([]byte(html))
}

// generateDashboardHTML creates the HTML dashboard
func (s *Server) generateDashboardHTML(repoInfo, serverInfo map[string]interface{}) string {
	tlsStatus := "❌ HTTP Only"
	if s.config.TLS.Enabled {
		tlsStatus = "✅ HTTPS Enabled"
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>TUF Repository Server v2.0</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; background: linear-gradient(135deg, #667eea 0%%, #764ba2 100%%); min-height: 100vh; }
        .container { max-width: 1200px; margin: 0 auto; padding: 20px; }
        .header { background: rgba(255,255,255,0.1); backdrop-filter: blur(10px); padding: 30px; border-radius: 15px; margin-bottom: 30px; color: white; text-align: center; }
        .header h1 { font-size: 2.5em; margin-bottom: 10px; text-shadow: 2px 2px 4px rgba(0,0,0,0.3); }
        .status { display: inline-block; background: #27ae60; color: white; padding: 8px 16px; border-radius: 20px; font-weight: bold; margin: 10px 5px; }
        .stats { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin: 30px 0; }
        .stat-card { background: rgba(255,255,255,0.95); padding: 25px; border-radius: 15px; box-shadow: 0 8px 32px rgba(0,0,0,0.1); transition: transform 0.3s ease; }
        .stat-card:hover { transform: translateY(-5px); }
        .stat-card h3 { color: #2c3e50; font-size: 1.2em; margin-bottom: 15px; display: flex; align-items: center; }
        .stat-card .icon { margin-right: 10px; font-size: 1.3em; }
        .endpoints { background: rgba(255,255,255,0.95); padding: 25px; border-radius: 15px; margin: 20px 0; box-shadow: 0 8px 32px rgba(0,0,0,0.1); }
        .endpoint { background: #f8f9fa; padding: 15px; margin: 10px 0; border-radius: 8px; border-left: 4px solid #3498db; transition: all 0.3s ease; }
        .endpoint:hover { background: #e9ecef; transform: translateX(5px); }
        .endpoint code { background: #2c3e50; color: #ecf0f1; padding: 4px 8px; border-radius: 4px; font-family: 'Monaco', 'Consolas', monospace; }
        .endpoint a { color: #3498db; text-decoration: none; font-weight: bold; }
        .endpoint a:hover { text-decoration: underline; }
        .security { background: linear-gradient(45deg, #fff3cd, #ffeaa7); border: 1px solid #ffeaa7; padding: 20px; border-radius: 10px; margin: 20px 0; }
        .footer { text-align: center; color: rgba(255,255,255,0.8); margin-top: 40px; }
        .metric { display: flex; justify-content: space-between; margin: 8px 0; }
        .metric-value { font-weight: bold; color: #2980b9; }
        @media (max-width: 768px) { 
            .container { padding: 10px; }
            .header h1 { font-size: 2em; }
            .stats { grid-template-columns: 1fr; }
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>🔐 TUF Repository Server</h1>
            <div class="status">✅ Server Online</div>
            <div class="status">%s</div>
            <div class="status">📊 Enhanced v2.0</div>
        </div>
        
        <div class="stats">
            <div class="stat-card">
                <h3><span class="icon">📁</span>Repository Info</h3>
                <div class="metric">
                    <span>Path:</span>
                    <span class="metric-value">%s</span>
                </div>
                <div class="metric">
                    <span>Metadata Files:</span>
                    <span class="metric-value">%v</span>
                </div>
                <div class="metric">
                    <span>Target Files:</span>
                    <span class="metric-value">%v</span>
                </div>
                <div class="metric">
                    <span>Server Port:</span>
                    <span class="metric-value">%v</span>
                </div>
            </div>
            
            <div class="stat-card">
                <h3><span class="icon">⚡</span>Server Stats</h3>
                <div class="metric">
                    <span>Version:</span>
                    <span class="metric-value">%v</span>
                </div>
                <div class="metric">
                    <span>Uptime:</span>
                    <span class="metric-value">%v</span>
                </div>
                <div class="metric">
                    <span>Total Requests:</span>
                    <span class="metric-value">%v</span>
                </div>
                <div class="metric">
                    <span>TLS Enabled:</span>
                    <span class="metric-value">%v</span>
                </div>
            </div>
        </div>

        <div class="endpoints">
            <h2 style="color: #2c3e50; margin-bottom: 20px;">📋 API Endpoints</h2>
            
            <div class="endpoint">
                <strong>🔍 Health Check:</strong><br>
                <code>GET <a href="/health">/health</a></code><br>
                Check server status, uptime, and repository health
            </div>
            
            <div class="endpoint">
                <strong>📊 Metrics:</strong><br>
                <code>GET <a href="/metrics">/metrics</a></code><br>
                Server performance metrics and monitoring data
            </div>
            
            <div class="endpoint">
                <strong>ℹ️ Repository Info:</strong><br>
                <code>GET <a href="/info">/info</a></code><br>
                Detailed repository statistics and configuration
            </div>
            
            <div class="endpoint">
                <strong>🔐 TUF Metadata:</strong><br>
                <code>GET <a href="/metadata/">/metadata/</a></code> - List all metadata files<br>
                <code>GET <a href="/metadata/root.json">/metadata/root.json</a></code> - Root metadata<br>
                <code>GET <a href="/metadata/targets.json">/metadata/targets.json</a></code> - Targets metadata<br>
                <code>GET <a href="/metadata/snapshot.json">/metadata/snapshot.json</a></code> - Snapshot metadata<br>
                <code>GET <a href="/metadata/timestamp.json">/metadata/timestamp.json</a></code> - Timestamp metadata
            </div>
            
            <div class="endpoint">
                <strong>🎯 Target Files:</strong><br>
                <code>GET <a href="/targets/">/targets/</a></code> - List all target files<br>
                <code>GET <a href="/targets/sample.txt">/targets/sample.txt</a></code> - Download files with TUF verification
            </div>
        </div>
        
        <div class="security">
            <h3 style="color: #856404; margin-bottom: 15px;">🛡️ Security & Performance Features</h3>
            <ul style="color: #856404; margin-left: 20px;">
                <li><strong>TUF Security:</strong> Cryptographic verification, rollback protection, freshness guarantees</li>
                <li><strong>Rate Limiting:</strong> Protection against abuse and DDoS attacks</li>
                <li><strong>Compression:</strong> Gzip compression for faster transfers</li>
                <li><strong>Security Headers:</strong> CSP, HSTS, XSS protection, and more</li>
                <li><strong>Monitoring:</strong> Real-time metrics and health checks</li>
                <li><strong>CORS Support:</strong> Cross-origin resource sharing for web clients</li>
            </ul>
        </div>
        
        <div class="footer">
            <p>TUF Repository Server v2.0 | Enhanced for Production Use</p>
            <p><small>Server running on port %d | Repository: %s</small></p>
        </div>
    </div>
</body>
</html>`,
		tlsStatus,
		repoInfo["path"],
		repoInfo["metadata_files"],
		repoInfo["target_files"],
		repoInfo["server_port"],
		serverInfo["version"],
		serverInfo["uptime"],
		serverInfo["requests"],
		serverInfo["tls_enabled"],
		s.config.Port,
		s.config.RepositoryPath)
}
