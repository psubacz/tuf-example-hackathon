package middleware

import (
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

const (
	RequestIDHeader = "X-Request-ID"
	TraceIDHeader   = "X-Trace-ID"
)

var (
	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "tuf_http_duration_seconds",
		Help: "Duration of HTTP requests.",
		Buckets: []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
	}, []string{"method", "path", "status"})

	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "tuf_http_requests_total",
		Help: "Total number of HTTP requests.",
	}, []string{"method", "path", "status"})

	httpActiveRequests = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "tuf_http_active_requests",
		Help: "Number of active HTTP requests.",
	})

	httpRequestSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "tuf_http_request_size_bytes",
		Help: "Size of HTTP requests in bytes.",
		Buckets: prometheus.ExponentialBuckets(100, 10, 8),
	}, []string{"method", "path"})

	httpResponseSize = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name: "tuf_http_response_size_bytes",
		Help: "Size of HTTP responses in bytes.",
		Buckets: prometheus.ExponentialBuckets(100, 10, 8),
	}, []string{"method", "path", "status"})
)

type responseWriter struct {
	gin.ResponseWriter
	size int
}

func (w *responseWriter) Write(b []byte) (int, error) {
	size, err := w.ResponseWriter.Write(b)
	w.size += size
	return size, err
}

func (w *responseWriter) WriteString(s string) (int, error) {
	size, err := w.ResponseWriter.WriteString(s)
	w.size += size
	return size, err
}

// Recovery middleware recovers from panics and returns 500 error
func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered interface{}) {
		if err, ok := recovered.(string); ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
				"message": err,
				"request_id": c.GetString("request_id"),
			})
		} else {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Internal server error",
				"request_id": c.GetString("request_id"),
			})
		}
		c.Abort()
	})
}

// Logger middleware provides structured logging with request details
func Logger() gin.HandlerFunc {
	return gin.LoggerWithConfig(gin.LoggerConfig{
		SkipPaths: []string{"/health", "/metrics"},
		Formatter: func(param gin.LogFormatterParams) string {
			return fmt.Sprintf("[%s] %s %s %s %d %s %s %s\n",
				param.TimeStamp.Format(time.RFC3339),
				param.ClientIP,
				param.Method,
				param.Path,
				param.StatusCode,
				param.Latency,
				param.Request.UserAgent(),
				param.ErrorMessage,
			)
		},
	})
}

// RequestID middleware adds a unique request ID to each request
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader(RequestIDHeader)
		if requestID == "" {
			requestID = uuid.New().String()
		}
		
		c.Set("request_id", requestID)
		c.Header(RequestIDHeader, requestID)
		
		// Also set trace ID if not present
		traceID := c.GetHeader(TraceIDHeader)
		if traceID == "" {
			traceID = requestID
		}
		c.Set("trace_id", traceID)
		c.Header(TraceIDHeader, traceID)
		
		c.Next()
	}
}

// Metrics middleware collects Prometheus metrics
func Metrics() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		
		// Track active requests
		httpActiveRequests.Inc()
		defer httpActiveRequests.Dec()
		
		// Track request size
		if c.Request.ContentLength > 0 {
			httpRequestSize.WithLabelValues(c.Request.Method, path).Observe(float64(c.Request.ContentLength))
		}
		
		// Wrap the response writer to capture response size
		rw := &responseWriter{ResponseWriter: c.Writer, size: 0}
		c.Writer = rw
		
		c.Next()
		
		// Record metrics after request is processed
		duration := time.Since(start).Seconds()
		status := fmt.Sprintf("%d", c.Writer.Status())
		
		httpDuration.WithLabelValues(c.Request.Method, path, status).Observe(duration)
		httpRequests.WithLabelValues(c.Request.Method, path, status).Inc()
		httpResponseSize.WithLabelValues(c.Request.Method, path, status).Observe(float64(rw.size))
	}
}

// CORS middleware handles Cross-Origin Resource Sharing
func CORS(allowedOrigins []string, allowedHeaders []string) gin.HandlerFunc {
	return func(c *gin.Context) {
		origin := c.Request.Header.Get("Origin")
		
		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range allowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}
		
		if allowed {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
			c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID, X-Trace-ID")
			c.Header("Access-Control-Max-Age", "86400")
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		
		c.Next()
	}
}

// Security middleware adds security headers
func Security() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-XSS-Protection", "1; mode=block")
		c.Header("Referrer-Policy", "strict-origin-when-cross-origin")
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")
		c.Next()
	}
}