.PHONY: setup init-repo run-client run-server run-network-client clean help all

# Setup project dependencies
setup:
	go mod tidy
	go mod download

# Initialize the TUF repository
init-repo:
	go run ./cmd/tuf-demo

# Run the local TUF client example
run-client:
	go run ./cmd/tuf-client-demo

# Start the TUF repository server
run-server:
	go run ./cmd/tuf-server-demo

# Run the network TUF client
run-network-client:
	go run ./cmd/network-client-demo

# Add more targets to repository
add-targets:
	go run ./cmd/add-targets-demo

# Clean generated files
clean:
	rm -rf tuf-repository/
	rm -rf client-cache/
	rm -rf network-client-cache/

# Test the complete over-the-air update workflow
test-ota:
	@echo "🚀 Testing Over-the-Air Updates..."
	@echo "1. Creating repository..."
	@go run ./cmd/tuf-demo
	@echo "2. Adding more targets..."
	@go run ./cmd/add-targets-demo
	@echo "3. Starting server in background..."
	@go run ./cmd/tuf-server-demo --port 8080 &
	@echo "4. Waiting for server to start..."
	@sleep 3
	@echo "5. Running network client..."
	@go run ./cmd/network-client-demo
	@echo "6. Stopping server..."
	@pkill -f "go run ./cmd/tuf-server-demo" || true
	@echo "✅ Over-the-air update test completed!"

# Show available commands
help:
	@echo "TUF Golang Project - Available Commands:"
	@echo "  setup           - Download Go dependencies"
	@echo "  init-repo       - Initialize TUF repository"
	@echo "  run-client      - Run local TUF client example"
	@echo "  run-server      - Start TUF repository server"
	@echo "  run-network-client - Run network TUF client"
	@echo "  add-targets     - Add more files to repository"
	@echo "  test-ota        - Test complete over-the-air workflow"
	@echo "  clean           - Remove generated files"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "📖 For complete guide including deployment, see README.md"
	@echo "🌐 For network access: go run ./cmd/tuf-server-demo then go run ./cmd/network-client-demo"

# Default target
all: setup init-repo add-targets
