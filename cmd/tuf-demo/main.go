package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"tuf-golang-project/pkg/tuf"
)

func main() {
	fmt.Println("🔐 Simple TUF Repository Demo")
	fmt.Println("Creating a basic TUF repository structure...")
	
	// Create repository directory
	repoDir := "./tuf-repository"
	if err := os.MkdirAll(repoDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create repository directory: %v", err))
	}
	
	// Create targets directory
	targetsDir := filepath.Join(repoDir, "targets")
	if err := os.MkdirAll(targetsDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create targets directory: %v", err))
	}
	
	// Create metadata directory
	metadataDir := filepath.Join(repoDir, "metadata")
	if err := os.MkdirAll(metadataDir, 0755); err != nil {
		panic(fmt.Sprintf("Failed to create metadata directory: %v", err))
	}
	
	// Create a sample target file
	sampleFile := filepath.Join(targetsDir, "sample.txt")
	sampleContent := fmt.Sprintf("Hello from TUF! Created at %s\n", time.Now().Format(time.RFC3339))
	if err := os.WriteFile(sampleFile, []byte(sampleContent), 0644); err != nil {
		panic(fmt.Sprintf("Failed to create sample file: %v", err))
	}
	fmt.Printf("✅ Created target file: sample.txt\n")
	
	// Calculate file hash (simplified - just using length for demo)
	fileInfo, _ := os.Stat(sampleFile)
	fileSize := int(fileInfo.Size())
	
	// Create TUF metadata
	expires := time.Now().AddDate(1, 0, 0).Format(time.RFC3339)
	
	// Root metadata
	root := tuf.Root{
		Type:    "root",
		Version: 1,
		Expires: expires,
		Keys: map[string]tuf.Key{
			"key1": {
				KeyType: "ed25519",
				Scheme:  "ed25519",
				KeyVal: struct {
					Public string `json:"public"`
				}{Public: "demo_public_key_data"},
			},
		},
		Roles: map[string]tuf.Role{
			"root": {
				KeyIDs:    []string{"key1"},
				Threshold: 1,
			},
			"targets": {
				KeyIDs:    []string{"key1"},
				Threshold: 1,
			},
			"snapshot": {
				KeyIDs:    []string{"key1"},
				Threshold: 1,
			},
			"timestamp": {
				KeyIDs:    []string{"key1"},
				Threshold: 1,
			},
		},
	}
	
	// Targets metadata
	targets := tuf.Targets{
		Type:    "targets",
		Version: 1,
		Expires: expires,
		Targets: map[string]tuf.TargetInfo{
			"sample.txt": {
				Length: fileSize,
				Hashes: map[string]string{
					"sha256": "demo_hash_value_for_sample_txt",
				},
			},
		},
	}
	
	// Snapshot metadata
	snapshot := tuf.Snapshot{
		Type:    "snapshot",
		Version: 1,
		Expires: expires,
		Meta: map[string]struct {
			Version int `json:"version"`
		}{
			"targets.json": {Version: 1},
		},
	}
	
	// Timestamp metadata
	timestamp := tuf.Timestamp{
		Type:    "timestamp",
		Version: 1,
		Expires: expires,
		Meta: map[string]struct {
			Version int `json:"version"`
		}{
			"snapshot.json": {Version: 1},
		},
	}
	
	// Save metadata files
	metadata := map[string]interface{}{
		"root.json":      root,
		"targets.json":   targets,
		"snapshot.json":  snapshot,
		"timestamp.json": timestamp,
	}
	
	for filename, data := range metadata {
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			panic(fmt.Sprintf("Failed to marshal %s: %v", filename, err))
		}
		
		filepath := filepath.Join(metadataDir, filename)
		if err := os.WriteFile(filepath, jsonData, 0644); err != nil {
			panic(fmt.Sprintf("Failed to write %s: %v", filename, err))
		}
		fmt.Printf("✅ Created metadata: %s\n", filename)
	}
	
	fmt.Printf("\n🎉 Simple TUF repository created successfully!\n")
	fmt.Printf("Repository location: %s\n", repoDir)
	fmt.Printf("Metadata files: %s\n", metadataDir)
	fmt.Printf("Target files: %s\n", targetsDir)
	fmt.Printf("\n📝 This demonstrates TUF concepts:\n")
	fmt.Printf("  - Root metadata defines keys and roles\n")
	fmt.Printf("  - Targets metadata lists available files with hashes\n")
	fmt.Printf("  - Snapshot metadata provides consistency\n")
	fmt.Printf("  - Timestamp metadata provides freshness\n")
	fmt.Printf("\n🔍 Explore the files to see TUF metadata structure!\n")
	fmt.Printf("Next: Run 'go run cmd/tuf-client-demo' to see a simple verification example\n")
}