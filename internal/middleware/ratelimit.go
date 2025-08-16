package middleware

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     int           // requests per window
	window   time.Duration // time window
}

type visitor struct {
	count    int
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter
func NewRateLimiter(rate int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate,
		window:   window,
	}
	
	// Start cleanup goroutine
	go rl.cleanupVisitors()
	
	return rl
}

// cleanupVisitors removes old entries from the visitors map
func (rl *RateLimiter) cleanupVisitors() {
	for {
		time.Sleep(time.Minute)
		
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.window {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// allow checks if a request from the given IP is allowed
func (rl *RateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	
	v, exists := rl.visitors[ip]
	if !exists {
		rl.visitors[ip] = &visitor{count: 1, lastSeen: time.Now()}
		return true
	}
	
	// Reset counter if window has passed
	if time.Since(v.lastSeen) > rl.window {
		v.count = 1
		v.lastSeen = time.Now()
		return true
	}
	
	// Check if limit exceeded
	if v.count >= rl.rate {
		return false
	}
	
	v.count++
	v.lastSeen = time.Now()
	return true
}

// RateLimit middleware for Gin
func RateLimit(rate int, window time.Duration) gin.HandlerFunc {
	limiter := NewRateLimiter(rate, window)
	
	return func(c *gin.Context) {
		ip := c.ClientIP()
		
		if !limiter.allow(ip) {
			c.Header("Retry-After", window.String())
			c.JSON(http.StatusTooManyRequests, gin.H{
				"error": "Too many requests",
				"message": "Rate limit exceeded. Please try again later.",
				"retry_after": window.String(),
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}

// GlobalRateLimiter provides a global rate limit across all IPs
type GlobalRateLimiter struct {
	tokens   int
	capacity int
	mu       sync.Mutex
	refill   time.Duration
}

// NewGlobalRateLimiter creates a new global rate limiter using token bucket algorithm
func NewGlobalRateLimiter(capacity int, refillRate time.Duration) *GlobalRateLimiter {
	grl := &GlobalRateLimiter{
		tokens:   capacity,
		capacity: capacity,
		refill:   refillRate,
	}
	
	// Start token refill goroutine
	go grl.refillTokens()
	
	return grl
}

// refillTokens adds tokens at the specified rate
func (grl *GlobalRateLimiter) refillTokens() {
	ticker := time.NewTicker(grl.refill)
	defer ticker.Stop()
	
	for range ticker.C {
		grl.mu.Lock()
		if grl.tokens < grl.capacity {
			grl.tokens++
		}
		grl.mu.Unlock()
	}
}

// allow checks if a request can proceed
func (grl *GlobalRateLimiter) allow() bool {
	grl.mu.Lock()
	defer grl.mu.Unlock()
	
	if grl.tokens > 0 {
		grl.tokens--
		return true
	}
	return false
}

// GlobalRateLimit middleware for Gin
func GlobalRateLimit(capacity int, refillRate time.Duration) gin.HandlerFunc {
	limiter := NewGlobalRateLimiter(capacity, refillRate)
	
	return func(c *gin.Context) {
		if !limiter.allow() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"error": "Service temporarily unavailable",
				"message": "Server is experiencing high load. Please try again later.",
			})
			c.Abort()
			return
		}
		
		c.Next()
	}
}