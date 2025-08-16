package storage

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FilesystemBackend implements storage backend for local filesystem
type FilesystemBackend struct {
	basePath string
}

// NewFilesystemBackend creates a new filesystem storage backend
func NewFilesystemBackend(config Config) (Backend, error) {
	basePath, ok := config.Properties["base_path"].(string)
	if !ok || basePath == "" {
		basePath = "./storage"
	}
	
	// Ensure base path exists
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create base path: %w", err)
	}
	
	// Convert to absolute path
	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	
	return &FilesystemBackend{
		basePath: absPath,
	}, nil
}

// Get retrieves a file from filesystem
func (fs *FilesystemBackend) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	fullPath := fs.fullPath(path)
	
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ErrNotFound{Path: path}
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	
	return file, nil
}

// GetWithInfo retrieves a file with metadata
func (fs *FilesystemBackend) GetWithInfo(ctx context.Context, path string) (*Object, error) {
	fullPath := fs.fullPath(path)
	
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ErrNotFound{Path: path}
		}
		return nil, fmt.Errorf("failed to open file: %w", err)
	}
	
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	
	// Calculate ETag (using modification time and size)
	etag := fs.calculateETag(info)
	
	return &Object{
		ReadCloser: file,
		Info: ObjectInfo{
			Path:         path,
			Size:         info.Size(),
			LastModified: info.ModTime(),
			ETag:         etag,
			ContentType:  getContentType(path),
			Metadata:     make(map[string]string),
		},
	}, nil
}

// Put stores a file in filesystem
func (fs *FilesystemBackend) Put(ctx context.Context, path string, reader io.Reader) error {
	fullPath := fs.fullPath(path)
	
	// Ensure directory exists
	dir := filepath.Dir(fullPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Create temporary file first
	tmpFile, err := os.CreateTemp(dir, ".tmp-")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()
	
	// Copy content to temporary file
	if _, err := io.Copy(tmpFile, reader); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}
	
	// Sync to disk
	if err := tmpFile.Sync(); err != nil {
		return fmt.Errorf("failed to sync file: %w", err)
	}
	tmpFile.Close()
	
	// Atomic rename
	if err := os.Rename(tmpFile.Name(), fullPath); err != nil {
		return fmt.Errorf("failed to rename file: %w", err)
	}
	
	return nil
}

// PutWithMetadata stores a file with metadata (metadata stored as extended attributes on supported systems)
func (fs *FilesystemBackend) PutWithMetadata(ctx context.Context, path string, reader io.Reader, metadata map[string]string) error {
	// First, store the file
	if err := fs.Put(ctx, path, reader); err != nil {
		return err
	}
	
	// Store metadata as a sidecar file (for cross-platform compatibility)
	if len(metadata) > 0 {
		metaPath := fs.fullPath(path + ".meta")
		metaFile, err := os.Create(metaPath)
		if err != nil {
			return fmt.Errorf("failed to create metadata file: %w", err)
		}
		defer metaFile.Close()
		
		for key, value := range metadata {
			fmt.Fprintf(metaFile, "%s=%s\n", key, value)
		}
	}
	
	return nil
}

// Delete removes a file from filesystem
func (fs *FilesystemBackend) Delete(ctx context.Context, path string) error {
	fullPath := fs.fullPath(path)
	
	if err := os.Remove(fullPath); err != nil {
		if os.IsNotExist(err) {
			return &ErrNotFound{Path: path}
		}
		return fmt.Errorf("failed to delete file: %w", err)
	}
	
	// Also remove metadata file if it exists
	os.Remove(fullPath + ".meta")
	
	return nil
}

// Exists checks if a file exists
func (fs *FilesystemBackend) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := fs.fullPath(path)
	
	_, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("failed to stat file: %w", err)
	}
	
	return true, nil
}

// List lists files in a directory
func (fs *FilesystemBackend) List(ctx context.Context, prefix string) ([]string, error) {
	basePath := fs.fullPath(prefix)
	
	var files []string
	err := filepath.Walk(basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			// If directory doesn't exist, return empty list
			if os.IsNotExist(err) {
				return filepath.SkipDir
			}
			return err
		}
		
		if !info.IsDir() && !strings.HasSuffix(path, ".meta") {
			// Convert to relative path
			relPath, err := filepath.Rel(fs.basePath, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(relPath))
		}
		
		return nil
	})
	
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("failed to list files: %w", err)
	}
	
	return files, nil
}

// ListWithInfo lists files with metadata
func (fs *FilesystemBackend) ListWithInfo(ctx context.Context, prefix string) ([]*Object, error) {
	files, err := fs.List(ctx, prefix)
	if err != nil {
		return nil, err
	}
	
	var objects []*Object
	for _, file := range files {
		obj, err := fs.GetWithInfo(ctx, file)
		if err != nil {
			// Skip files that can't be read
			continue
		}
		obj.Close() // We only need the info, not the content
		objects = append(objects, obj)
	}
	
	return objects, nil
}

// Stat gets file metadata without downloading
func (fs *FilesystemBackend) Stat(ctx context.Context, path string) (*ObjectInfo, error) {
	fullPath := fs.fullPath(path)
	
	info, err := os.Stat(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &ErrNotFound{Path: path}
		}
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}
	
	etag := fs.calculateETag(info)
	
	// Load metadata if exists
	metadata := make(map[string]string)
	if metaData, err := os.ReadFile(fullPath + ".meta"); err == nil {
		lines := strings.Split(string(metaData), "\n")
		for _, line := range lines {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				metadata[parts[0]] = parts[1]
			}
		}
	}
	
	return &ObjectInfo{
		Path:         path,
		Size:         info.Size(),
		LastModified: info.ModTime(),
		ETag:         etag,
		ContentType:  getContentType(path),
		Metadata:     metadata,
	}, nil
}

// Copy copies a file within the filesystem
func (fs *FilesystemBackend) Copy(ctx context.Context, src, dst string) error {
	srcPath := fs.fullPath(src)
	dstPath := fs.fullPath(dst)
	
	// Open source file
	srcFile, err := os.Open(srcPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ErrNotFound{Path: src}
		}
		return fmt.Errorf("failed to open source file: %w", err)
	}
	defer srcFile.Close()
	
	// Ensure destination directory exists
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Create destination file
	dstFile, err := os.Create(dstPath)
	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}
	defer dstFile.Close()
	
	// Copy content
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy file: %w", err)
	}
	
	// Copy metadata if exists
	if srcMeta, err := os.ReadFile(srcPath + ".meta"); err == nil {
		_ = os.WriteFile(dstPath+".meta", srcMeta, 0644)
	}
	
	return dstFile.Sync()
}

// Move moves a file within the filesystem
func (fs *FilesystemBackend) Move(ctx context.Context, src, dst string) error {
	srcPath := fs.fullPath(src)
	dstPath := fs.fullPath(dst)
	
	// Ensure destination directory exists
	dstDir := filepath.Dir(dstPath)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}
	
	// Try atomic rename first
	if err := os.Rename(srcPath, dstPath); err != nil {
		// If rename fails (e.g., across filesystems), fall back to copy and delete
		if err := fs.Copy(ctx, src, dst); err != nil {
			return err
		}
		return fs.Delete(ctx, src)
	}
	
	// Also move metadata file if it exists
	_ = os.Rename(srcPath+".meta", dstPath+".meta")
	
	return nil
}

// GetURL generates a file:// URL for local filesystem
func (fs *FilesystemBackend) GetURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	fullPath := fs.fullPath(path)
	
	// Check if file exists
	if _, err := os.Stat(fullPath); err != nil {
		if os.IsNotExist(err) {
			return "", &ErrNotFound{Path: path}
		}
		return "", fmt.Errorf("failed to stat file: %w", err)
	}
	
	// Return file:// URL
	return "file://" + fullPath, nil
}

// SupportsDirectURL returns true for filesystem backend
func (fs *FilesystemBackend) SupportsDirectURL() bool {
	return true
}

// Close closes the filesystem backend (no-op for filesystem)
func (fs *FilesystemBackend) Close() error {
	return nil
}

// Helper methods

func (fs *FilesystemBackend) fullPath(path string) string {
	// Clean the path and join with base path
	clean := filepath.Clean(path)
	// Remove leading slash if present
	clean = strings.TrimPrefix(clean, "/")
	return filepath.Join(fs.basePath, clean)
}

func (fs *FilesystemBackend) calculateETag(info os.FileInfo) string {
	// Simple ETag based on size and modification time
	data := fmt.Sprintf("%d-%d", info.Size(), info.ModTime().Unix())
	hash := md5.Sum([]byte(data))
	return hex.EncodeToString(hash[:])
}

func getContentType(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".json":
		return "application/json"
	case ".txt":
		return "text/plain"
	case ".html":
		return "text/html"
	case ".xml":
		return "application/xml"
	case ".tar":
		return "application/x-tar"
	case ".gz":
		return "application/gzip"
	case ".zip":
		return "application/zip"
	default:
		return "application/octet-stream"
	}
}

func init() {
	// Register filesystem backend
	Register("filesystem", NewFilesystemBackend)
}