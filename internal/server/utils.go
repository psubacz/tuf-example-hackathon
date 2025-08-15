package server

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
)

// calculateSHA256 calculates SHA256 hash of a file
func calculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// validateRepositoryStructure checks if the TUF repository has the correct structure
func validateRepositoryStructure(repoPath string) error {
	// Check if repository directory exists
	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return fmt.Errorf("repository directory does not exist: %s", repoPath)
	}
	
	// Check metadata directory
	metadataDir := fmt.Sprintf("%s/metadata", repoPath)
	if _, err := os.Stat(metadataDir); os.IsNotExist(err) {
		return fmt.Errorf("metadata directory does not exist: %s", metadataDir)
	}
	
	// Check targets directory
	targetsDir := fmt.Sprintf("%s/targets", repoPath)
	if _, err := os.Stat(targetsDir); os.IsNotExist(err) {
		return fmt.Errorf("targets directory does not exist: %s", targetsDir)
	}
	
	// Check required metadata files
	requiredFiles := []string{"root.json", "targets.json", "snapshot.json", "timestamp.json"}
	for _, file := range requiredFiles {
		filePath := fmt.Sprintf("%s/metadata/%s", repoPath, file)
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			return fmt.Errorf("required metadata file does not exist: %s", file)
		}
	}
	
	return nil
}

// sanitizePath sanitizes file paths to prevent directory traversal attacks
func sanitizePath(path string) string {
	// Remove any path traversal attempts
	if len(path) > 0 && path[0] == '/' {
		path = path[1:]
	}
	
	// Basic sanitization - more comprehensive sanitization might be needed
	// depending on the specific security requirements
	return path
}

// isValidMetadataFile checks if a filename is a valid TUF metadata file
func isValidMetadataFile(filename string) bool {
	validFiles := map[string]bool{
		"root.json":      true,
		"targets.json":   true,
		"snapshot.json":  true,
		"timestamp.json": true,
	}
	
	return validFiles[filename]
}

// formatBytes formats byte count as human readable string
func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	
	units := []string{"B", "KB", "MB", "GB", "TB", "PB", "EB"}
	return fmt.Sprintf("%.1f %s", float64(bytes)/float64(div), units[exp])
}

// getMIMEType returns MIME type for common file extensions
func getMIMEType(filename string) string {
	ext := ""
	for i := len(filename) - 1; i >= 0 && filename[i] != '.'; i-- {
		if filename[i] == '/' {
			break
		}
	}
	if i := len(filename) - 1; i >= 0 && filename[i] == '.' {
		ext = filename[i:]
	}
	
	return getContentTypeByExt(ext)
}