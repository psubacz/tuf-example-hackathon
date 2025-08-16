package middleware

import (
	"bytes"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// CoalescedRequest represents a pending request that is being coalesced
type CoalescedRequest struct {
	key        string
	inProgress bool
	result     *CoalescedResult
	waiters    []chan *CoalescedResult
	startTime  time.Time
	mu         sync.Mutex
}

// CoalescedResult holds the result of a coalesced request
type CoalescedResult struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
	Error      error
}

// RequestCoalescer handles request coalescing for identical concurrent requests
type RequestCoalescer struct {
	requests map[string]*CoalescedRequest
	mu       sync.RWMutex
	ttl      time.Duration
	maxWait  time.Duration
}

// NewRequestCoalescer creates a new request coalescer
func NewRequestCoalescer(ttl, maxWait time.Duration) *RequestCoalescer {
	rc := &RequestCoalescer{
		requests: make(map[string]*CoalescedRequest),
		ttl:      ttl,
		maxWait:  maxWait,
	}

	// Start cleanup goroutine
	go rc.cleanup()

	return rc
}

// Middleware returns a Gin middleware for request coalescing
func (rc *RequestCoalescer) Middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only coalesce GET requests
		if c.Request.Method != http.MethodGet {
			c.Next()
			return
		}

		// Generate coalescing key based on request
		key := rc.generateKey(c)

		// Try to get or create a coalesced request
		result, isLeader := rc.getOrCreate(key, c)

		if isLeader {
			// This request is the leader, execute it
			rc.executeLeader(c, key)
		} else {
			// This request is a follower, wait for result
			if result != nil {
				rc.applyResult(c, result)
			} else {
				// Timeout or error occurred
				c.AbortWithStatusJSON(http.StatusServiceUnavailable, gin.H{
					"error": "request coalescing timeout",
				})
			}
		}
	}
}

// generateKey creates a unique key for request coalescing
func (rc *RequestCoalescer) generateKey(c *gin.Context) string {
	// Include method, path, and query parameters in the key
	return fmt.Sprintf("%s:%s:%s", c.Request.Method, c.Request.URL.Path, c.Request.URL.RawQuery)
}

// getOrCreate gets an existing coalesced request or creates a new one
func (rc *RequestCoalescer) getOrCreate(key string, c *gin.Context) (*CoalescedResult, bool) {
	rc.mu.Lock()
	defer rc.mu.Unlock()

	// Check if request already exists
	if req, exists := rc.requests[key]; exists {
		// Add tracing
		if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
			span.SetAttributes(
				attribute.String("coalesce.action", "join"),
				attribute.String("coalesce.key", key),
				attribute.Int("coalesce.waiters", len(req.waiters)),
			)
		}

		// Request is already in progress, wait for result
		waiter := make(chan *CoalescedResult, 1)
		req.mu.Lock()
		req.waiters = append(req.waiters, waiter)
		req.mu.Unlock()

		// Wait for result with timeout
		select {
		case result := <-waiter:
			return result, false
		case <-time.After(rc.maxWait):
			return nil, false
		case <-c.Request.Context().Done():
			return nil, false
		}
	}

	// Create new coalesced request
	req := &CoalescedRequest{
		key:        key,
		inProgress: true,
		waiters:    make([]chan *CoalescedResult, 0),
		startTime:  time.Now(),
	}
	rc.requests[key] = req

	// Add tracing
	if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
		span.SetAttributes(
			attribute.String("coalesce.action", "lead"),
			attribute.String("coalesce.key", key),
		)
	}

	return nil, true
}

// executeLeader executes the leader request and distributes results
func (rc *RequestCoalescer) executeLeader(c *gin.Context, key string) {
	// Create a response recorder to capture the response
	recorder := &responseRecorder{
		ResponseWriter: c.Writer,
		body:           &bytes.Buffer{},
		headers:        make(http.Header),
	}
	c.Writer = recorder

	// Execute the actual handler
	c.Next()

	// Create result from recorded response
	result := &CoalescedResult{
		StatusCode: recorder.statusCode,
		Headers:    recorder.headers,
		Body:       recorder.body.Bytes(),
		Error:      nil,
	}

	// Distribute result to waiters
	rc.distributeResult(key, result)
}

// distributeResult sends the result to all waiting requests
func (rc *RequestCoalescer) distributeResult(key string, result *CoalescedResult) {
	rc.mu.Lock()
	req, exists := rc.requests[key]
	if !exists {
		rc.mu.Unlock()
		return
	}

	// Store result for future requests (within TTL)
	req.result = result
	req.inProgress = false
	rc.mu.Unlock()

	// Send result to all waiters
	req.mu.Lock()
	for _, waiter := range req.waiters {
		select {
		case waiter <- result:
		default:
			// Waiter has already timed out
		}
		close(waiter)
	}
	req.waiters = nil
	req.mu.Unlock()

	// Schedule removal after TTL
	time.AfterFunc(rc.ttl, func() {
		rc.mu.Lock()
		delete(rc.requests, key)
		rc.mu.Unlock()
	})
}

// applyResult applies a coalesced result to a response
func (rc *RequestCoalescer) applyResult(c *gin.Context, result *CoalescedResult) {
	if result.Error != nil {
		c.AbortWithError(http.StatusInternalServerError, result.Error)
		return
	}

	// Copy headers
	for k, v := range result.Headers {
		for _, val := range v {
			c.Header(k, val)
		}
	}

	// Add coalescing header
	c.Header("X-Coalesced", "true")

	// Write status and body
	c.Status(result.StatusCode)
	c.Writer.Write(result.Body)
	c.Abort()
}

// cleanup periodically removes expired coalesced requests
func (rc *RequestCoalescer) cleanup() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		rc.mu.Lock()
		now := time.Now()
		for key, req := range rc.requests {
			// Remove requests that have been around too long
			if now.Sub(req.startTime) > rc.ttl*2 {
				delete(rc.requests, key)
			}
		}
		rc.mu.Unlock()
	}
}

// responseRecorder captures the response for coalescing
type responseRecorder struct {
	gin.ResponseWriter
	body       *bytes.Buffer
	statusCode int
	headers    http.Header
	written    bool
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	if !r.written {
		r.statusCode = http.StatusOK
		r.written = true
	}
	r.body.Write(data)
	return r.ResponseWriter.Write(data)
}

func (r *responseRecorder) WriteHeader(code int) {
	if !r.written {
		r.statusCode = code
		r.written = true
		// Copy headers
		for k, v := range r.ResponseWriter.Header() {
			r.headers[k] = v
		}
	}
	r.ResponseWriter.WriteHeader(code)
}

// Stats returns statistics about coalesced requests
func (rc *RequestCoalescer) Stats() map[string]interface{} {
	rc.mu.RLock()
	defer rc.mu.RUnlock()

	activeRequests := 0
	totalWaiters := 0

	for _, req := range rc.requests {
		if req.inProgress {
			activeRequests++
		}
		req.mu.Lock()
		totalWaiters += len(req.waiters)
		req.mu.Unlock()
	}

	return map[string]interface{}{
		"total_requests":  len(rc.requests),
		"active_requests": activeRequests,
		"total_waiters":   totalWaiters,
	}
}