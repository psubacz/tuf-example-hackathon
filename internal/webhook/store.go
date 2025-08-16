package webhook

import (
	"fmt"
	"sync"
	"time"
)

// MemoryEventStore is an in-memory implementation of EventStore
type MemoryEventStore struct {
	events map[string]*Event
	byRepo map[string][]*Event
	mu     sync.RWMutex
}

// NewMemoryEventStore creates a new in-memory event store
func NewMemoryEventStore() *MemoryEventStore {
	return &MemoryEventStore{
		events: make(map[string]*Event),
		byRepo: make(map[string][]*Event),
	}
}

// Store stores an event
func (s *MemoryEventStore) Store(event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.events[event.ID] = event
	
	// Index by repository
	s.byRepo[event.Repository] = append(s.byRepo[event.Repository], event)
	
	return nil
}

// GetSince returns events since a given timestamp
func (s *MemoryEventStore) GetSince(repository string, since time.Time, limit int) ([]*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	repoEvents := s.byRepo[repository]
	if repoEvents == nil {
		return []*Event{}, nil
	}

	// Filter events after 'since' timestamp
	var result []*Event
	for i := len(repoEvents) - 1; i >= 0; i-- {
		event := repoEvents[i]
		if event.Timestamp.After(since) {
			result = append([]*Event{event}, result...)
			if limit > 0 && len(result) >= limit {
				break
			}
		}
	}

	return result, nil
}

// GetByID returns an event by ID
func (s *MemoryEventStore) GetByID(id string) (*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	event, ok := s.events[id]
	if !ok {
		return nil, fmt.Errorf("event not found: %s", id)
	}
	
	return event, nil
}

// DeleteOlderThan deletes events older than the given time
func (s *MemoryEventStore) DeleteOlderThan(before time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Find events to delete
	toDelete := []string{}
	for id, event := range s.events {
		if event.Timestamp.Before(before) {
			toDelete = append(toDelete, id)
		}
	}

	// Delete from main store
	for _, id := range toDelete {
		delete(s.events, id)
	}

	// Rebuild repository index
	newByRepo := make(map[string][]*Event)
	for _, event := range s.events {
		newByRepo[event.Repository] = append(newByRepo[event.Repository], event)
	}
	s.byRepo = newByRepo

	return nil
}