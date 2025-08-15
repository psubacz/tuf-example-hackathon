package main

import (
	"fmt"
	"log"
	"tuf-golang-project/internal/tuf"
)

func main() {
	fmt.Println("🔐 TUF Repository Demo with go-tuf v2")
	fmt.Println("Creating a production-grade TUF repository using official go-tuf v2 library...")

	// Create repository instance
	repo := tuf.NewRepositoryV2("./tuf-repository-v2")
	
	// Initialize the repository
	if err := repo.Initialize(); err != nil {
		log.Fatalf("Failed to initialize repository: %v", err)
	}

	fmt.Printf("\n🎉 TUF repository with go-tuf v2 created successfully!\n")
	fmt.Printf("Repository location: ./tuf-repository-v2\n")
	fmt.Printf("Metadata files: tuf-repository-v2/metadata\n")
	fmt.Printf("Target files: tuf-repository-v2/targets\n")

	fmt.Printf("\n📝 This demonstrates production-grade TUF implementation:\n")
	fmt.Printf("  - go-tuf v2 metadata structures\n")
	fmt.Printf("  - Official TUF library integration\n")
	fmt.Printf("  - Production-ready metadata format\n")
	fmt.Printf("  - Cryptographic signature framework\n")

	fmt.Printf("\n🔍 Explore the files to see go-tuf v2 metadata structure!\n")
	fmt.Printf("Next: Run 'make run-server' to serve the repository over HTTP\n")
}