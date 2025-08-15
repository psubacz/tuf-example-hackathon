package tuf

import (
	"crypto"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sigstore/sigstore/pkg/signature"
	"github.com/theupdateframework/go-tuf/v2/metadata"
	"tuf-golang-project/internal/logger"
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
	logger.Logger.Info("Generating cryptographic keys for TUF roles")
	keyManager, err := NewKeyManager()
	if err != nil {
		return fmt.Errorf("failed to generate keys: %w", err)
	}
	
	// Create root metadata using go-tuf v2 factory function
	rootMetadata := metadata.Root(expirationTime)
	rootMetadata.Signed.Version = 1

	// Add keys to root metadata using proper go-tuf v2 API
	for roleName, keyPair := range keyManager.GetKeyPairs() {
		err := rootMetadata.Signed.AddKey(keyPair.ToTUFKey(), roleName)
		if err != nil {
			return fmt.Errorf("failed to add key for role %s: %w", roleName, err)
		}
	}

	logger.Logger.Info("Generated cryptographic keys for TUF roles", "count", len(keyManager.GetKeyPairs()))

	// Create targets metadata using factory function
	targetsMetadata := metadata.Targets(expirationTime)
	targetsMetadata.Signed.Version = 1
	targetsMetadata.Signed.Targets = make(map[string]*metadata.TargetFiles)

	// Add sample.txt to targets
	if err := r.addFileToTargetsV2(targetsMetadata, "sample.txt"); err != nil {
		return fmt.Errorf("failed to add sample target: %w", err)
	}

	// Create snapshot metadata using factory function
	snapshotMetadata := metadata.Snapshot(expirationTime)
	snapshotMetadata.Signed.Version = 1
	snapshotMetadata.Signed.Meta = map[string]*metadata.MetaFiles{
		"targets.json": metadata.MetaFile(1),
	}

	// Create timestamp metadata using factory function
	timestampMetadata := metadata.Timestamp(expirationTime)
	timestampMetadata.Signed.Version = 1
	timestampMetadata.Signed.Meta = map[string]*metadata.MetaFiles{
		"snapshot.json": metadata.MetaFile(1),
	}

	// Sign all metadata with their respective keys
	logger.Logger.Info("Signing metadata with generated keys")
	
	// Sign targets metadata
	targetsSigner, err := signature.LoadSigner(keyManager.GetTargetsKey().PrivateKey, crypto.Hash(0))
	if err != nil {
		return fmt.Errorf("failed to create targets signer: %w", err)
	}
	if _, err := targetsMetadata.Sign(targetsSigner); err != nil {
		return fmt.Errorf("failed to sign targets metadata: %w", err)
	}

	// Sign snapshot metadata
	snapshotSigner, err := signature.LoadSigner(keyManager.GetSnapshotKey().PrivateKey, crypto.Hash(0))
	if err != nil {
		return fmt.Errorf("failed to create snapshot signer: %w", err)
	}
	if _, err := snapshotMetadata.Sign(snapshotSigner); err != nil {
		return fmt.Errorf("failed to sign snapshot metadata: %w", err)
	}

	// Sign timestamp metadata
	timestampSigner, err := signature.LoadSigner(keyManager.GetTimestampKey().PrivateKey, crypto.Hash(0))
	if err != nil {
		return fmt.Errorf("failed to create timestamp signer: %w", err)
	}
	if _, err := timestampMetadata.Sign(timestampSigner); err != nil {
		return fmt.Errorf("failed to sign timestamp metadata: %w", err)
	}

	// Sign root metadata
	rootSigner, err := signature.LoadSigner(keyManager.GetRootKey().PrivateKey, crypto.Hash(0))
	if err != nil {
		return fmt.Errorf("failed to create root signer: %w", err)
	}
	if _, err := rootMetadata.Sign(rootSigner); err != nil {
		return fmt.Errorf("failed to sign root metadata: %w", err)
	}

	logger.Logger.Info("All metadata signed successfully")

	// Save all signed metadata
	if err := r.saveMetadataV2("root.json", rootMetadata); err != nil {
		return fmt.Errorf("failed to save signed root metadata: %w", err)
	}
	if err := r.saveMetadataV2("targets.json", targetsMetadata); err != nil {
		return fmt.Errorf("failed to save signed targets metadata: %w", err)
	}
	if err := r.saveMetadataV2("snapshot.json", snapshotMetadata); err != nil {
		return fmt.Errorf("failed to save signed snapshot metadata: %w", err)
	}
	if err := r.saveMetadataV2("timestamp.json", timestampMetadata); err != nil {
		return fmt.Errorf("failed to save signed timestamp metadata: %w", err)
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

	// Calculate real SHA256 hash
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file %s: %w", filename, err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return fmt.Errorf("failed to calculate hash for %s: %w", filename, err)
	}
	hashBytes := hash.Sum(nil)

	// Create target file with real hash and length
	targetFile := metadata.TargetFile()
	targetFile.Length = fileInfo.Size()
	targetFile.Hashes = metadata.Hashes{
		"sha256": hashBytes,
	}

	targets.Signed.Targets[filename] = targetFile
	logger.Logger.Info("Added target file", 
		"filename", filename, 
		"size", fileInfo.Size(), 
		"sha256", hex.EncodeToString(hashBytes))
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