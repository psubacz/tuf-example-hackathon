package retry

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"time"

	"tuf-golang-project/internal/logger"
)

// Config holds retry configuration
type Config struct {
	MaxRetries     int           `json:"max_retries"`
	InitialDelay   time.Duration `json:"initial_delay"`
	MaxDelay       time.Duration `json:"max_delay"`
	Multiplier     float64       `json:"multiplier"`
	JitterFraction float64       `json:"jitter_fraction"`
	RetryableErrors []string     `json:"retryable_errors"`
}

// DefaultConfig returns default retry configuration
func DefaultConfig() *Config {
	return &Config{
		MaxRetries:     3,
		InitialDelay:   100 * time.Millisecond,
		MaxDelay:       30 * time.Second,
		Multiplier:     2.0,
		JitterFraction: 0.1,
	}
}

// Retrier provides retry functionality with exponential backoff
type Retrier struct {
	config *Config
	rand   *rand.Rand
}

// NewRetrier creates a new retrier with the given configuration
func NewRetrier(config *Config) *Retrier {
	if config == nil {
		config = DefaultConfig()
	}
	
	// Validate configuration
	if config.MaxRetries < 0 {
		config.MaxRetries = 0
	}
	if config.Multiplier <= 1 {
		config.Multiplier = 2.0
	}
	if config.JitterFraction < 0 || config.JitterFraction > 1 {
		config.JitterFraction = 0.1
	}
	
	return &Retrier{
		config: config,
		rand:   rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// RetryFunc is a function that can be retried
type RetryFunc func() error

// RetryableError represents an error that can be retried
type RetryableError struct {
	Err error
}

func (e *RetryableError) Error() string {
	return e.Err.Error()
}

// IsRetryable checks if an error should trigger a retry
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}
	
	// Check if it's explicitly marked as retryable
	var retryableErr *RetryableError
	if errors.As(err, &retryableErr) {
		return true
	}
	
	// Check for specific error types that are typically retryable
	errStr := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"too many requests",
		"service unavailable",
		"gateway timeout",
		"bad gateway",
	}
	
	for _, pattern := range retryablePatterns {
		if contains(errStr, pattern) {
			return true
		}
	}
	
	return false
}

// Do executes the given function with retry logic
func (r *Retrier) Do(ctx context.Context, fn RetryFunc) error {
	return r.DoWithName(ctx, "operation", fn)
}

// DoWithName executes the given function with retry logic and a descriptive name
func (r *Retrier) DoWithName(ctx context.Context, name string, fn RetryFunc) error {
	var lastErr error
	
	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Check context before attempting
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("context cancelled: %w", err)
		}
		
		// Execute the function
		err := fn()
		
		// Success
		if err == nil {
			if attempt > 0 {
				logger.Logger.Info("Retry succeeded",
					"name", name,
					"attempt", attempt+1,
					"total_attempts", attempt+1)
			}
			return nil
		}
		
		lastErr = err
		
		// Check if we should retry
		if !IsRetryable(err) {
			logger.Logger.Debug("Error is not retryable",
				"name", name,
				"error", err)
			return err
		}
		
		// Check if we've exhausted retries
		if attempt >= r.config.MaxRetries {
			logger.Logger.Warn("Max retries exhausted",
				"name", name,
				"attempts", attempt+1,
				"error", err)
			break
		}
		
		// Calculate delay with exponential backoff
		delay := r.calculateDelay(attempt)
		
		logger.Logger.Info("Retrying after delay",
			"name", name,
			"attempt", attempt+1,
			"delay", delay,
			"error", err)
		
		// Wait before retrying
		select {
		case <-time.After(delay):
			// Continue to next attempt
		case <-ctx.Done():
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		}
	}
	
	return fmt.Errorf("operation failed after %d attempts: %w", r.config.MaxRetries+1, lastErr)
}

// calculateDelay calculates the delay for the given attempt number
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	// Calculate base delay with exponential backoff
	baseDelay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt))
	
	// Cap at max delay
	if baseDelay > float64(r.config.MaxDelay) {
		baseDelay = float64(r.config.MaxDelay)
	}
	
	// Add jitter to avoid thundering herd
	jitterRange := baseDelay * r.config.JitterFraction
	jitter := (r.rand.Float64() * 2 - 1) * jitterRange // Random between -jitterRange and +jitterRange
	
	finalDelay := baseDelay + jitter
	if finalDelay < 0 {
		finalDelay = 0
	}
	
	return time.Duration(finalDelay)
}

// WithMaxRetries returns a new retrier with the specified max retries
func (r *Retrier) WithMaxRetries(maxRetries int) *Retrier {
	newConfig := *r.config
	newConfig.MaxRetries = maxRetries
	return NewRetrier(&newConfig)
}

// WithDelay returns a new retrier with the specified initial delay
func (r *Retrier) WithDelay(initialDelay time.Duration) *Retrier {
	newConfig := *r.config
	newConfig.InitialDelay = initialDelay
	return NewRetrier(&newConfig)
}

// WithMultiplier returns a new retrier with the specified multiplier
func (r *Retrier) WithMultiplier(multiplier float64) *Retrier {
	newConfig := *r.config
	newConfig.Multiplier = multiplier
	return NewRetrier(&newConfig)
}

// WithJitter returns a new retrier with the specified jitter fraction
func (r *Retrier) WithJitter(jitterFraction float64) *Retrier {
	newConfig := *r.config
	newConfig.JitterFraction = jitterFraction
	return NewRetrier(&newConfig)
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr || 
		len(s) > len(substr) && containsInMiddle(s, substr)
}

func containsInMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// RetryClient wraps HTTP or other clients with retry logic
type RetryClient struct {
	retrier *Retrier
}

// NewRetryClient creates a new retry client
func NewRetryClient(config *Config) *RetryClient {
	return &RetryClient{
		retrier: NewRetrier(config),
	}
}

// Execute wraps a function with retry logic
func (c *RetryClient) Execute(ctx context.Context, name string, fn func() error) error {
	return c.retrier.DoWithName(ctx, name, fn)
}

// ExecuteWithResult wraps a function that returns a result with retry logic
func (c *RetryClient) ExecuteWithResult(ctx context.Context, name string, fn func() (interface{}, error)) (interface{}, error) {
	var result interface{}
	
	err := c.retrier.DoWithName(ctx, name, func() error {
		var err error
		result, err = fn()
		return err
	})
	
	return result, err
}

// Middleware provides retry middleware for HTTP handlers
type Middleware struct {
	retrier *Retrier
}

// NewMiddleware creates retry middleware
func NewMiddleware(config *Config) *Middleware {
	return &Middleware{
		retrier: NewRetrier(config),
	}
}

// Stats tracks retry statistics
type Stats struct {
	TotalAttempts   int64
	SuccessfulRetries int64
	FailedRetries   int64
	TotalDelay      time.Duration
}

// GlobalStats provides global retry statistics
var globalStats Stats

// GetStats returns current retry statistics
func GetStats() Stats {
	return globalStats
}