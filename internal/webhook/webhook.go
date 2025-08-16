package webhook

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
	"tuf-golang-project/internal/utils"
)

// EventType represents the type of webhook event
type EventType string

const (
	EventFileAdded     EventType = "file.added"
	EventFileUpdated   EventType = "file.updated"
	EventFileDeleted   EventType = "file.deleted"
	EventMetadataUpdate EventType = "metadata.update"
	EventKeyRotation   EventType = "key.rotation"
	EventRepoCreated   EventType = "repository.created"
	EventRepoDeleted   EventType = "repository.deleted"
)

// Event represents a webhook event
type Event struct {
	ID         string                 `json:"id"`
	Type       EventType              `json:"type"`
	Repository string                 `json:"repository"`
	Timestamp  time.Time              `json:"timestamp"`
	Data       map[string]interface{} `json:"data"`
	Signature  string                 `json:"signature,omitempty"`
}

// WebhookEndpoint represents a configured webhook endpoint
type WebhookEndpoint struct {
	ID          string            `json:"id"`
	URL         string            `json:"url"`
	Secret      string            `json:"secret,omitempty"`
	Events      []EventType       `json:"events"`
	Active      bool              `json:"active"`
	Headers     map[string]string `json:"headers,omitempty"`
	RetryConfig *RetryConfig      `json:"retry_config,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// RetryConfig configures retry behavior for webhook delivery
type RetryConfig struct {
	MaxAttempts int           `json:"max_attempts"`
	InitialWait time.Duration `json:"initial_wait"`
	MaxWait     time.Duration `json:"max_wait"`
}

// Manager manages webhook endpoints and event delivery
type Manager struct {
	endpoints map[string]*WebhookEndpoint
	events    chan *Event
	store     EventStore
	client    *http.Client
	mu        sync.RWMutex
	workers   int
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// EventStore interface for persisting events (for polling)
type EventStore interface {
	Store(event *Event) error
	GetSince(repository string, since time.Time, limit int) ([]*Event, error)
	GetByID(id string) (*Event, error)
	DeleteOlderThan(before time.Time) error
}

// Config for webhook manager
type Config struct {
	Workers        int
	BufferSize     int
	EventRetention time.Duration
	DefaultRetry   *RetryConfig
	CACertFile     string // Path to CA certificate file for HTTPS requests
}

// NewManager creates a new webhook manager
func NewManager(config *Config, store EventStore) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	
	// Create HTTP client with optional custom CA certificate
	clientConfig := utils.HTTPClientConfig{
		CACertFile: config.CACertFile,
	}
	httpClient, err := utils.CreateHTTPClient(clientConfig, 30*time.Second)
	if err != nil {
		// Fall back to default client if custom configuration fails
		httpClient = &http.Client{
			Timeout: 30 * time.Second,
		}
	}
	
	m := &Manager{
		endpoints: make(map[string]*WebhookEndpoint),
		events:    make(chan *Event, config.BufferSize),
		store:     store,
		client:    httpClient,
		workers:   config.Workers,
		ctx:       ctx,
		cancel:    cancel,
	}

	// Start workers
	for i := 0; i < config.Workers; i++ {
		m.wg.Add(1)
		go m.worker()
	}

	// Start cleanup routine
	m.wg.Add(1)
	go m.cleanupRoutine(config.EventRetention)

	return m
}

// RegisterEndpoint registers a new webhook endpoint
func (m *Manager) RegisterEndpoint(endpoint *WebhookEndpoint) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if endpoint.ID == "" {
		endpoint.ID = uuid.New().String()
	}
	
	endpoint.CreatedAt = time.Now()
	endpoint.UpdatedAt = time.Now()

	m.endpoints[endpoint.ID] = endpoint
	return nil
}

// UnregisterEndpoint removes a webhook endpoint
func (m *Manager) UnregisterEndpoint(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	delete(m.endpoints, id)
	return nil
}

// GetEndpoint retrieves a webhook endpoint by ID
func (m *Manager) GetEndpoint(id string) (*WebhookEndpoint, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	endpoint, ok := m.endpoints[id]
	return endpoint, ok
}

// ListEndpoints returns all registered endpoints
func (m *Manager) ListEndpoints() []*WebhookEndpoint {
	m.mu.RLock()
	defer m.mu.RUnlock()

	endpoints := make([]*WebhookEndpoint, 0, len(m.endpoints))
	for _, endpoint := range m.endpoints {
		endpoints = append(endpoints, endpoint)
	}
	return endpoints
}

// Publish publishes an event to all matching endpoints
func (m *Manager) Publish(eventType EventType, repository string, data map[string]interface{}) error {
	event := &Event{
		ID:         uuid.New().String(),
		Type:       eventType,
		Repository: repository,
		Timestamp:  time.Now(),
		Data:       data,
	}

	// Store event for polling
	if err := m.store.Store(event); err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	// Queue for delivery
	select {
	case m.events <- event:
		return nil
	case <-m.ctx.Done():
		return fmt.Errorf("manager is shutting down")
	default:
		return fmt.Errorf("event buffer full")
	}
}

// worker processes events and delivers them to webhooks
func (m *Manager) worker() {
	defer m.wg.Done()

	for {
		select {
		case event := <-m.events:
			m.deliverEvent(event)
		case <-m.ctx.Done():
			return
		}
	}
}

// deliverEvent delivers an event to all matching endpoints
func (m *Manager) deliverEvent(event *Event) {
	m.mu.RLock()
	endpoints := make([]*WebhookEndpoint, 0)
	for _, endpoint := range m.endpoints {
		if endpoint.Active && m.shouldDeliver(endpoint, event) {
			endpoints = append(endpoints, endpoint)
		}
	}
	m.mu.RUnlock()

	var wg sync.WaitGroup
	for _, endpoint := range endpoints {
		wg.Add(1)
		go func(ep *WebhookEndpoint) {
			defer wg.Done()
			m.deliverToEndpoint(event, ep)
		}(endpoint)
	}
	wg.Wait()
}

// shouldDeliver checks if an event should be delivered to an endpoint
func (m *Manager) shouldDeliver(endpoint *WebhookEndpoint, event *Event) bool {
	if len(endpoint.Events) == 0 {
		return true // Deliver all events if no filter
	}

	for _, eventType := range endpoint.Events {
		if eventType == event.Type {
			return true
		}
	}
	return false
}

// deliverToEndpoint delivers an event to a specific endpoint with retries
func (m *Manager) deliverToEndpoint(event *Event, endpoint *WebhookEndpoint) {
	retryConfig := endpoint.RetryConfig
	if retryConfig == nil {
		retryConfig = &RetryConfig{
			MaxAttempts: 3,
			InitialWait: 1 * time.Second,
			MaxWait:     30 * time.Second,
		}
	}

	wait := retryConfig.InitialWait
	for attempt := 1; attempt <= retryConfig.MaxAttempts; attempt++ {
		if err := m.sendWebhook(event, endpoint); err == nil {
			return
		}

		if attempt < retryConfig.MaxAttempts {
			time.Sleep(wait)
			wait *= 2
			if wait > retryConfig.MaxWait {
				wait = retryConfig.MaxWait
			}
		}
	}
}

// sendWebhook sends a webhook HTTP request
func (m *Manager) sendWebhook(event *Event, endpoint *WebhookEndpoint) error {
	// Add signature if secret is configured
	if endpoint.Secret != "" {
		event.Signature = m.generateSignature(event, endpoint.Secret)
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequestWithContext(m.ctx, "POST", endpoint.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-TUF-Event", string(event.Type))
	req.Header.Set("X-TUF-Event-ID", event.ID)
	req.Header.Set("X-TUF-Repository", event.Repository)
	
	if event.Signature != "" {
		req.Header.Set("X-TUF-Signature", event.Signature)
	}

	// Add custom headers
	for key, value := range endpoint.Headers {
		req.Header.Set(key, value)
	}

	resp, err := m.client.Do(req)
	if err != nil {
		return fmt.Errorf("webhook request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

// generateSignature generates HMAC-SHA256 signature for the event
func (m *Manager) generateSignature(event *Event, secret string) string {
	// Create a copy without signature field
	eventCopy := *event
	eventCopy.Signature = ""
	
	data, _ := json.Marshal(eventCopy)
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// cleanupRoutine periodically cleans up old events
func (m *Manager) cleanupRoutine(retention time.Duration) {
	defer m.wg.Done()

	ticker := time.NewTicker(1 * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			before := time.Now().Add(-retention)
			if err := m.store.DeleteOlderThan(before); err != nil {
				// Log error but continue cleanup routine
				_ = err
			}
		case <-m.ctx.Done():
			return
		}
	}
}

// Shutdown gracefully shuts down the webhook manager
func (m *Manager) Shutdown() {
	m.cancel()
	close(m.events)
	m.wg.Wait()
}

// GetEventsSince returns events since a given timestamp (for polling)
func (m *Manager) GetEventsSince(repository string, since time.Time, limit int) ([]*Event, error) {
	return m.store.GetSince(repository, since, limit)
}

// GetEvent returns a specific event by ID
func (m *Manager) GetEvent(id string) (*Event, error) {
	return m.store.GetByID(id)
}