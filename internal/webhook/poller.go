package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// PollerClient is a client for polling webhook events from sidecar containers
type PollerClient struct {
	serverURL  string
	repository string
	client     *http.Client
	lastPoll   time.Time
}

// NewPollerClient creates a new webhook poller client
func NewPollerClient(serverURL, repository string) *PollerClient {
	return &PollerClient{
		serverURL:  serverURL,
		repository: repository,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		lastPoll: time.Now(),
	}
}

// PollEvents polls for new events since last poll
func (p *PollerClient) PollEvents(ctx context.Context) ([]*Event, error) {
	url := fmt.Sprintf("%s/api/v1/webhooks/poll?repository=%s&since=%d",
		p.serverURL,
		p.repository,
		p.lastPoll.Unix(),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("poll request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var events []*Event
	if err := json.NewDecoder(resp.Body).Decode(&events); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Update last poll time
	if len(events) > 0 {
		p.lastPoll = events[len(events)-1].Timestamp
	} else {
		p.lastPoll = time.Now()
	}

	return events, nil
}

// Watch continuously polls for events and sends them to a channel
func (p *PollerClient) Watch(ctx context.Context, eventChan chan<- *Event, pollInterval time.Duration) error {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			events, err := p.PollEvents(ctx)
			if err != nil {
				// Log error but continue polling
				continue
			}

			for _, event := range events {
				select {
				case eventChan <- event:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
	}
}

// EventHandler handles webhook events
type EventHandler interface {
	HandleEvent(event *Event) error
}

// FileUpdateHandler handles file update events
type FileUpdateHandler struct {
	onFileAdded   func(repository, path string, data map[string]interface{}) error
	onFileUpdated func(repository, path string, data map[string]interface{}) error
	onFileDeleted func(repository, path string, data map[string]interface{}) error
}

// NewFileUpdateHandler creates a new file update handler
func NewFileUpdateHandler() *FileUpdateHandler {
	return &FileUpdateHandler{}
}

// OnFileAdded sets the handler for file added events
func (h *FileUpdateHandler) OnFileAdded(fn func(repository, path string, data map[string]interface{}) error) *FileUpdateHandler {
	h.onFileAdded = fn
	return h
}

// OnFileUpdated sets the handler for file updated events
func (h *FileUpdateHandler) OnFileUpdated(fn func(repository, path string, data map[string]interface{}) error) *FileUpdateHandler {
	h.onFileUpdated = fn
	return h
}

// OnFileDeleted sets the handler for file deleted events
func (h *FileUpdateHandler) OnFileDeleted(fn func(repository, path string, data map[string]interface{}) error) *FileUpdateHandler {
	h.onFileDeleted = fn
	return h
}

// HandleEvent processes an event
func (h *FileUpdateHandler) HandleEvent(event *Event) error {
	switch event.Type {
	case EventFileAdded:
		if h.onFileAdded != nil {
			path := event.Data["path"].(string)
			return h.onFileAdded(event.Repository, path, event.Data)
		}
	case EventFileUpdated:
		if h.onFileUpdated != nil {
			path := event.Data["path"].(string)
			return h.onFileUpdated(event.Repository, path, event.Data)
		}
	case EventFileDeleted:
		if h.onFileDeleted != nil {
			path := event.Data["path"].(string)
			return h.onFileDeleted(event.Repository, path, event.Data)
		}
	}
	return nil
}

// Sidecar represents a sidecar container that watches for file updates
type Sidecar struct {
	poller   *PollerClient
	handler  EventHandler
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

// NewSidecar creates a new sidecar instance
func NewSidecar(serverURL, repository string, handler EventHandler) *Sidecar {
	ctx, cancel := context.WithCancel(context.Background())
	return &Sidecar{
		poller:   NewPollerClient(serverURL, repository),
		handler:  handler,
		interval: 5 * time.Second,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// SetPollInterval sets the polling interval
func (s *Sidecar) SetPollInterval(interval time.Duration) {
	s.interval = interval
}

// Start starts the sidecar polling loop
func (s *Sidecar) Start() error {
	eventChan := make(chan *Event, 100)
	
	// Start event processor
	go func() {
		for event := range eventChan {
			if err := s.handler.HandleEvent(event); err != nil {
				// Log error
			}
		}
	}()

	// Start polling
	return s.poller.Watch(s.ctx, eventChan, s.interval)
}

// Stop stops the sidecar
func (s *Sidecar) Stop() {
	s.cancel()
}