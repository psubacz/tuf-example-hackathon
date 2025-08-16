package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

// RequestSizeLimiter creates a middleware that limits request body size
func RequestSizeLimiter(maxBytes int64) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check Content-Length header
		if c.Request.ContentLength > maxBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "Request body too large",
				"message": fmt.Sprintf("Request body exceeds maximum size of %d bytes", maxBytes),
				"max_size": maxBytes,
				"request_size": c.Request.ContentLength,
			})
			c.Abort()
			return
		}

		// Limit the reader to prevent malicious requests without Content-Length
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		
		c.Next()
	}
}

// TimeoutMiddleware adds timeout control to requests
func TimeoutMiddleware(timeout int) gin.HandlerFunc {
	return func(c *gin.Context) {
		// Note: Gin doesn't directly support per-request timeouts
		// This is handled at the server level in http.Server configuration
		// This middleware adds a header to indicate the timeout
		c.Header("X-Timeout-Seconds", strconv.Itoa(timeout))
		c.Next()
	}
}

// ChunkedTransferMiddleware enables chunked transfer encoding for large responses
func ChunkedTransferMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if client supports chunked encoding
		acceptEncoding := c.GetHeader("Accept-Encoding")
		if strings.Contains(acceptEncoding, "chunked") {
			c.Header("Transfer-Encoding", "chunked")
		}
		c.Next()
	}
}

// RangeRequestMiddleware handles byte-range requests for partial content
func RangeRequestMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Store range header for handler to process
		rangeHeader := c.GetHeader("Range")
		if rangeHeader != "" {
			c.Set("range_header", rangeHeader)
		}
		c.Next()
	}
}

// ParseRangeHeader parses a Range header value
func ParseRangeHeader(rangeHeader string, fileSize int64) (start, end int64, err error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("invalid range header format")
	}

	rangeSpec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.Split(rangeSpec, "-")
	
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range specification")
	}

	// Parse start
	if parts[0] != "" {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range start: %w", err)
		}
	}

	// Parse end
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid range end: %w", err)
		}
	} else {
		// If no end specified, serve to end of file
		end = fileSize - 1
	}

	// Validate range
	if start < 0 || end < 0 || start > end || start >= fileSize {
		return 0, 0, fmt.Errorf("invalid range: start=%d, end=%d, fileSize=%d", start, end, fileSize)
	}

	// Adjust end if it exceeds file size
	if end >= fileSize {
		end = fileSize - 1
	}

	return start, end, nil
}

// CompressionMiddleware enables response compression for supported content types
func CompressionMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Check if client accepts compression
		acceptEncoding := c.GetHeader("Accept-Encoding")
		
		// Enable gzip if supported
		if strings.Contains(acceptEncoding, "gzip") {
			c.Header("Vary", "Accept-Encoding")
			// Gin has built-in gzip support through gin.Gzip() middleware
			// This is just to set headers appropriately
		}
		
		c.Next()
	}
}

// RequestBodyLimitByEndpoint returns different size limits based on endpoint
func RequestBodyLimitByEndpoint() gin.HandlerFunc {
	limits := map[string]int64{
		"/admin/targets/add":     500 * 1024 * 1024,  // 500MB for file uploads
		"/admin/metadata/sign":   10 * 1024 * 1024,   // 10MB for metadata
		"/api/":                  1 * 1024 * 1024,    // 1MB for API calls
		"/":                      100 * 1024,          // 100KB default
	}

	return func(c *gin.Context) {
		path := c.Request.URL.Path
		maxBytes := int64(100 * 1024) // Default 100KB

		// Find matching limit
		for prefix, limit := range limits {
			if strings.HasPrefix(path, prefix) {
				maxBytes = limit
				break
			}
		}

		// Apply the limit
		if c.Request.ContentLength > maxBytes {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{
				"error": "Request body too large",
				"message": fmt.Sprintf("Request body exceeds maximum size of %d bytes for this endpoint", maxBytes),
				"max_size": maxBytes,
				"request_size": c.Request.ContentLength,
			})
			c.Abort()
			return
		}

		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBytes)
		c.Next()
	}
}