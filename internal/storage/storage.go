package storage

import (
	"context"
	"io"
	"time"
)

// Backend represents a storage backend interface
type Backend interface {
	// Get retrieves a file from storage
	Get(ctx context.Context, path string) (io.ReadCloser, error)
	
	// GetWithInfo retrieves a file with metadata
	GetWithInfo(ctx context.Context, path string) (*Object, error)
	
	// Put stores a file in storage
	Put(ctx context.Context, path string, reader io.Reader) error
	
	// PutWithMetadata stores a file with metadata
	PutWithMetadata(ctx context.Context, path string, reader io.Reader, metadata map[string]string) error
	
	// Delete removes a file from storage
	Delete(ctx context.Context, path string) error
	
	// Exists checks if a file exists
	Exists(ctx context.Context, path string) (bool, error)
	
	// List lists files in a directory
	List(ctx context.Context, prefix string) ([]string, error)
	
	// ListWithInfo lists files with metadata
	ListWithInfo(ctx context.Context, prefix string) ([]*Object, error)
	
	// Stat gets file metadata without downloading
	Stat(ctx context.Context, path string) (*ObjectInfo, error)
	
	// Copy copies a file within the storage
	Copy(ctx context.Context, src, dst string) error
	
	// Move moves a file within the storage
	Move(ctx context.Context, src, dst string) error
	
	// GetURL generates a pre-signed URL for direct access (if supported)
	GetURL(ctx context.Context, path string, expiry time.Duration) (string, error)
	
	// SupportsDirectURL returns true if the backend supports direct URL access
	SupportsDirectURL() bool
	
	// Close closes the storage backend connection
	Close() error
}

// Object represents a stored object with its content and metadata
type Object struct {
	io.ReadCloser
	Info ObjectInfo
}

// ObjectInfo contains metadata about a stored object
type ObjectInfo struct {
	Path         string            `json:"path"`
	Size         int64             `json:"size"`
	LastModified time.Time         `json:"last_modified"`
	ETag         string            `json:"etag"`
	ContentType  string            `json:"content_type"`
	Metadata     map[string]string `json:"metadata"`
}

// Config represents storage backend configuration
type Config struct {
	Type       string                 `json:"type"`       // "filesystem", "s3", "gcs", "azure"
	Properties map[string]interface{} `json:"properties"` // Backend-specific properties
}

// Factory is a function that creates a storage backend
type Factory func(config Config) (Backend, error)

// Registry stores storage backend factories
var registry = make(map[string]Factory)

// Register registers a storage backend factory
func Register(name string, factory Factory) {
	registry[name] = factory
}

// New creates a new storage backend based on configuration
func New(config Config) (Backend, error) {
	factory, ok := registry[config.Type]
	if !ok {
		return nil, &ErrUnsupportedBackend{Type: config.Type}
	}
	return factory(config)
}

// Error types
type ErrUnsupportedBackend struct {
	Type string
}

func (e *ErrUnsupportedBackend) Error() string {
	return "unsupported storage backend: " + e.Type
}

type ErrNotFound struct {
	Path string
}

func (e *ErrNotFound) Error() string {
	return "object not found: " + e.Path
}

type ErrAlreadyExists struct {
	Path string
}

func (e *ErrAlreadyExists) Error() string {
	return "object already exists: " + e.Path
}

type ErrAccessDenied struct {
	Path string
}

func (e *ErrAccessDenied) Error() string {
	return "access denied: " + e.Path
}