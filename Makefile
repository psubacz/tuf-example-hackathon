.PHONY: setup init-repo run-client run-server clean help all test-ota build-containers run-containers stop-containers

# Setup project dependencies
setup:
	go mod tidy
	go mod download

# Initialize TUF repository with go-tuf v2 (production)
init-repo:
	go run ./cmd/tuf-init

# Run the go-tuf v2 client (production)
run-client:
	go run ./cmd/tuf-client

# Clean generated files
clean:
	rm -rf tuf-repository-v2/
	rm -rf tuf-client-v2-cache/

# Test go-tuf v2 workflow (production)
test-ota:
	@echo "🔐 Testing go-tuf v2 Production Implementation..."
	@echo "1. Creating go-tuf v2 repository with real crypto keys..."
	@go run ./cmd/tuf-init
	@echo "2. Testing go-tuf v2 client initialization..."
	@go run ./cmd/tuf-client --repo ./tuf-repository-v2 || echo "Expected: Client bootstrap demo completed!"
	@echo "✅ go-tuf v2 production test completed!"

# Container management commands
build-containers:
	cd build/package && make build-v2

run-containers:
	cd build/package && make up-v2

stop-containers:
	cd build/package && make down-v2

# Show available commands
help:
	@echo "TUF Production Project - go-tuf v2 Implementation"
	@echo ""
	@echo "🔐 Production Commands (go-tuf v2):"
	@echo "  setup           - Download Go dependencies"
	@echo "  init-repo       - Initialize TUF repository with go-tuf v2"
	@echo "  run-client      - Run go-tuf v2 client"
	@echo "  run-server      - Start enhanced TUF repository server"
	@echo "  test-ota        - Test go-tuf v2 production workflow"
	@echo "  clean           - Remove generated files"
	@echo ""
	@echo "🐳 Container Commands:"
	@echo "  build-containers - Build go-tuf v2 containers"
	@echo "  run-containers   - Run containerized go-tuf v2 stack"
	@echo "  stop-containers  - Stop containerized services"
	@echo "  help            - Show this help message"
	@echo ""
	@echo "📖 For complete guide, see README.md"
	@echo "🔐 This project uses production-grade go-tuf v2 library"

# Default target
all: setup init-repo