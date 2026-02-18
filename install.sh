#!/bin/bash

# Masterblaster install script for Linux based operating systems.
# Requirements:
# * curl
# * uname
# * /tmp directory

set -e

VERSION="${MB_VERSION:-latest}"
BASE_URL="https://mb.stereos.ai"

# Detect OS
OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux*) OS="linux" ;;
  darwin*) OS="darwin" ;;
  *) echo "Unsupported OS: $OS"; exit 1 ;;
esac

# Detect architecture
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $ARCH"; exit 1 ;;
esac

# Download and install
DOWNLOAD_URL="$BASE_URL/$VERSION/$OS/$ARCH/mb"
INSTALL_DIR="${MB_INSTALL_DIR:-/usr/local/bin}"

echo "Downloading mb $VERSION for $OS/$ARCH ..."
curl -fsSL "$DOWNLOAD_URL" -o /tmp/mb
chmod +x /tmp/mb

echo "Installing to $INSTALL_DIR ..."
sudo mv /tmp/mb "$INSTALL_DIR/mb"

echo "Installed mb version:"
mb version
