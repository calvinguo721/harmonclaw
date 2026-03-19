#!/bin/bash
# HarmonClaw install script - Linux/macOS
set -e
VERSION="${HC_VERSION:-latest}"
INSTALL_DIR="${HC_INSTALL_DIR:-/usr/local/bin}"
BINARY="harmonclaw"

echo "HarmonClaw installer v1.0"
echo "Install dir: $INSTALL_DIR"

if [ "$VERSION" = "latest" ]; then
  echo "Building from source..."
  if ! command -v go &>/dev/null; then
    echo "Error: Go not found. Install Go 1.22+ or set HC_BINARY to pre-built path."
    exit 1
  fi
  cd "$(dirname "$0")/.."
  CGO_ENABLED=0 go build -o "${INSTALL_DIR}/${BINARY}" ./cmd/harmonclaw/
  echo "Built and installed to ${INSTALL_DIR}/${BINARY}"
else
  echo "Download from GitHub Releases not implemented. Use: go build ./cmd/harmonclaw/"
  exit 1
fi

echo "Done. Run: $BINARY or $INSTALL_DIR/$BINARY"
