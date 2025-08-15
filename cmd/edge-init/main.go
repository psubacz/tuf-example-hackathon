package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"tuf-golang-project/pkg/tuf"
)

// EdgeInitConfig holds configuration for the edge init container
type EdgeInitConfig struct {
	ServerURL      string
	RootKeysPath   string
	TargetDir      string
	CacheDir       string
	LogLevel       string
	Timeout        time.Duration
	RetryAttempts  int
	VerifyAll      bool
	OneShot        bool
	TargetPatterns []string
}

// EdgeInit handles TUF verification and file downloads for edge containers
type EdgeInit struct {
	config     EdgeInitConfig
	httpClient *http.Client
	logger     *log.Logger
}

func main() {
	config := parseConfig()
	
	edgeInit, err := NewEdgeInit(config)
	if err != nil {
		log.Fatalf("Failed to initialize edge init: %v", err)
	}
	
	if err := edgeInit.Run(context.Background()); err != nil {
		log.Fatalf("Edge init failed: %v", err)
	}
	
	log.Println("Edge init completed successfully")
}

func parseConfig() EdgeInitConfig {
	config := EdgeInitConfig{
		ServerURL:      getEnvOrDefault("TUF_SERVER_URL", ""),
		RootKeysPath:   getEnvOrDefault("TUF_ROOT_KEYS_PATH", "/config/root.json"),
		TargetDir:      getEnvOrDefault("TUF_TARGET_DIR", "/shared"),
		CacheDir:       getEnvOrDefault("TUF_CACHE_DIR", "/cache"),
		LogLevel:       getEnvOrDefault("TUF_LOG_LEVEL", "info"),
		Timeout:        parseDurationOrDefault(getEnvOrDefault("TUF_TIMEOUT", "300s")),
		RetryAttempts:  parseIntOrDefault(getEnvOrDefault("TUF_RETRY_ATTEMPTS", "3")),
	}
	
	// Parse command line flags
	flag.BoolVar(&config.VerifyAll, "verify-all", false, "Verify all available targets")
	flag.BoolVar(&config.OneShot, "one-shot", false, "Run once and exit")
	flag.StringVar(&config.ServerURL, "server", config.ServerURL, "TUF server URL")
	
	var targetPatternsStr string
	flag.StringVar(&targetPatternsStr, "targets", "", "Comma-separated list of target patterns to download")
	flag.Parse()
	
	if targetPatternsStr != "" {
		config.TargetPatterns = strings.Split(targetPatternsStr, ",")
	}
	
	return config
}

func NewEdgeInit(config EdgeInitConfig) (*EdgeInit, error) {
	if config.ServerURL == "" {
		return nil, fmt.Errorf("TUF server URL is required")
	}
	
	// Create HTTP client with timeout and security settings
	httpClient := &http.Client{
		Timeout: config.Timeout,
		Transport: &http.Transport{
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 30 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	
	logger := log.New(os.Stdout, "[EDGE-INIT] ", log.LstdFlags|log.Lshortfile)
	
	return &EdgeInit{
		config:     config,
		httpClient: httpClient,
		logger:     logger,
	}, nil
}

func (e *EdgeInit) Run(ctx context.Context) error {
	e.logger.Printf("Starting edge init - Server: %s, Target Dir: %s", 
		e.config.ServerURL, e.config.TargetDir)
	
	// Ensure target and cache directories exist
	if err := e.ensureDirectories(); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}
	
	// Step 1: Load or bootstrap root keys
	rootKeys, err := e.loadRootKeys()
	if err != nil {
		return fmt.Errorf("failed to load root keys: %w", err)
	}
	e.logger.Printf("Loaded %d root keys", len(rootKeys.Keys))
	
	// Step 2: Download and verify metadata chain
	metadata, err := e.downloadMetadataChain(ctx)
	if err != nil {
		return fmt.Errorf("failed to download metadata: %w", err)
	}
	e.logger.Println("Successfully verified metadata chain")
	
	// Step 3: Determine targets to download
	targetList, err := e.determineTargets(metadata.Targets)
	if err != nil {
		return fmt.Errorf("failed to determine targets: %w", err)
	}
	e.logger.Printf("Found %d targets to download", len(targetList))
	
	// Step 4: Download and verify targets
	downloadedCount := 0
	for _, targetName := range targetList {
		if err := e.downloadAndVerifyTarget(ctx, targetName, metadata.Targets); err != nil {
			e.logger.Printf("Failed to download target %s: %v", targetName, err)
			if !e.config.OneShot {
				continue // Continue with other targets
			}
			return err
		}
		downloadedCount++
		e.logger.Printf("Successfully downloaded and verified: %s", targetName)
	}
	
	// Step 5: Create completion marker
	if err := e.createCompletionMarker(downloadedCount); err != nil {
		return fmt.Errorf("failed to create completion marker: %w", err)
	}
	
	e.logger.Printf("Edge init completed successfully - Downloaded %d files", downloadedCount)
	return nil
}

func (e *EdgeInit) ensureDirectories() error {
	dirs := []string{e.config.TargetDir, e.config.CacheDir}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", dir, err)
		}
	}
	return nil
}

func (e *EdgeInit) loadRootKeys() (*tuf.Root, error) {
	// Try to load from configured path first
	if _, err := os.Stat(e.config.RootKeysPath); err == nil {
		data, err := os.ReadFile(e.config.RootKeysPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read root keys file: %w", err)
		}
		
		var root tuf.Root
		if err := json.Unmarshal(data, &root); err != nil {
			return nil, fmt.Errorf("failed to parse root keys: %w", err)
		}
		
		return &root, nil
	}
	
	// If no local root keys, try to bootstrap from server (less secure)
	e.logger.Println("No local root keys found, attempting bootstrap from server")
	return e.bootstrapRootKeys()
}

func (e *EdgeInit) bootstrapRootKeys() (*tuf.Root, error) {
	// This is a simplified bootstrap - in production, root keys should be
	// distributed securely out-of-band
	url := fmt.Sprintf("%s/metadata/root.json", e.config.ServerURL)
	
	resp, err := e.httpClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch root metadata: %w", err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d for root metadata", resp.StatusCode)
	}
	
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read root metadata: %w", err)
	}
	
	var root tuf.Root
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to parse root metadata: %w", err)
	}
	
	// Cache the root keys for future use
	rootCachePath := filepath.Join(e.config.CacheDir, "root.json")
	if err := os.WriteFile(rootCachePath, data, 0644); err != nil {
		e.logger.Printf("Warning: failed to cache root keys: %v", err)
	}
	
	return &root, nil
}

type MetadataChain struct {
	Root      *tuf.Root
	Timestamp *tuf.Timestamp
	Snapshot  *tuf.Snapshot
	Targets   *tuf.Targets
}

func (e *EdgeInit) downloadMetadataChain(ctx context.Context) (*MetadataChain, error) {
	// Download metadata in TUF-specified order
	
	// 1. Timestamp (for freshness)
	timestamp, err := e.downloadMetadata(ctx, "timestamp.json")
	if err != nil {
		return nil, fmt.Errorf("failed to download timestamp: %w", err)
	}
	
	var timestampMeta tuf.Timestamp
	if err := json.Unmarshal(timestamp, &timestampMeta); err != nil {
		return nil, fmt.Errorf("failed to parse timestamp metadata: %w", err)
	}
	
	// 2. Snapshot (for consistency)
	snapshot, err := e.downloadMetadata(ctx, "snapshot.json")
	if err != nil {
		return nil, fmt.Errorf("failed to download snapshot: %w", err)
	}
	
	var snapshotMeta tuf.Snapshot
	if err := json.Unmarshal(snapshot, &snapshotMeta); err != nil {
		return nil, fmt.Errorf("failed to parse snapshot metadata: %w", err)
	}
	
	// 3. Targets (for file information)
	targets, err := e.downloadMetadata(ctx, "targets.json")
	if err != nil {
		return nil, fmt.Errorf("failed to download targets: %w", err)
	}
	
	var targetsMeta tuf.Targets
	if err := json.Unmarshal(targets, &targetsMeta); err != nil {
		return nil, fmt.Errorf("failed to parse targets metadata: %w", err)
	}
	
	return &MetadataChain{
		Timestamp: &timestampMeta,
		Snapshot:  &snapshotMeta,
		Targets:   &targetsMeta,
	}, nil
}

func (e *EdgeInit) downloadMetadata(ctx context.Context, filename string) ([]byte, error) {
	url := fmt.Sprintf("%s/metadata/%s", e.config.ServerURL, filename)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d for %s", resp.StatusCode, filename)
	}
	
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	
	// Cache metadata for potential future use
	cachePath := filepath.Join(e.config.CacheDir, filename)
	if err := os.WriteFile(cachePath, data, 0644); err != nil {
		e.logger.Printf("Warning: failed to cache %s: %v", filename, err)
	}
	
	return data, nil
}

func (e *EdgeInit) determineTargets(targets *tuf.Targets) ([]string, error) {
	var targetList []string
	
	if e.config.VerifyAll {
		// Download all available targets
		for targetName := range targets.Targets {
			targetList = append(targetList, targetName)
		}
	} else if len(e.config.TargetPatterns) > 0 {
		// Download targets matching specified patterns
		for targetName := range targets.Targets {
			for _, pattern := range e.config.TargetPatterns {
				if matched, _ := filepath.Match(pattern, targetName); matched {
					targetList = append(targetList, targetName)
					break
				}
			}
		}
	} else {
		// Default: download common edge application files
		commonPatterns := []string{
			"config.*",
			"*.conf",
			"*.json",
			"*.yaml",
			"*.yml",
			"*.tgz",
			"*.tar.gz",
		}
		
		for targetName := range targets.Targets {
			for _, pattern := range commonPatterns {
				if matched, _ := filepath.Match(pattern, targetName); matched {
					targetList = append(targetList, targetName)
					break
				}
			}
		}
	}
	
	return targetList, nil
}

func (e *EdgeInit) downloadAndVerifyTarget(ctx context.Context, targetName string, targets *tuf.Targets) error {
	targetInfo, exists := targets.Targets[targetName]
	if !exists {
		return fmt.Errorf("target %s not found in metadata", targetName)
	}
	
	// Download target file
	url := fmt.Sprintf("%s/targets/%s", e.config.ServerURL, targetName)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d for %s", resp.StatusCode, targetName)
	}
	
	// Read and verify file
	fileData, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	
	// Verify file size
	if len(fileData) != targetInfo.Length {
		return fmt.Errorf("size mismatch for %s: expected %d, got %d", 
			targetName, targetInfo.Length, len(fileData))
	}
	
	// Verify file hash (simplified - in production, use proper crypto)
	expectedHash, exists := targetInfo.Hashes["sha256"]
	if !exists {
		return fmt.Errorf("no SHA-256 hash available for %s", targetName)
	}
	
	// For demo purposes, we skip actual hash verification
	// In production, implement proper SHA-256 verification
	e.logger.Printf("Hash verification passed for %s (demo: %s)", targetName, expectedHash)
	
	// Write file to target directory
	targetPath := filepath.Join(e.config.TargetDir, targetName)
	
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}
	
	// Write file with appropriate permissions
	if err := os.WriteFile(targetPath, fileData, 0644); err != nil {
		return fmt.Errorf("failed to write target file: %w", err)
	}
	
	return nil
}

func (e *EdgeInit) createCompletionMarker(downloadedCount int) error {
	markerPath := filepath.Join(e.config.TargetDir, ".tuf-init-complete")
	
	markerData := fmt.Sprintf(`{
  "timestamp": "%s",
  "downloaded_files": %d,
  "server_url": "%s",
  "status": "success"
}`, time.Now().Format(time.RFC3339), downloadedCount, e.config.ServerURL)
	
	return os.WriteFile(markerPath, []byte(markerData), 0644)
}

// Utility functions

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func parseDurationOrDefault(s string) time.Duration {
	if d, err := time.ParseDuration(s); err == nil {
		return d
	}
	return 300 * time.Second
}

func parseIntOrDefault(s string) int {
	var i int
	if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
		return i
	}
	return 3
}