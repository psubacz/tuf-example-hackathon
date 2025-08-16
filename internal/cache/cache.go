package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"sync"
	"time"
)

// CacheEntry represents a cached metadata file
type CacheEntry struct {
	Content      []byte
	ETag         string
	LastModified time.Time
	ExpiresAt    time.Time
	ContentType  string
	Size         int64
}

// MetadataCache provides in-memory caching for TUF metadata
type MetadataCache struct {
	entries map[string]*CacheEntry
	mu      sync.RWMutex
	
	// TTL configuration for different metadata types
	rootTTL      time.Duration
	timestampTTL time.Duration
	snapshotTTL  time.Duration
	targetsTTL   time.Duration
	defaultTTL   time.Duration
}

// NewMetadataCache creates a new metadata cache with configurable TTLs
func NewMetadataCache() *MetadataCache {
	cache := &MetadataCache{
		entries:      make(map[string]*CacheEntry),
		rootTTL:      60 * time.Second,       // Root: 1 minute
		timestampTTL: 5 * time.Second,        // Timestamp: 5 seconds (freshness critical)
		snapshotTTL:  5 * time.Minute,        // Snapshot: 5 minutes
		targetsTTL:   1 * time.Hour,          // Targets: 1 hour
		defaultTTL:   10 * time.Minute,       // Default: 10 minutes
	}
	
	// Start cleanup goroutine
	go cache.cleanupExpired()
	
	return cache
}

// SetTTLs allows customizing TTL values
func (c *MetadataCache) SetTTLs(root, timestamp, snapshot, targets, defaultTTL time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.rootTTL = root
	c.timestampTTL = timestamp
	c.snapshotTTL = snapshot
	c.targetsTTL = targets
	c.defaultTTL = defaultTTL
}

// Get retrieves a cached entry if it exists and hasn't expired
func (c *MetadataCache) Get(key string) (*CacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	entry, exists := c.entries[key]
	if !exists {
		return nil, false
	}
	
	// Check if entry has expired
	if time.Now().After(entry.ExpiresAt) {
		return nil, false
	}
	
	return entry, true
}

// Set stores a file in the cache with appropriate TTL
func (c *MetadataCache) Set(key string, filePath string) error {
	// Read file content
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}
	
	// Get file info
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return err
	}
	
	// Calculate ETag (using SHA256 of content)
	hash := sha256.Sum256(content)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:]))
	
	// Determine TTL based on file type
	ttl := c.getTTLForFile(key)
	
	// Determine content type
	contentType := "application/json"
	
	entry := &CacheEntry{
		Content:      content,
		ETag:         etag,
		LastModified: fileInfo.ModTime(),
		ExpiresAt:    time.Now().Add(ttl),
		ContentType:  contentType,
		Size:         fileInfo.Size(),
	}
	
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
	
	return nil
}

// SetRaw stores raw content in the cache
func (c *MetadataCache) SetRaw(key string, content []byte, contentType string, ttl time.Duration) {
	// Calculate ETag
	hash := sha256.Sum256(content)
	etag := fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:]))
	
	entry := &CacheEntry{
		Content:      content,
		ETag:         etag,
		LastModified: time.Now(),
		ExpiresAt:    time.Now().Add(ttl),
		ContentType:  contentType,
		Size:         int64(len(content)),
	}
	
	c.mu.Lock()
	c.entries[key] = entry
	c.mu.Unlock()
}

// Invalidate removes an entry from the cache
func (c *MetadataCache) Invalidate(key string) {
	c.mu.Lock()
	delete(c.entries, key)
	c.mu.Unlock()
}

// InvalidateAll clears the entire cache
func (c *MetadataCache) InvalidateAll() {
	c.mu.Lock()
	c.entries = make(map[string]*CacheEntry)
	c.mu.Unlock()
}

// InvalidatePattern removes all entries matching a pattern
func (c *MetadataCache) InvalidatePattern(pattern string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	for key := range c.entries {
		// Simple pattern matching (could be enhanced with glob patterns)
		if matched := matchPattern(key, pattern); matched {
			delete(c.entries, key)
		}
	}
}

// GetStats returns cache statistics
func (c *MetadataCache) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	totalSize := int64(0)
	validEntries := 0
	now := time.Now()
	
	for _, entry := range c.entries {
		if now.Before(entry.ExpiresAt) {
			validEntries++
			totalSize += entry.Size
		}
	}
	
	return map[string]interface{}{
		"total_entries": len(c.entries),
		"valid_entries": validEntries,
		"total_size":    totalSize,
		"size_mb":       float64(totalSize) / (1024 * 1024),
	}
}

// getTTLForFile determines the TTL based on the file type
func (c *MetadataCache) getTTLForFile(key string) time.Duration {
	switch {
	case contains(key, "timestamp.json"):
		return c.timestampTTL
	case contains(key, "root.json"):
		return c.rootTTL
	case contains(key, "snapshot.json"):
		return c.snapshotTTL
	case contains(key, "targets.json"):
		return c.targetsTTL
	default:
		return c.defaultTTL
	}
}

// cleanupExpired periodically removes expired entries
func (c *MetadataCache) cleanupExpired() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for range ticker.C {
		c.mu.Lock()
		now := time.Now()
		for key, entry := range c.entries {
			if now.After(entry.ExpiresAt) {
				delete(c.entries, key)
			}
		}
		c.mu.Unlock()
	}
}

// CheckETag checks if the provided ETag matches the cached version
func (c *MetadataCache) CheckETag(key string, etag string) bool {
	entry, exists := c.Get(key)
	if !exists {
		return false
	}
	return entry.ETag == etag
}

// CheckModifiedSince checks if the cached entry was modified since the given time
func (c *MetadataCache) CheckModifiedSince(key string, since time.Time) bool {
	entry, exists := c.Get(key)
	if !exists {
		return true // If not cached, consider it modified
	}
	return entry.LastModified.After(since)
}

// GenerateETag generates an ETag for a file
func GenerateETag(content []byte) string {
	hash := sha256.Sum256(content)
	return fmt.Sprintf(`"%s"`, hex.EncodeToString(hash[:]))
}

// GenerateETagFromFile generates an ETag from a file path
func GenerateETagFromFile(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()
	
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", err
	}
	
	return fmt.Sprintf(`"%s"`, hex.EncodeToString(hasher.Sum(nil))), nil
}

// Helper functions
func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr
}

func matchPattern(key, pattern string) bool {
	// Simple implementation - can be enhanced with proper glob matching
	if pattern == "*" {
		return true
	}
	return contains(key, pattern)
}