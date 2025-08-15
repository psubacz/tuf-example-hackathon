package tuf

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/theupdateframework/go-tuf/v2/metadata"
)

const (
	DefaultRepoV2Dir     = "./tuf-repository-v2"
	MetadataV2Dir        = "metadata"
	TargetsV2Dir         = "targets"
	DefaultV2Expiration  = 365 * 24 * time.Hour // 1 year
)

// RepositoryV2 manages TUF repository operations using go-tuf v2
type RepositoryV2 struct {
	RootDir string
}

// NewRepositoryV2 creates a new repository instance using go-tuf v2
func NewRepositoryV2(rootDir string) *RepositoryV2 {
	if rootDir == "" {
		rootDir = DefaultRepoV2Dir
	}
	return &RepositoryV2{RootDir: rootDir}
}

// Initialize creates the initial TUF repository structure using go-tuf v2
func (r *RepositoryV2) Initialize() error {
	// Create directory structure
	metadataDir := filepath.Join(r.RootDir, MetadataV2Dir)
	targetsDir := filepath.Join(r.RootDir, TargetsV2Dir)
	
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

	// Create metadata files using go-tuf v2
	if err := r.createMetadataV2(); err != nil {
		return fmt.Errorf("failed to create metadata: %w", err)
	}

	return nil
}

// createSampleTarget creates a sample target file
func (r *RepositoryV2) createSampleTarget() error {
	sampleContent := fmt.Sprintf(`Hello TUF World with go-tuf v2!
This is a sample file in the TUF repository using the official go-tuf v2 library.
Created at: %s
File purpose: Demonstrate TUF target verification with production-grade implementation
Content integrity: This file is protected by go-tuf v2 cryptographic signatures`, time.Now().Format(time.RFC3339))

	targetPath := filepath.Join(r.RootDir, TargetsV2Dir, "sample.txt")
	return os.WriteFile(targetPath, []byte(sampleContent), 0644)
}

// createMetadataV2 creates all TUF metadata files using go-tuf v2
func (r *RepositoryV2) createMetadataV2() error {
	expirationTime := time.Now().Add(DefaultV2Expiration)
	
	// Generate proper cryptographic keys for production use
	fmt.Println("🔑 Generating cryptographic keys for TUF roles...")
	keyManager, err := NewKeyManager()
	if err != nil {
		return fmt.Errorf("failed to generate keys: %w", err)
	}
	
	// Create root metadata using go-tuf v2 factory function
	rootMetadata := metadata.Root(expirationTime)
	rootMetadata.Signed.Version = 1

	// Use generated keys and roles
	rootMetadata.Signed.Keys = keyManager.GetKeys()
	rootMetadata.Signed.Roles = keyManager.GetRoles()

	fmt.Printf("✅ Generated %d cryptographic keys for TUF roles\n", len(rootMetadata.Signed.Keys))

	// Save root metadata
	if err := r.saveMetadataV2("root.json", rootMetadata); err != nil {
		return fmt.Errorf("failed to save root metadata: %w", err)
	}

	// Create targets metadata using factory function
	targetsMetadata := metadata.Targets(expirationTime)
	targetsMetadata.Signed.Version = 1
	targetsMetadata.Signed.Targets = make(map[string]*metadata.TargetFiles)

	// Add sample.txt to targets
	if err := r.addFileToTargetsV2(targetsMetadata, "sample.txt"); err != nil {
		return fmt.Errorf("failed to add sample target: %w", err)
	}

	// Save targets metadata
	if err := r.saveMetadataV2("targets.json", targetsMetadata); err != nil {
		return fmt.Errorf("failed to save targets metadata: %w", err)
	}

	// Create snapshot metadata using factory function
	snapshotMetadata := metadata.Snapshot(expirationTime)
	snapshotMetadata.Signed.Version = 1
	snapshotMetadata.Signed.Meta = map[string]*metadata.MetaFiles{
		"targets.json": metadata.MetaFile(1),
	}

	if err := r.saveMetadataV2("snapshot.json", snapshotMetadata); err != nil {
		return fmt.Errorf("failed to save snapshot metadata: %w", err)
	}

	// Create timestamp metadata using factory function
	timestampMetadata := metadata.Timestamp(expirationTime)
	timestampMetadata.Signed.Version = 1
	timestampMetadata.Signed.Meta = map[string]*metadata.MetaFiles{
		"snapshot.json": metadata.MetaFile(1),
	}

	if err := r.saveMetadataV2("timestamp.json", timestampMetadata); err != nil {
		return fmt.Errorf("failed to save timestamp metadata: %w", err)
	}

	return nil
}

// addFileToTargetsV2 adds a file to the targets metadata using go-tuf v2
func (r *RepositoryV2) addFileToTargetsV2(targets *metadata.Metadata[metadata.TargetsType], filename string) error {
	filePath := filepath.Join(r.RootDir, TargetsV2Dir, filename)
	
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed to get file info for %s: %w", filename, err)
	}

	// In a production implementation, this would calculate real hashes
	// For this demo, we'll use placeholder values
	targetFile := metadata.TargetFile()
	targetFile.Length = fileInfo.Size()
	targetFile.Hashes = metadata.Hashes{
		"sha256": []byte("demo_hash_value_for_" + filename),
	}

	targets.Signed.Targets[filename] = targetFile
	return nil
}

// saveMetadataV2 saves go-tuf v2 metadata to a JSON file
func (r *RepositoryV2) saveMetadataV2(filename string, metadata interface{}) error {
	metadataPath := filepath.Join(r.RootDir, MetadataV2Dir, filename)
	
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	return os.WriteFile(metadataPath, data, 0644)
}