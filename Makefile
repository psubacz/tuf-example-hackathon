.PHONY: setup run-server run-client clean help

# Setup project dependencies
setup:
	go mod tidy
	go mod download

# Initialize the TUF repository
init-repo:
	go run main.go

# Run the TUF client example
run-client:
	go run client.go

# Clean generated files
clean:
	rm -rf tuf-repository/
	rm -rf client-cache/

# Show available commands
help:
	@echo "Available commands:"
	@echo "  setup      - Download Go dependencies"
	@echo "  init-repo  - Initialize TUF repository"
	@echo "  run-client - Run TUF client example"
	@echo "  clean      - Remove generated files"
	@echo "  help       - Show this help message"

# Default target
all: setup init-repo run-client
