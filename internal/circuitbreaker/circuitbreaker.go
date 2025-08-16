package circuitbreaker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// State represents the state of the circuit breaker
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	Name             string        `json:"name"`
	MaxRequests      uint32        `json:"max_requests"`
	Interval         time.Duration `json:"interval"`
	Timeout          time.Duration `json:"timeout"`
	Threshold        float64       `json:"threshold"`
	MinRequests      uint32        `json:"min_requests"`
	OnStateChange    func(name string, from, to State)
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config        *Config
	state         State
	lastFailTime  time.Time
	requests      uint32
	failures      uint32
	successes     uint32
	consecutiveFails uint32
	mu            sync.RWMutex
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(cfg *Config) *CircuitBreaker {
	if cfg.MaxRequests == 0 {
		cfg.MaxRequests = 1
	}
	if cfg.Threshold == 0 {
		cfg.Threshold = 0.5
	}
	if cfg.MinRequests == 0 {
		cfg.MinRequests = 5
	}
	if cfg.Interval == 0 {
		cfg.Interval = time.Minute
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &CircuitBreaker{
		config: cfg,
		state:  StateClosed,
	}
}

// Execute runs the given function with circuit breaker protection
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func() error) error {
	// Check if we can execute
	if err := cb.canExecute(); err != nil {
		return err
	}

	// Record request
	cb.recordRequest()

	// Execute function
	err := fn()

	// Record result
	cb.recordResult(err)

	return err
}

// canExecute checks if a request can be executed
func (cb *CircuitBreaker) canExecute() error {
	cb.mu.RLock()
	state := cb.state
	cb.mu.RUnlock()

	switch state {
	case StateClosed:
		return nil
	case StateOpen:
		// Check if timeout has passed
		cb.mu.RLock()
		timeout := cb.lastFailTime.Add(cb.config.Timeout)
		cb.mu.RUnlock()

		if time.Now().After(timeout) {
			// Try to transition to half-open
			cb.transitionTo(StateHalfOpen)
			return nil
		}
		return fmt.Errorf("circuit breaker %s is open", cb.config.Name)
	case StateHalfOpen:
		// Allow limited requests in half-open state
		cb.mu.RLock()
		requests := cb.requests
		maxRequests := cb.config.MaxRequests
		cb.mu.RUnlock()

		if requests >= maxRequests {
			return fmt.Errorf("circuit breaker %s is half-open, max requests reached", cb.config.Name)
		}
		return nil
	default:
		return errors.New("unknown circuit breaker state")
	}
}

// recordRequest increments the request counter
func (cb *CircuitBreaker) recordRequest() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if cb.state == StateHalfOpen {
		cb.requests++
	}
}

// recordResult records the result of a request
func (cb *CircuitBreaker) recordResult(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	if err != nil {
		cb.failures++
		cb.consecutiveFails++

		// Record failure time
		cb.lastFailTime = time.Now()

		// Check if we should open the circuit
		if cb.state == StateClosed {
			if cb.shouldOpen() {
				cb.transitionToLocked(StateOpen)
			}
		} else if cb.state == StateHalfOpen {
			// Any failure in half-open state reopens the circuit
			cb.transitionToLocked(StateOpen)
		}
	} else {
		cb.successes++
		cb.consecutiveFails = 0

		// Success in half-open state closes the circuit
		if cb.state == StateHalfOpen {
			cb.transitionToLocked(StateClosed)
		}
	}
}

// shouldOpen determines if the circuit should open
func (cb *CircuitBreaker) shouldOpen() bool {
	total := cb.failures + cb.successes
	if total < cb.config.MinRequests {
		return false
	}

	failureRate := float64(cb.failures) / float64(total)
	return failureRate >= cb.config.Threshold
}

// transitionTo transitions to a new state
func (cb *CircuitBreaker) transitionTo(newState State) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.transitionToLocked(newState)
}

// transitionToLocked transitions to a new state (must be called with lock held)
func (cb *CircuitBreaker) transitionToLocked(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState

	// Reset counters when transitioning
	if newState == StateHalfOpen {
		cb.requests = 0
		cb.failures = 0
		cb.successes = 0
	} else if newState == StateClosed {
		cb.failures = 0
		cb.successes = 0
		cb.consecutiveFails = 0
	}

	// Call state change handler
	if cb.config.OnStateChange != nil {
		go cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// GetStats returns statistics about the circuit breaker
func (cb *CircuitBreaker) GetStats() map[string]interface{} {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	total := cb.failures + cb.successes
	failureRate := float64(0)
	if total > 0 {
		failureRate = float64(cb.failures) / float64(total)
	}

	return map[string]interface{}{
		"name":              cb.config.Name,
		"state":             cb.state.String(),
		"failures":          cb.failures,
		"successes":         cb.successes,
		"consecutive_fails": cb.consecutiveFails,
		"failure_rate":      failureRate,
		"last_fail_time":    cb.lastFailTime,
	}
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.state = StateClosed
	cb.failures = 0
	cb.successes = 0
	cb.consecutiveFails = 0
	cb.requests = 0
}

// CircuitBreakerManager manages multiple circuit breakers
type Manager struct {
	breakers map[string]*CircuitBreaker
	mu       sync.RWMutex
}

// NewManager creates a new circuit breaker manager
func NewManager() *Manager {
	return &Manager{
		breakers: make(map[string]*CircuitBreaker),
	}
}

// Get gets or creates a circuit breaker
func (m *Manager) Get(name string) *CircuitBreaker {
	m.mu.RLock()
	cb, exists := m.breakers[name]
	m.mu.RUnlock()

	if exists {
		return cb
	}

	// Create new circuit breaker with default config
	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cb, exists := m.breakers[name]; exists {
		return cb
	}

	cb = NewCircuitBreaker(&Config{
		Name:        name,
		MaxRequests: 3,
		Interval:    time.Minute,
		Timeout:     30 * time.Second,
		Threshold:   0.5,
		MinRequests: 5,
	})

	m.breakers[name] = cb
	return cb
}

// Register registers a new circuit breaker with custom config
func (m *Manager) Register(name string, cfg *Config) *CircuitBreaker {
	m.mu.Lock()
	defer m.mu.Unlock()

	cfg.Name = name
	cb := NewCircuitBreaker(cfg)
	m.breakers[name] = cb
	return cb
}

// GetAll returns all circuit breakers
func (m *Manager) GetAll() map[string]*CircuitBreaker {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]*CircuitBreaker)
	for k, v := range m.breakers {
		result[k] = v
	}
	return result
}

// GetStats returns statistics for all circuit breakers
func (m *Manager) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})
	for name, cb := range m.breakers {
		stats[name] = cb.GetStats()
	}
	return stats
}

// Middleware returns a Gin middleware that uses circuit breakers
func (m *Manager) Middleware(name string) gin.HandlerFunc {
	return func(c *gin.Context) {
		cb := m.Get(name)

		// Add tracing
		if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
			span.SetAttributes(
				attribute.String("circuitbreaker.name", name),
				attribute.String("circuitbreaker.state", cb.GetState().String()),
			)
		}

		// Execute with circuit breaker
		err := cb.Execute(c.Request.Context(), func() error {
			c.Next()
			
			// Check if response indicates failure
			if c.Writer.Status() >= 500 {
				return fmt.Errorf("downstream error: status %d", c.Writer.Status())
			}
			
			// Check if there were any errors in the context
			if len(c.Errors) > 0 {
				return c.Errors.Last()
			}
			
			return nil
		})

		if err != nil {
			// Circuit is open or request failed
			if span := trace.SpanFromContext(c.Request.Context()); span.IsRecording() {
				span.RecordError(err)
			}

			// Don't override if already written
			if !c.Writer.Written() {
				c.AbortWithStatusJSON(503, gin.H{
					"error": "service unavailable",
					"details": err.Error(),
				})
			}
		}
	}
}