#!/bin/bash
# Cross-platform build script for Remote Agent Terminal

set -e

APP_NAME="remote-agent-terminal"
VERSION="${VERSION:-$(git describe --tags --always --dirty 2>/dev/null || echo "dev")}"
BUILD_TIME=$(date -u '+%Y-%m-%d_%H:%M:%S')
LDFLAGS="-X main.Version=${VERSION} -X main.BuildTime=${BUILD_TIME}"

DIST_DIR="./dist"
CMD_DIR="./cmd/server"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# Create dist directory
mkdir -p "${DIST_DIR}"

# Build function
build_target() {
    local os=$1
    local arch=$2
    local ext=$3
    local output="${DIST_DIR}/${APP_NAME}-${os}-${arch}${ext}"
    
    log_info "Building for ${os}/${arch}..."
    
    # CGO is required for sqlite3
    CGO_ENABLED=1 GOOS="${os}" GOARCH="${arch}" go build \
        -ldflags "${LDFLAGS}" \
        -o "${output}" \
        "${CMD_DIR}"
    
    if [ $? -eq 0 ]; then
        log_info "Successfully built: ${output}"
    else
        log_error "Failed to build for ${os}/${arch}"
        return 1
    fi
}

# Parse arguments
TARGET="${1:-current}"

case "${TARGET}" in
    "current")
        log_info "Building for current platform..."
        mkdir -p ./bin
        CGO_ENABLED=1 go build -ldflags "${LDFLAGS}" -o "./bin/${APP_NAME}" "${CMD_DIR}"
        log_info "Built: ./bin/${APP_NAME}"
        ;;
    "linux")
        build_target "linux" "amd64" ""
        build_target "linux" "arm64" ""
        ;;
    "windows")
        build_target "windows" "amd64" ".exe"
        ;;
    "darwin")
        build_target "darwin" "amd64" ""
        build_target "darwin" "arm64" ""
        ;;
    "all")
        log_info "Building for all platforms..."
        build_target "linux" "amd64" ""
        build_target "linux" "arm64" ""
        build_target "darwin" "amd64" ""
        build_target "darwin" "arm64" ""
        build_target "windows" "amd64" ".exe"
        ;;
    *)
        log_error "Unknown target: ${TARGET}"
        echo "Usage: $0 [current|linux|windows|darwin|all]"
        exit 1
        ;;
esac

log_info "Build complete!"
