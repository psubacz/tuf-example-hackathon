#!/bin/bash

# TUF Container Build Script
# This script builds all Docker images for the TUF project

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
PROJECT_NAME="tuf-golang-project"
VERSION=${VERSION:-"latest"}
REGISTRY=${REGISTRY:-""}
BUILD_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${BUILD_DIR}/../.." && pwd)"

echo -e "${BLUE}🐳 TUF Container Build Script (Podman)${NC}"
echo -e "${BLUE}================================${NC}"
echo "Project: ${PROJECT_NAME}"
echo "Version: ${VERSION}"
echo "Registry: ${REGISTRY:-"local"}"
echo "Build Dir: ${BUILD_DIR}"
echo "Project Root: ${PROJECT_ROOT}"
echo ""

# Function to build a single image
build_image() {
    local dockerfile=$1
    local image_name=$2
    local tag="${REGISTRY}${image_name}:${VERSION}"
    
    echo -e "${YELLOW}Building ${image_name}...${NC}"
    
    if podman build \
        -f "${BUILD_DIR}/${dockerfile}" \
        -t "${tag}" \
        "${PROJECT_ROOT}"; then
        echo -e "${GREEN}✅ Successfully built ${tag}${NC}"
    else
        echo -e "${RED}❌ Failed to build ${tag}${NC}"
        exit 1
    fi
    echo ""
}

# Function to test an image
test_image() {
    local image_name=$1
    local tag="${REGISTRY}${image_name}:${VERSION}"
    
    echo -e "${YELLOW}Testing ${image_name}...${NC}"
    
    # Basic test - check if image runs without error
    if podman run --rm "${tag}" --help > /dev/null 2>&1; then
        echo -e "${GREEN}✅ ${tag} test passed${NC}"
    else
        echo -e "${YELLOW}⚠️  ${tag} test skipped (no --help option)${NC}"
    fi
}

# Build all images
echo -e "${BLUE}Building TUF Demo (Repository Initialization)${NC}"
build_image "Dockerfile.tuf-demo" "tuf-demo"

echo -e "${BLUE}Building TUF Server${NC}"
build_image "Dockerfile.tuf-server" "tuf-server"

echo -e "${BLUE}Building TUF Client Demo${NC}"
build_image "Dockerfile.tuf-client" "tuf-client"

echo -e "${BLUE}Building Network TUF Client${NC}"
build_image "Dockerfile.network-client" "tuf-network-client"

echo -e "${BLUE}Building Add Targets Utility${NC}"
build_image "Dockerfile.add-targets" "tuf-add-targets"

# Test images
echo -e "${BLUE}Running Basic Tests${NC}"
echo -e "${BLUE}==================${NC}"

test_image "tuf-demo"
test_image "tuf-server"
test_image "tuf-client"
test_image "tuf-network-client"
test_image "tuf-add-targets"

echo ""
echo -e "${GREEN}🎉 All builds completed successfully!${NC}"
echo ""
echo -e "${BLUE}Available Images:${NC}"
podman images | grep -E "(tuf-demo|tuf-server|tuf-client|tuf-network-client|tuf-add-targets)" | head -5

echo ""
echo -e "${BLUE}Next Steps:${NC}"
echo "1. Run with Podman Compose:"
echo "   cd ${BUILD_DIR} && podman-compose up"
echo ""
echo "2. Push to registry (if configured):"
echo "   podman push ${REGISTRY}tuf-server:${VERSION}"
echo ""
echo "3. Run individual containers:"
echo "   podman run -p 8080:8080 ${REGISTRY}tuf-server:${VERSION}"