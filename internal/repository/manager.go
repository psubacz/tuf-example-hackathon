package repository

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"tuf-golang-project/internal/logger"
	"tuf-golang-project/internal/storage"
)

// Repository represents a TUF repository with namespace support
type Repository struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Description string            `json:"description"`
	Backend     storage.Backend   `json:"-"`
	Config      *RepositoryConfig `json:"config"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
	mu          sync.RWMutex
}

// RepositoryConfig holds repository-specific configuration
type RepositoryConfig struct {
	Public          bool              `json:"public"`
	AllowedClients  []string          `json:"allowed_clients,omitempty"`
	MaxFileSize     int64             `json:"max_file_size"`
	RetentionDays   int               `json:"retention_days"`
	SigningKeys     map[string]string `json:"signing_keys,omitempty"`
	WebhookEndpoints []string         `json:"webhook_endpoints,omitempty"`
}

// Manager manages multiple TUF repositories
type Manager struct {
	repositories map[string]*Repository
	backends     map[string]storage.Backend
	defaultBackend storage.Backend
	logger       *slog.Logger
	mu           sync.RWMutex
}

// NewManager creates a new repository manager
func NewManager(defaultBackend storage.Backend) *Manager {
	m := &Manager{
		repositories:   make(map[string]*Repository),
		backends:       make(map[string]storage.Backend),
		defaultBackend: defaultBackend,
		logger:         logger.Logger,
	}
	
	// Load existing repositories from storage
	if err := m.loadRepositories(); err != nil {
		// Log error but don't fail - allows server to start even if some repos are corrupted
		m.logger.Warn("Failed to load existing repositories", "error", err)
	} else {
		m.logger.Info("Repository manager initialized", "loaded_repos", len(m.repositories))
	}
	
	return m
}

// CreateRepository creates a new repository
func (m *Manager) CreateRepository(namespace, name string, config *RepositoryConfig) (*Repository, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Validate namespace and name
	if err := validateNamespace(namespace); err != nil {
		return nil, err
	}
	if err := validateRepoName(name); err != nil {
		return nil, err
	}

	repoID := formatRepoID(namespace, name)
	
	// Check if repository already exists
	if _, exists := m.repositories[repoID]; exists {
		return nil, fmt.Errorf("repository %s already exists", repoID)
	}

	// Create backend for repository (could be different per repo)
	backend := m.getBackendForRepo(namespace, name)

	repo := &Repository{
		Name:      name,
		Namespace: namespace,
		Backend:   backend,
		Config:    config,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	// Initialize repository structure
	if err := m.initializeRepository(repo); err != nil {
		return nil, fmt.Errorf("failed to initialize repository: %w", err)
	}

	m.repositories[repoID] = repo
	
	// Persist repository metadata
	if err := m.saveRepositoryMetadata(repo); err != nil {
		// If we can't persist, remove from memory and fail
		delete(m.repositories, repoID)
		return nil, fmt.Errorf("failed to persist repository metadata: %w", err)
	}
	
	return repo, nil
}

// GetRepository retrieves a repository by namespace and name
func (m *Manager) GetRepository(namespace, name string) (*Repository, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	repoID := formatRepoID(namespace, name)
	repo, exists := m.repositories[repoID]
	if !exists {
		return nil, fmt.Errorf("repository %s not found", repoID)
	}

	return repo, nil
}

// ListRepositories lists all repositories, optionally filtered by namespace
func (m *Manager) ListRepositories(namespace string) []*Repository {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var repos []*Repository
	for id, repo := range m.repositories {
		if namespace == "" || strings.HasPrefix(id, namespace+"/") {
			repos = append(repos, repo)
		}
	}
	return repos
}

// DeleteRepository deletes a repository
func (m *Manager) DeleteRepository(namespace, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	repoID := formatRepoID(namespace, name)
	repo, exists := m.repositories[repoID]
	if !exists {
		return fmt.Errorf("repository %s not found", repoID)
	}

	// Clean up repository data
	if err := m.cleanupRepository(repo); err != nil {
		return fmt.Errorf("failed to cleanup repository: %w", err)
	}

	delete(m.repositories, repoID)
	
	// Remove persisted repository metadata
	if err := m.deleteRepositoryMetadata(repo); err != nil {
		return fmt.Errorf("failed to delete repository metadata: %w", err)
	}
	
	return nil
}

// UpdateRepository updates repository configuration
func (m *Manager) UpdateRepository(namespace, name string, config *RepositoryConfig) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	repoID := formatRepoID(namespace, name)
	repo, exists := m.repositories[repoID]
	if !exists {
		return fmt.Errorf("repository %s not found", repoID)
	}

	repo.mu.Lock()
	defer repo.mu.Unlock()

	repo.Config = config
	repo.UpdatedAt = time.Now()
	
	// Persist updated repository metadata
	return m.saveRepositoryMetadata(repo)
}

// initializeRepository initializes the directory structure for a repository
func (m *Manager) initializeRepository(repo *Repository) error {
	// Create metadata directories
	metadataDirs := []string{
		"metadata",
		"metadata/staged",
		"targets",
		"temp",
	}

	for _, dir := range metadataDirs {
		// Create a marker file in each directory to ensure it exists
		markerPath := filepath.Join(formatRepoPath(repo.Namespace, repo.Name), dir, ".keep")
		if err := repo.Backend.Put(context.Background(), markerPath, bytes.NewReader([]byte{})); err != nil {
			// Directory creation might fail but that's ok if backend doesn't support it
			_ = err // Explicitly ignore error
		}
	}

	// Initialize root metadata
	rootMetadata := m.createInitialRootMetadata(repo)
	rootPath := filepath.Join(formatRepoPath(repo.Namespace, repo.Name), "metadata", "root.json")
	
	if err := repo.Backend.Put(context.Background(), rootPath, bytes.NewReader(rootMetadata)); err != nil {
		return fmt.Errorf("failed to create root metadata: %w", err)
	}

	return nil
}

// cleanupRepository removes all repository data
func (m *Manager) cleanupRepository(repo *Repository) error {
	// Delete all files in repository
	// Most backends don't support recursive delete, so we just mark it as deleted
	// The actual cleanup can be done asynchronously or by a cleanup job
	
	return nil
}

// getBackendForRepo returns the appropriate backend for a repository
func (m *Manager) getBackendForRepo(namespace, name string) storage.Backend {
	// Could return different backends based on namespace/name
	// For now, return default backend
	return m.defaultBackend
}

// createInitialRootMetadata creates the initial root.json for a new repository
func (m *Manager) createInitialRootMetadata(repo *Repository) []byte {
	// This would create a properly formatted TUF root metadata
	// For now, return a basic template
	rootTemplate := `{
		"signed": {
			"_type": "root",
			"spec_version": "1.0.0",
			"version": 1,
			"expires": "%s",
			"keys": {},
			"roles": {
				"root": {"keyids": [], "threshold": 1},
				"snapshot": {"keyids": [], "threshold": 1},
				"targets": {"keyids": [], "threshold": 1},
				"timestamp": {"keyids": [], "threshold": 1}
			}
		},
		"signatures": []
	}`
	
	expires := time.Now().Add(365 * 24 * time.Hour).Format(time.RFC3339)
	return []byte(fmt.Sprintf(rootTemplate, expires))
}

// Helper functions

func formatRepoID(namespace, name string) string {
	return fmt.Sprintf("%s/%s", namespace, name)
}

func formatRepoPath(namespace, name string) string {
	return filepath.Join("repositories", namespace, name)
}

func validateNamespace(namespace string) error {
	if namespace == "" {
		return fmt.Errorf("namespace cannot be empty")
	}
	if strings.Contains(namespace, "/") {
		return fmt.Errorf("namespace cannot contain '/'")
	}
	if strings.Contains(namespace, "..") {
		return fmt.Errorf("namespace cannot contain '..'")
	}
	return nil
}

func validateRepoName(name string) error {
	if name == "" {
		return fmt.Errorf("repository name cannot be empty")
	}
	if strings.Contains(name, "/") {
		return fmt.Errorf("repository name cannot contain '/'")
	}
	if strings.Contains(name, "..") {
		return fmt.Errorf("repository name cannot contain '..'")
	}
	return nil
}

// Repository methods

// GetMetadata retrieves metadata from the repository
func (r *Repository) GetMetadata(role string) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	path := filepath.Join(formatRepoPath(r.Namespace, r.Name), "metadata", role+".json")
	reader, err := r.Backend.Get(context.Background(), path)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	
	return io.ReadAll(reader)
}

// PutMetadata stores metadata in the repository
func (r *Repository) PutMetadata(role string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	path := filepath.Join(formatRepoPath(r.Namespace, r.Name), "metadata", role+".json")
	if err := r.Backend.Put(context.Background(), path, bytes.NewReader(data)); err != nil {
		return err
	}

	r.UpdatedAt = time.Now()
	return nil
}

// GetTarget retrieves a target file from the repository
func (r *Repository) GetTarget(path string) ([]byte, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targetPath := filepath.Join(formatRepoPath(r.Namespace, r.Name), "targets", path)
	reader, err := r.Backend.Get(context.Background(), targetPath)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	
	return io.ReadAll(reader)
}

// PutTarget stores a target file in the repository
func (r *Repository) PutTarget(path string, data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Check file size limit
	if r.Config.MaxFileSize > 0 && int64(len(data)) > r.Config.MaxFileSize {
		return fmt.Errorf("file size exceeds limit of %d bytes", r.Config.MaxFileSize)
	}

	targetPath := filepath.Join(formatRepoPath(r.Namespace, r.Name), "targets", path)
	if err := r.Backend.Put(context.Background(), targetPath, bytes.NewReader(data)); err != nil {
		return err
	}

	r.UpdatedAt = time.Now()
	return nil
}

// DeleteTarget removes a target file from the repository
func (r *Repository) DeleteTarget(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	targetPath := filepath.Join(formatRepoPath(r.Namespace, r.Name), "targets", path)
	if err := r.Backend.Delete(context.Background(), targetPath); err != nil {
		return err
	}

	r.UpdatedAt = time.Now()
	return nil
}

// ListTargets lists all target files in the repository
func (r *Repository) ListTargets() ([]string, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	targetPath := filepath.Join(formatRepoPath(r.Namespace, r.Name), "targets")
	return r.Backend.List(context.Background(), targetPath)
}

// IsPublic returns whether the repository is public
func (r *Repository) IsPublic() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.Config.Public
}

// IsClientAllowed checks if a client is allowed to access the repository
func (r *Repository) IsClientAllowed(clientID string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if r.Config.Public {
		return true
	}

	for _, allowed := range r.Config.AllowedClients {
		if allowed == clientID || allowed == "*" {
			return true
		}
	}
	return false
}

// Repository persistence functions

// RepositoryMetadata represents the persistent metadata for a repository
type RepositoryMetadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"`
	Description string            `json:"description"`
	Config      *RepositoryConfig `json:"config"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// loadRepositories loads all existing repositories from storage
func (m *Manager) loadRepositories() error {
	// List all repository metadata files
	const metadataPrefix = "_repositories/"
	
	// For filesystem backend, we need to check if the directories exist
	entries, err := m.listRepositoryMetadataFiles()
	if err != nil {
		return fmt.Errorf("failed to list repository metadata: %w", err)
	}
	
	for _, entry := range entries {
		if err := m.loadSingleRepository(entry); err != nil {
			m.logger.Warn("Failed to load repository", "metadata_path", entry, "error", err)
			continue
		}
		m.logger.Debug("Successfully loaded repository", "metadata_path", entry)
	}
	
	return nil
}

// listRepositoryMetadataFiles lists all repository metadata files by scanning the storage backend
func (m *Manager) listRepositoryMetadataFiles() ([]string, error) {
	// Scan the _repositories directory for all metadata.json files
	const repositoriesPrefix = "_repositories/"
	
	var foundMetadataFiles []string
	
	// Check if the storage backend supports filesystem scanning
	// Handle both direct filesystem backend and retry-wrapped backend
	var fsBackend *storage.FilesystemBackend
	if fs, ok := m.defaultBackend.(*storage.FilesystemBackend); ok {
		fsBackend = fs
	} else if retryBackend, ok := m.defaultBackend.(*storage.RetryBackend); ok {
		if fs, ok := retryBackend.GetUnderlyingBackend().(*storage.FilesystemBackend); ok {
			fsBackend = fs
		}
	}
	
	if fsBackend != nil {
		foundMetadataFiles = m.scanFilesystemForRepositories(fsBackend)
	} else {
		// For non-filesystem backends (S3, etc.), try common patterns
		// This is a fallback approach for backends that don't support directory listing
		foundMetadataFiles = m.scanCommonRepositoryPatterns()
	}
	
	m.logger.Info("Repository discovery completed", "found_count", len(foundMetadataFiles))
	
	return foundMetadataFiles, nil
}

// scanFilesystemForRepositories scans filesystem backend for repository metadata files
func (m *Manager) scanFilesystemForRepositories(fsBackend *storage.FilesystemBackend) []string {
	var foundFiles []string
	
	// Get the base path from filesystem backend
	basePath := fsBackend.GetBasePath()
	repositoriesPath := filepath.Join(basePath, "_repositories")
	
	// Walk the _repositories directory to find all metadata.json files
	err := filepath.Walk(repositoriesPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Continue walking even if some directories are inaccessible
		}
		
		if !info.IsDir() && info.Name() == "metadata.json" {
			// Convert absolute path to relative path for storage backend
			relPath, err := filepath.Rel(basePath, path)
			if err != nil {
				m.logger.Warn("Failed to get relative path", "path", path, "error", err)
				return nil
			}
			
			m.logger.Debug("Found repository metadata", "path", relPath)
			foundFiles = append(foundFiles, relPath)
		}
		
		return nil
	})
	
	if err != nil {
		m.logger.Debug("Repository directory scan completed with errors", "error", err)
	}
	
	return foundFiles
}

// scanCommonRepositoryPatterns checks common repository patterns for non-filesystem backends
func (m *Manager) scanCommonRepositoryPatterns() []string {
	// Common namespace patterns to check as fallback for non-filesystem backends
	namespacesToCheck := []string{"prod", "dev", "test", "staging", "default"}
	repoNamesToCheck := []string{"main", "firmware", "packages", "app", "service"}
	
	var foundMetadataFiles []string
	
	for _, namespace := range namespacesToCheck {
		for _, repoName := range repoNamesToCheck {
			metadataPath := m.getRepositoryMetadataPath(namespace, repoName)
			
			// Check if this metadata file exists
			if exists, _ := m.defaultBackend.Exists(context.Background(), metadataPath); exists {
				m.logger.Debug("Found repository metadata", "path", metadataPath)
				foundMetadataFiles = append(foundMetadataFiles, metadataPath)
			}
		}
	}
	
	return foundMetadataFiles
}

// loadSingleRepository loads a single repository from its metadata file
func (m *Manager) loadSingleRepository(metadataPath string) error {
	reader, err := m.defaultBackend.Get(context.Background(), metadataPath)
	if err != nil {
		return err
	}
	defer reader.Close()
	
	content, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	
	var metadata RepositoryMetadata
	if err := json.Unmarshal(content, &metadata); err != nil {
		return err
	}
	
	// Create repository object from metadata
	backend := m.getBackendForRepo(metadata.Namespace, metadata.Name)
	repo := &Repository{
		Name:        metadata.Name,
		Namespace:   metadata.Namespace,
		Description: metadata.Description,
		Backend:     backend,
		Config:      metadata.Config,
		CreatedAt:   metadata.CreatedAt,
		UpdatedAt:   metadata.UpdatedAt,
	}
	
	repoID := formatRepoID(metadata.Namespace, metadata.Name)
	m.repositories[repoID] = repo
	
	return nil
}

// saveRepositoryMetadata saves repository metadata to storage
func (m *Manager) saveRepositoryMetadata(repo *Repository) error {
	metadata := RepositoryMetadata{
		Name:        repo.Name,
		Namespace:   repo.Namespace,
		Description: repo.Description,
		Config:      repo.Config,
		CreatedAt:   repo.CreatedAt,
		UpdatedAt:   repo.UpdatedAt,
	}
	
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	
	metadataPath := m.getRepositoryMetadataPath(repo.Namespace, repo.Name)
	return m.defaultBackend.Put(context.Background(), metadataPath, bytes.NewReader(data))
}

// deleteRepositoryMetadata removes repository metadata from storage
func (m *Manager) deleteRepositoryMetadata(repo *Repository) error {
	metadataPath := m.getRepositoryMetadataPath(repo.Namespace, repo.Name)
	return m.defaultBackend.Delete(context.Background(), metadataPath)
}

// getRepositoryMetadataPath returns the storage path for repository metadata
func (m *Manager) getRepositoryMetadataPath(namespace, name string) string {
	return fmt.Sprintf("_repositories/%s/%s/metadata.json", namespace, name)
}