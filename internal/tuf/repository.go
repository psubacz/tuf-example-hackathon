package tuf

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"tuf-golang-project/pkg/tuf"
)

const (
	DefaultRepoDir     = "./tuf-repository"
	MetadataDir        = "metadata"
	TargetsDir         = "targets"
	DefaultExpiration  = 365 * 24 * time.Hour // 1 year
)

// Repository manages TUF repository operations
type Repository struct {
	RootDir string
}

// NewRepository creates a new repository instance
func NewRepository(rootDir string) *Repository {
	if rootDir == "" {
		rootDir = DefaultRepoDir
	}
	return &Repository{RootDir: rootDir}
}

// Initialize creates the initial TUF repository structure
func (r *Repository) Initialize() error {
	// Create directory structure
	metadataDir := filepath.Join(r.RootDir, MetadataDir)
	targetsDir := filepath.Join(r.RootDir, TargetsDir)
	
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		return fmt.Errorf("failed to create metadata directory: %w", err)
	}
	
	if err := os.MkdirAll(targetsDir, 0755); err != nil {
		return fmt.Errorf("failed to create targets directory: %w", err)
	}

	// Create initial target file
	if err := r.createSampleTarget(); err != nil {
		return fmt.Errorf("failed to create sample target: %w", err)
	}

	// Create metadata files
	if err := r.createMetadata(); err != nil {
		return fmt.Errorf("failed to create metadata: %w", err)
	}

	return nil
}

// createSampleTarget creates a sample target file
func (r *Repository) createSampleTarget() error {
	sampleContent := fmt.Sprintf(`Hello TUF World!
This is a sample file in the TUF repository.
Created at: %s
File purpose: Demonstrate TUF target verification
Content integrity: This file is protected by TUF metadata`, time.Now().Format(time.RFC3339))

	targetPath := filepath.Join(r.RootDir, TargetsDir, "sample.txt")
	return os.WriteFile(targetPath, []byte(sampleContent), 0644)
}

// createMetadata creates all TUF metadata files
func (r *Repository) createMetadata() error {
	expirationTime := time.Now().Add(DefaultExpiration).Format(time.RFC3339)
	
	// Create targets metadata first (needed for snapshot)
	targets := tuf.Targets{
		Type:    "targets",
		Version: 1,
		Expires: expirationTime,
		Targets: make(map[string]tuf.TargetInfo),
	}

	// Add sample.txt to targets
	if err := r.addFileToTargets(&targets, "sample.txt"); err != nil {
		return fmt.Errorf("failed to add sample target: %w", err)
	}

	// Save targets metadata
	if err := r.saveMetadata("targets.json", targets); err != nil {
		return fmt.Errorf("failed to save targets metadata: %w", err)
	}

	// Create root metadata
	root := tuf.Root{
		Type:    "root",
		Version: 1,
		Expires: expirationTime,
		Keys: map[string]tuf.Key{
			"ed25519key": {
				KeyType: "ed25519",
				Scheme:  "ed25519",
				KeyVal: struct {
					Public string `json:"public"`
				}{Public: "demo_public_key_for_educational_purposes_only"},
			},
		},
		Roles: map[string]tuf.Role{
			"root":      {KeyIDs: []string{"ed25519key"}, Threshold: 1},
			"targets":   {KeyIDs: []string{"ed25519key"}, Threshold: 1},
			"snapshot":  {KeyIDs: []string{"ed25519key"}, Threshold: 1},
			"timestamp": {KeyIDs: []string{"ed25519key"}, Threshold: 1},
		},
	}

	if err := r.saveMetadata("root.json", root); err != nil {
		return fmt.Errorf("failed to save root metadata: %w", err)
	}

	// Create snapshot metadata
	snapshot := tuf.Snapshot{
		Type:    "snapshot",
		Version: 1,
		Expires: expirationTime,
		Meta: map[string]struct {
			Version int `json:"version"`
		}{
			"targets.json": {Version: 1},
		},
	}

	if err := r.saveMetadata("snapshot.json", snapshot); err != nil {
		return fmt.Errorf("failed to save snapshot metadata: %w", err)
	}

	// Create timestamp metadata
	timestamp := tuf.Timestamp{
		Type:    "timestamp",
		Version: 1,
		Expires: expirationTime,
		Meta: map[string]struct {
			Version int `json:"version"`
		}{
			"snapshot.json": {Version: 1},
		},
	}

	if err := r.saveMetadata("timestamp.json", timestamp); err != nil {
		return fmt.Errorf("failed to save timestamp metadata: %w", err)
	}

	return nil
}

// addFileToTargets adds a file to the targets metadata
func (r *Repository) addFileToTargets(targets *tuf.Targets, filename string) error {
	filePath := filepath.Join(r.RootDir, TargetsDir, filename)
	
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	// Calculate file hash and size
	hasher := sha256.New()
	size, err := io.Copy(hasher, file)
	if err != nil {
		return fmt.Errorf("failed to read file %s: %w", filename, err)
	}

	hash := fmt.Sprintf("%x", hasher.Sum(nil))
	
	targets.Targets[filename] = tuf.TargetInfo{
		Length: int(size),
		Hashes: map[string]string{
			"sha256": hash,
		},
	}

	return nil
}

// saveMetadata saves metadata to a JSON file
func (r *Repository) saveMetadata(filename string, metadata interface{}) error {
	metadataPath := filepath.Join(r.RootDir, MetadataDir, filename)
	
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return os.WriteFile(metadataPath, data, 0644)
}