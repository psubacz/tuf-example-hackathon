package main

import (
	"tuf-golang-project/internal/logger"
	"tuf-golang-project/internal/tuf"
)

func main() {
	logger.Logger.Info("TUF Repository Demo with go-tuf v2")
	logger.Logger.Info("Creating a production-grade TUF repository using official go-tuf v2 library")

	// Create repository instance
	repo := tuf.NewRepositoryV2("./tuf-repository-v2")
	
	// Initialize the repository
	if err := repo.Initialize(); err != nil {
		logger.Logger.Error("Failed to initialize repository", "error", err)
		return
	}

	logger.Logger.Info("TUF repository with go-tuf v2 created successfully",
		"location", "./tuf-repository-v2",
		"metadata_dir", "tuf-repository-v2/metadata",
		"targets_dir", "tuf-repository-v2/targets")

	logger.Logger.Info("Production-grade TUF implementation features",
		"features", []string{
			"go-tuf v2 metadata structures",
			"Official TUF library integration", 
			"Production-ready metadata format",
			"Cryptographic signature framework",
		})

	logger.Logger.Info("Next step: Run 'make run-server' to serve the repository over HTTP")
}