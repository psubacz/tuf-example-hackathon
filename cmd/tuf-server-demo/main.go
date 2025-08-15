package main

import (
	"fmt"
	"os"
	"tuf-golang-project/internal/server"
)

// main is the entry point for the TUF Repository Server
func main() {
	// Create CLI instance
	cli := server.NewCLI()

	// Run the CLI with command line arguments
	if err := cli.Run(os.Args); err != nil {
		fmt.Fprintf(os.Stderr, "❌ Error: %v\n", err)
		os.Exit(1)
	}
}
