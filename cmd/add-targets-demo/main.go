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
	fmt.Println("📦 Adding More Targets to TUF Repository")
	
	repoDir := "./tuf-repository"
	if _, err := os.Stat(repoDir); os.IsNotExist(err) {
		fmt.Println("❌ TUF repository not found!")
		fmt.Println("Please run 'go run cmd/tuf-demo' first to create the repository.")
		os.Exit(1)
	}
	
	// Create additional target files
	targetsDir := filepath.Join(repoDir, "targets")
	
	additionalTargets := map[string]string{
		"config.json": `{
  "version": "2.0.0",
  "debug": false,
  "features": ["security", "performance", "scalability"],
  "database": {
    "host": "localhost",
    "port": 5432
  }
}`,
		"update.txt": fmt.Sprintf(`Application Update Log
======================
Updated at: %s
Version: 2.0.0
Changes:
- Added new security features
- Improved performance by 25%%
- Fixed critical vulnerabilities
- Enhanced user interface

This update is recommended for all users.
`, time.Now().Format(time.RFC3339)),
		"data.csv": `name,value,category,date
item1,100,A,2025-01-01
item2,200,B,2025-01-02
item3,300,C,2025-01-03
item4,150,A,2025-01-04
item5,250,B,2025-01-05`,
		"readme.md": `# TUF Protected Application

This application is secured using The Update Framework (TUF).

## Security Features
- Cryptographic signatures on all files
- Protection against replay attacks
- Secure key distribution
- Integrity verification

## Version Information
- Current Version: 2.0.0
- Last Updated: ` + time.Now().Format("2006-01-02") + `

For more information about TUF, visit: https://theupdateframework.io/
`,
	}
	
	fmt.Printf("Creating %d additional target files...\n", len(additionalTargets))
	
	// Create the files
	for filename, content := range additionalTargets {
		filepath := filepath.Join(targetsDir, filename)
		if err := os.WriteFile(filepath, []byte(content), 0644); err != nil {
			panic(fmt.Sprintf("Failed to create %s: %v", filename, err))
		}
		fmt.Printf("✅ Created: %s (%d bytes)\n", filename, len(content))
	}
	
	// Load existing targets metadata
	targetsMetadataPath := filepath.Join(repoDir, "metadata", "targets.json")
	targetsData, err := os.ReadFile(targetsMetadataPath)
	if err != nil {
		panic(fmt.Sprintf("Failed to read targets.json: %v", err))
	}
	
	var targets tuf.Targets
	if err := json.Unmarshal(targetsData, &targets); err != nil {
		panic(fmt.Sprintf("Failed to parse targets.json: %v", err))
	}
	
	// Add new targets to metadata
	fmt.Println("\n📝 Updating targets metadata...")
	
	for filename := range additionalTargets {
		filePath := filepath.Join(targetsDir, filename)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			panic(fmt.Sprintf("Failed to stat %s: %v", filename, err))
		}
		
		// In production, you'd calculate actual cryptographic hashes
		targets.Targets[filename] = tuf.TargetInfo{
			Length: int(fileInfo.Size()),
			Hashes: map[string]string{
				"sha256": fmt.Sprintf("demo_hash_value_for_%s", filename),
			},
		}
		fmt.Printf("✅ Added to metadata: %s\n", filename)
	}
	
	// Update version and expiry
	targets.Version += 1
	targets.Expires = time.Now().AddDate(1, 0, 0).Format(time.RFC3339)
	
	// Save updated metadata
	updatedData, err := json.MarshalIndent(targets, "", "  ")
	if err != nil {
		panic(fmt.Sprintf("Failed to marshal updated targets: %v", err))
	}
	
	if err := os.WriteFile(targetsMetadataPath, updatedData, 0644); err != nil {
		panic(fmt.Sprintf("Failed to write updated targets.json: %v", err))
	}
	
	fmt.Printf("\n🎉 Successfully added %d targets to the repository!\n", len(additionalTargets))
	fmt.Printf("Targets metadata updated to version %d\n", targets.Version)
	fmt.Printf("Total targets now available: %d\n", len(targets.Targets))
	
	fmt.Println("\n📋 All available targets:")
	for filename, info := range targets.Targets {
		fmt.Printf("  - %s (%d bytes)\n", filename, info.Length)
	}
	
	fmt.Println("\n🔍 Run 'go run cmd/tuf-client-demo' to verify all targets!")
}