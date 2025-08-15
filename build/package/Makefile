# Container Build Makefile
# Provides convenient commands for building and managing TUF containers

.PHONY: build build-dev build-prod test clean push pull up down logs help

# Configuration
PROJECT_NAME := tuf-golang-project
VERSION ?= latest
REGISTRY ?= 
COMPOSE_FILE := podman-compose.yml
COMPOSE_PROD_FILE := podman-compose.prod.yml

# Build all containers
build:
	@echo "🐳 Building all TUF containers..."
	chmod +x build.sh
	./build.sh

# Build for development
build-dev:
	@echo "🔧 Building development containers..."
	podman-compose -f $(COMPOSE_FILE) build

# Build for production
build-prod:
	@echo "🚀 Building production containers..."
	podman-compose -f $(COMPOSE_PROD_FILE) build

# Test containers
test:
	@echo "🧪 Testing container stack..."
	podman-compose -f $(COMPOSE_FILE) config
	podman-compose -f $(COMPOSE_FILE) up -d
	@echo "Waiting for services to start..."
	sleep 30
	@echo "Testing TUF server health..."
	curl -f http://localhost:8080/health || (echo "❌ Health check failed" && exit 1)
	@echo "✅ All tests passed"
	podman-compose -f $(COMPOSE_FILE) down

# Clean up containers and volumes
clean:
	@echo "🧹 Cleaning up containers and volumes..."
	podman-compose -f $(COMPOSE_FILE) down -v --remove-orphans
	podman-compose -f $(COMPOSE_PROD_FILE) down -v --remove-orphans
	podman system prune -f
	podman volume prune -f

# Push images to registry
push:
	@echo "📤 Pushing images to registry..."
	@if [ -z "$(REGISTRY)" ]; then \
		echo "❌ REGISTRY variable not set. Use: make push REGISTRY=your-registry.com/"; \
		exit 1; \
	fi
	podman push $(REGISTRY)tuf-server:$(VERSION)
	podman push $(REGISTRY)tuf-network-client:$(VERSION)
	podman push $(REGISTRY)tuf-demo:$(VERSION)
	podman push $(REGISTRY)tuf-client:$(VERSION)
	podman push $(REGISTRY)tuf-add-targets:$(VERSION)

# Pull images from registry
pull:
	@echo "📥 Pulling images from registry..."
	@if [ -z "$(REGISTRY)" ]; then \
		echo "❌ REGISTRY variable not set. Use: make pull REGISTRY=your-registry.com/"; \
		exit 1; \
	fi
	podman pull $(REGISTRY)tuf-server:$(VERSION)
	podman pull $(REGISTRY)tuf-network-client:$(VERSION)

# Start development stack
up:
	@echo "🚀 Starting TUF development stack..."
	podman-compose -f $(COMPOSE_FILE) up -d
	@echo "✅ Stack started! Access TUF server at http://localhost:8080"

# Start production stack
up-prod:
	@echo "🚀 Starting TUF production stack..."
	podman-compose -f $(COMPOSE_PROD_FILE) up -d
	@echo "✅ Production stack started!"

# Stop all services
down:
	@echo "🛑 Stopping TUF stack..."
	podman-compose -f $(COMPOSE_FILE) down
	podman-compose -f $(COMPOSE_PROD_FILE) down

# View logs
logs:
	@echo "📋 Viewing container logs..."
	podman-compose -f $(COMPOSE_FILE) logs -f

# View production logs
logs-prod:
	@echo "📋 Viewing production container logs..."
	podman-compose -f $(COMPOSE_PROD_FILE) logs -f

# Show running containers
ps:
	@echo "📊 Container status:"
	podman-compose -f $(COMPOSE_FILE) ps
	@echo ""
	@echo "📊 Production container status:"
	podman-compose -f $(COMPOSE_PROD_FILE) ps

# Health check
health:
	@echo "🏥 Checking container health..."
	@curl -s http://localhost:8080/health | jq . || echo "❌ Server not responding"

# Restart services
restart:
	@echo "🔄 Restarting TUF services..."
	podman-compose -f $(COMPOSE_FILE) restart

# Update containers
update:
	@echo "🔄 Updating containers..."
	make down
	make build
	make up

# Show help
help:
	@echo "TUF Container Management Commands:"
	@echo ""
	@echo "Build Commands:"
	@echo "  build         - Build all containers"
	@echo "  build-dev     - Build development containers"
	@echo "  build-prod    - Build production containers"
	@echo ""
	@echo "Runtime Commands:"
	@echo "  up            - Start development stack"
	@echo "  up-prod       - Start production stack"
	@echo "  down          - Stop all services"
	@echo "  restart       - Restart services"
	@echo "  ps            - Show container status"
	@echo ""
	@echo "Maintenance Commands:"
	@echo "  test          - Test container stack"
	@echo "  clean         - Clean up containers and volumes"
	@echo "  logs          - View container logs"
	@echo "  logs-prod     - View production logs"
	@echo "  health        - Check service health"
	@echo ""
	@echo "Registry Commands:"
	@echo "  push          - Push images to registry"
	@echo "  pull          - Pull images from registry"
	@echo "  update        - Update and restart containers"
	@echo ""
	@echo "Examples:"
	@echo "  make build"
	@echo "  make up"
	@echo "  make push REGISTRY=ghcr.io/username/ VERSION=v1.0.0"
	@echo "  make logs"

# Default target
.DEFAULT_GOAL := help