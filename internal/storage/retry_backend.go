package storage

import (
	"context"
	"fmt"
	"io"
	"time"

	"tuf-golang-project/internal/logger"
	"tuf-golang-project/internal/retry"
)

// RetryBackend wraps a storage backend with retry capabilities
type RetryBackend struct {
	backend Backend
	retrier *retry.Retrier
}

// NewRetryBackend creates a new storage backend with retry capabilities
func NewRetryBackend(backend Backend, config *retry.Config) Backend {
	if config == nil {
		config = retry.DefaultConfig()
	}

	return &RetryBackend{
		backend: backend,
		retrier: retry.NewRetrier(config),
	}
}

// Get retrieves a file with retry logic
func (r *RetryBackend) Get(ctx context.Context, path string) (io.ReadCloser, error) {
	var reader io.ReadCloser
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.Get(%s)", path), func() error {
		var err error
		reader, err = r.backend.Get(ctx, path)
		if err != nil {
			// Check if error is retryable
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage Get failed after retries", "path", path, "error", err)
		return nil, err
	}

	return reader, nil
}

// GetWithInfo retrieves a file with metadata with retry logic
func (r *RetryBackend) GetWithInfo(ctx context.Context, path string) (*Object, error) {
	var obj *Object
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.GetWithInfo(%s)", path), func() error {
		var err error
		obj, err = r.backend.GetWithInfo(ctx, path)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage GetWithInfo failed after retries", "path", path, "error", err)
		return nil, err
	}

	return obj, nil
}

// Put stores a file with retry logic
func (r *RetryBackend) Put(ctx context.Context, path string, reader io.Reader) error {
	// For Put operations, we need to be careful about retrying
	// as we might have already partially written data
	// Only retry on clearly transient errors
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.Put(%s)", path), func() error {
		err := r.backend.Put(ctx, path, reader)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage Put failed after retries", "path", path, "error", err)
		return err
	}

	return nil
}

// PutWithMetadata stores a file with metadata with retry logic
func (r *RetryBackend) PutWithMetadata(ctx context.Context, path string, reader io.Reader, metadata map[string]string) error {
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.PutWithMetadata(%s)", path), func() error {
		err := r.backend.PutWithMetadata(ctx, path, reader, metadata)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage PutWithMetadata failed after retries", "path", path, "error", err)
		return err
	}

	return nil
}

// Delete removes a file with retry logic
func (r *RetryBackend) Delete(ctx context.Context, path string) error {
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.Delete(%s)", path), func() error {
		err := r.backend.Delete(ctx, path)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage Delete failed after retries", "path", path, "error", err)
		return err
	}

	return nil
}

// Exists checks if a file exists with retry logic
func (r *RetryBackend) Exists(ctx context.Context, path string) (bool, error) {
	var exists bool
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.Exists(%s)", path), func() error {
		var err error
		exists, err = r.backend.Exists(ctx, path)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage Exists failed after retries", "path", path, "error", err)
		return false, err
	}

	return exists, nil
}

// List lists files with retry logic
func (r *RetryBackend) List(ctx context.Context, prefix string) ([]string, error) {
	var files []string
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.List(%s)", prefix), func() error {
		var err error
		files, err = r.backend.List(ctx, prefix)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage List failed after retries", "prefix", prefix, "error", err)
		return nil, err
	}

	return files, nil
}

// ListWithInfo lists files with metadata with retry logic
func (r *RetryBackend) ListWithInfo(ctx context.Context, prefix string) ([]*Object, error) {
	var objects []*Object
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.ListWithInfo(%s)", prefix), func() error {
		var err error
		objects, err = r.backend.ListWithInfo(ctx, prefix)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage ListWithInfo failed after retries", "prefix", prefix, "error", err)
		return nil, err
	}

	return objects, nil
}

// Stat gets file metadata with retry logic
func (r *RetryBackend) Stat(ctx context.Context, path string) (*ObjectInfo, error) {
	var info *ObjectInfo
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.Stat(%s)", path), func() error {
		var err error
		info, err = r.backend.Stat(ctx, path)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage Stat failed after retries", "path", path, "error", err)
		return nil, err
	}

	return info, nil
}

// Copy copies a file with retry logic
func (r *RetryBackend) Copy(ctx context.Context, src, dst string) error {
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.Copy(%s->%s)", src, dst), func() error {
		err := r.backend.Copy(ctx, src, dst)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage Copy failed after retries", "src", src, "dst", dst, "error", err)
		return err
	}

	return nil
}

// Move moves a file with retry logic
func (r *RetryBackend) Move(ctx context.Context, src, dst string) error {
	// Move is more complex as it involves both copy and delete
	// We should be careful about retrying to avoid partial state
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.Move(%s->%s)", src, dst), func() error {
		err := r.backend.Move(ctx, src, dst)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage Move failed after retries", "src", src, "dst", dst, "error", err)
		return err
	}

	return nil
}

// GetURL generates a pre-signed URL with retry logic
func (r *RetryBackend) GetURL(ctx context.Context, path string, expiry time.Duration) (string, error) {
	var url string
	
	err := r.retrier.DoWithName(ctx, fmt.Sprintf("storage.GetURL(%s)", path), func() error {
		var err error
		url, err = r.backend.GetURL(ctx, path, expiry)
		if err != nil {
			if isRetryableStorageError(err) {
				return &retry.RetryableError{Err: err}
			}
			return err
		}
		return nil
	})

	if err != nil {
		logger.Logger.Error("Storage GetURL failed after retries", "path", path, "error", err)
		return "", err
	}

	return url, nil
}

// SupportsDirectURL returns whether the backend supports direct URLs
func (r *RetryBackend) SupportsDirectURL() bool {
	return r.backend.SupportsDirectURL()
}

// Close closes the backend
func (r *RetryBackend) Close() error {
	return r.backend.Close()
}

// isRetryableStorageError determines if a storage error should trigger a retry
func isRetryableStorageError(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry on not found errors
	var notFoundErr *ErrNotFound
	if errorsAs(err, &notFoundErr) {
		return false
	}

	// Check error message for retryable patterns
	errStr := err.Error()
	retryablePatterns := []string{
		"connection",
		"timeout",
		"temporary",
		"unavailable",
		"too many",
		"throttl",
		"rate limit",
		"503",
		"429",
	}

	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}

	return false
}

// Helper function for error type checking
func errorsAs(err error, target interface{}) bool {
	// Simple implementation - can be replaced with errors.As
	switch t := target.(type) {
	case **ErrNotFound:
		if e, ok := err.(*ErrNotFound); ok {
			*t = e
			return true
		}
	}
	return false
}

// Helper function for string matching
func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}