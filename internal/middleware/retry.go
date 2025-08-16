package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"tuf-golang-project/internal/logger"
	"tuf-golang-project/internal/retry"
)

// RetryConfig holds retry middleware configuration
type RetryConfig struct {
	Enabled        bool          `json:"enabled"`
	MaxRetries     int           `json:"max_retries"`
	InitialDelay   time.Duration `json:"initial_delay"`
	MaxDelay       time.Duration `json:"max_delay"`
	Multiplier     float64       `json:"multiplier"`
	JitterFraction float64       `json:"jitter_fraction"`
	// Endpoints that should have retry enabled
	RetryEndpoints []string `json:"retry_endpoints"`
}

// RetryMiddleware adds retry capability to outbound requests
func RetryMiddleware(config *RetryConfig) gin.HandlerFunc {
	if config == nil {
		config = &RetryConfig{
			Enabled:        false,
			MaxRetries:     3,
			InitialDelay:   100 * time.Millisecond,
			MaxDelay:       10 * time.Second,
			Multiplier:     2.0,
			JitterFraction: 0.1,
		}
	}

	retrier := retry.NewRetrier(&retry.Config{
		MaxRetries:     config.MaxRetries,
		InitialDelay:   config.InitialDelay,
		MaxDelay:       config.MaxDelay,
		Multiplier:     config.Multiplier,
		JitterFraction: config.JitterFraction,
	})

	return func(c *gin.Context) {
		// Only apply retry logic if enabled
		if !config.Enabled {
			c.Next()
			return
		}

		// Check if this endpoint should have retry logic
		if !shouldRetryEndpoint(c.Request.URL.Path, config.RetryEndpoints) {
			c.Next()
			return
		}

		// Store the retrier in context for handlers to use
		c.Set("retrier", retrier)
		
		// Add retry info to response headers
		c.Header("X-Retry-Enabled", "true")
		c.Header("X-Max-Retries", string(rune(config.MaxRetries)))

		c.Next()
	}
}

// shouldRetryEndpoint checks if an endpoint should have retry logic
func shouldRetryEndpoint(path string, retryEndpoints []string) bool {
	if len(retryEndpoints) == 0 {
		// If no specific endpoints configured, apply to all
		return true
	}

	for _, endpoint := range retryEndpoints {
		if matchPath(path, endpoint) {
			return true
		}
	}

	return false
}

// matchPath matches a path against a pattern (simple glob support)
func matchPath(path, pattern string) bool {
	// Simple implementation - can be enhanced with proper glob matching
	if pattern == "*" {
		return true
	}
	
	// Check for prefix match with wildcard
	if len(pattern) > 0 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(path) >= len(prefix) && path[:len(prefix)] == prefix
	}
	
	return path == pattern
}

// RetryableResponseWriter wraps response writer to capture status for retry decisions
type RetryableResponseWriter struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
}

func (w *RetryableResponseWriter) Write(data []byte) (int, error) {
	w.body.Write(data)
	return w.ResponseWriter.Write(data)
}

func (w *RetryableResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

// RetryHTTPClient provides an HTTP client with retry capabilities
type RetryHTTPClient struct {
	client  *http.Client
	retrier *retry.Retrier
}

// NewRetryHTTPClient creates a new HTTP client with retry capabilities
func NewRetryHTTPClient(retryConfig *retry.Config, httpTimeout time.Duration) *RetryHTTPClient {
	if retryConfig == nil {
		retryConfig = retry.DefaultConfig()
	}

	return &RetryHTTPClient{
		client: &http.Client{
			Timeout: httpTimeout,
		},
		retrier: retry.NewRetrier(retryConfig),
	}
}

// Do performs an HTTP request with retry logic
func (c *RetryHTTPClient) Do(ctx context.Context, req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var lastErr error

	// Clone the request body if present (as it can only be read once)
	var bodyBytes []byte
	if req.Body != nil {
		bodyBytes, _ = io.ReadAll(req.Body)
		req.Body.Close()
	}

	err := c.retrier.DoWithName(ctx, req.URL.String(), func() error {
		// Reset body for each retry
		if bodyBytes != nil {
			req.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		}

		var err error
		resp, err = c.client.Do(req)
		if err != nil {
			lastErr = err
			return &retry.RetryableError{Err: err}
		}

		// Check if response indicates we should retry
		if shouldRetryResponse(resp) {
			// Read and close the response body
			if resp.Body != nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
			lastErr = &retry.RetryableError{
				Err: &HTTPError{
					StatusCode: resp.StatusCode,
					Status:     resp.Status,
				},
			}
			return lastErr
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	return resp, nil
}

// HTTPError represents an HTTP error
type HTTPError struct {
	StatusCode int
	Status     string
}

func (e *HTTPError) Error() string {
	return e.Status
}

// shouldRetryResponse determines if an HTTP response warrants a retry
func shouldRetryResponse(resp *http.Response) bool {
	if resp == nil {
		return true
	}

	// Retry on 5xx errors (server errors)
	if resp.StatusCode >= 500 && resp.StatusCode < 600 {
		return true
	}

	// Retry on specific 4xx errors
	switch resp.StatusCode {
	case http.StatusTooManyRequests, // 429
	     http.StatusRequestTimeout:   // 408
		return true
	}

	return false
}

// GetRetrier gets the retrier from the Gin context
func GetRetrier(c *gin.Context) *retry.Retrier {
	if val, exists := c.Get("retrier"); exists {
		if retrier, ok := val.(*retry.Retrier); ok {
			return retrier
		}
	}
	return nil
}

// WithRetry wraps a handler function with retry logic
func WithRetry(c *gin.Context, name string, fn func() error) error {
	retrier := GetRetrier(c)
	if retrier == nil {
		// No retrier available, execute without retry
		return fn()
	}

	ctx := c.Request.Context()
	return retrier.DoWithName(ctx, name, fn)
}

// RetryStats provides middleware for tracking retry statistics
func RetryStats() gin.HandlerFunc {
	return func(c *gin.Context) {
		startTime := time.Now()
		
		// Track retry attempts in context
		c.Set("retry_attempts", 0)
		
		c.Next()
		
		// Log retry statistics if any retries occurred
		if attempts, exists := c.Get("retry_attempts"); exists {
			if attemptCount := attempts.(int); attemptCount > 1 {
				duration := time.Since(startTime)
				logger.Logger.Info("Request completed with retries",
					"path", c.Request.URL.Path,
					"attempts", attemptCount,
					"duration", duration,
					"status", c.Writer.Status())
			}
		}
	}
}