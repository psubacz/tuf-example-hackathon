package server

import (
	"compress/gzip"
	"context"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code and size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int64
}

func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

func (rw *responseWriter) Write(data []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(data)
	rw.size += int64(n)
	return n, err
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code and size
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Process request
		next(rw, r)

		// Log request
		duration := time.Since(start)
		s.logger.Info("%s %s %s %d %d bytes %v",
			r.Method, r.URL.Path, r.RemoteAddr, rw.statusCode, rw.size, duration)

		// Record metrics
		s.metrics.RecordRequest(r.Method, r.URL.Path, rw.statusCode, duration)
	}
}

// metricsMiddleware tracks connection metrics
func (s *Server) metricsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.metrics.IncrementActiveConnections()
		defer s.metrics.DecrementActiveConnections()

		next(w, r)
	}
}

// securityMiddleware adds security headers
func (s *Server) securityMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Basic security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Content Security Policy
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'")

		// HTTPS headers (only if TLS is enabled)
		if s.config.TLS.Enabled {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}

		next(w, r)
	}
}

// corsMiddleware handles CORS
func (s *Server) corsMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.CORS.Enabled {
			next(w, r)
			return
		}

		origin := r.Header.Get("Origin")

		// Check if origin is allowed
		allowed := false
		for _, allowedOrigin := range s.config.CORS.AllowedOrigins {
			if allowedOrigin == "*" || allowedOrigin == origin {
				allowed = true
				break
			}
		}

		if allowed {
			w.Header().Set("Access-Control-Allow-Origin", origin)
		}

		// Set other CORS headers
		w.Header().Set("Access-Control-Allow-Methods", strings.Join(s.config.CORS.AllowedMethods, ", "))
		w.Header().Set("Access-Control-Allow-Headers", strings.Join(s.config.CORS.AllowedHeaders, ", "))
		w.Header().Set("Access-Control-Max-Age", strconv.Itoa(s.config.CORS.MaxAge))

		if len(s.config.CORS.ExposedHeaders) > 0 {
			w.Header().Set("Access-Control-Expose-Headers", strings.Join(s.config.CORS.ExposedHeaders, ", "))
		}

		if s.config.CORS.AllowCredentials {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next(w, r)
	}
}

// compressionMiddleware handles gzip compression
func (s *Server) compressionMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.config.Compression.Enabled {
			next(w, r)
			return
		}

		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next(w, r)
			return
		}

		// Wrap writer with gzip compression
		gz, err := gzip.NewWriterLevel(w, s.config.Compression.Level)
		if err != nil {
			next(w, r)
			return
		}
		defer gz.Close()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Set("Vary", "Accept-Encoding")

		gzw := &gzipResponseWriter{Writer: gz, ResponseWriter: w}
		next(gzw, r)
	}
}

// gzipResponseWriter wraps response writer with gzip compression
type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

// RateLimiter implements token bucket rate limiting
type RateLimiter struct {
	clients map[string]*clientLimiter
	mu      sync.RWMutex
	config  RateLimitConfig
}

type clientLimiter struct {
	tokens   int
	lastSeen time.Time
}

func newRateLimiter(config RateLimitConfig) *RateLimiter {
	rl := &RateLimiter{
		clients: make(map[string]*clientLimiter),
		config:  config,
	}

	// Start cleanup goroutine
	go rl.cleanup()

	return rl
}

func (rl *RateLimiter) cleanup() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, client := range rl.clients {
			if time.Since(client.lastSeen) > rl.config.Window*2 {
				delete(rl.clients, ip)
			}
		}
		rl.mu.Unlock()
	}
}

func (rl *RateLimiter) allow(ip string) bool {
	// Check whitelist
	for _, whitelisted := range rl.config.IPWhitelist {
		if ip == whitelisted {
			return true
		}
	}

	// Check blacklist
	for _, blacklisted := range rl.config.IPBlacklist {
		if ip == blacklisted {
			return false
		}
	}

	rl.mu.Lock()
	defer rl.mu.Unlock()

	client, exists := rl.clients[ip]
	now := time.Now()

	if !exists {
		rl.clients[ip] = &clientLimiter{
			tokens:   rl.config.BurstSize - 1,
			lastSeen: now,
		}
		return true
	}

	// Refill tokens based on elapsed time
	elapsed := now.Sub(client.lastSeen)
	tokensToAdd := int(elapsed / rl.config.Window * time.Duration(rl.config.Requests))

	client.tokens += tokensToAdd
	if client.tokens > rl.config.BurstSize {
		client.tokens = rl.config.BurstSize
	}
	client.lastSeen = now

	if client.tokens > 0 {
		client.tokens--
		return true
	}

	return false
}

// rateLimitMiddleware implements rate limiting
func (s *Server) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	if !s.config.RateLimit.Enabled {
		return next
	}

	limiter := newRateLimiter(s.config.RateLimit)

	return func(w http.ResponseWriter, r *http.Request) {
		// Get client IP
		ip := getClientIP(r)

		if !limiter.allow(ip) {
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(s.config.RateLimit.Requests))
			w.Header().Set("X-RateLimit-Window", s.config.RateLimit.Window.String())
			w.Header().Set("Retry-After", strconv.Itoa(int(s.config.RateLimit.Window.Seconds())))

			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next(w, r)
	}
}

// getClientIP extracts the real client IP from request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (load balancer/proxy)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP header (nginx)
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to remote address
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}

	return ip
}

// timeoutMiddleware adds request timeout
func (s *Server) timeoutMiddleware(timeout time.Duration) func(http.HandlerFunc) http.HandlerFunc {
	return func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			r = r.WithContext(ctx)

			done := make(chan struct{})
			go func() {
				next(w, r)
				close(done)
			}()

			select {
			case <-done:
				return
			case <-ctx.Done():
				http.Error(w, "Request timeout", http.StatusRequestTimeout)
				return
			}
		}
	}
}

// recoveryMiddleware recovers from panics
func (s *Server) recoveryMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				s.logger.Info("PANIC: %v", err)
				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		next(w, r)
	}
}
