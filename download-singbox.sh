#!/bin/bash
# Download sing-box for Linux/macOS
# This script downloads the latest sing-box release from GitHub

VERSION="1.13.14"
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

# Map architecture names
case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo "Unsupported architecture: $ARCH"
        exit 1
        ;;
esac

FILENAME="sing-box-${VERSION}-${OS}-${ARCH}.tar.gz"
DOWNLOAD_URL="https://github.com/SagerNet/sing-box/releases/download/v${VERSION}/${FILENAME}"
BIN_DIR="bin"
TEMP_DIR="/tmp/singbox-download-$$"

echo "========================================"
echo "  sing-box Downloader for OneProxy"
echo "========================================"
echo ""
echo "Version: $VERSION"
echo "Platform: $OS-$ARCH"
echo "Target: $BIN_DIR/sing-box"
echo ""

# Create bin directory
mkdir -p "$BIN_DIR"

# Check if sing-box already exists
if [ -f "$BIN_DIR/sing-box" ]; then
    read -p "sing-box already exists. Overwrite? (y/n): " OVERWRITE
    if [ "$OVERWRITE" != "y" ]; then
        echo "Aborted."
        exit 0
    fi
fi

echo ""
echo "Downloading sing-box v$VERSION..."
echo "From: $DOWNLOAD_URL"
echo ""

# Download
mkdir -p "$TEMP_DIR"
if ! curl -L -o "$TEMP_DIR/$FILENAME" "$DOWNLOAD_URL"; then
    echo "ERROR: Download failed!"
    rm -rf "$TEMP_DIR"
    exit 1
fi

echo ""
echo "Extracting..."

# Extract
if ! tar -xzf "$TEMP_DIR/$FILENAME" -C "$TEMP_DIR"; then
    echo "ERROR: Extraction failed!"
    rm -rf "$TEMP_DIR"
    exit 1
fi

# Find and copy sing-box
SINGBOX_BINARY=$(find "$TEMP_DIR" -name "sing-box" -type f | head -n 1)
if [ -z "$SINGBOX_BINARY" ]; then
    echo "ERROR: sing-box binary not found in archive!"
    rm -rf "$TEMP_DIR"
    exit 1
fi

echo "Copying sing-box to $BIN_DIR/..."
cp "$SINGBOX_BINARY" "$BIN_DIR/sing-box"
chmod +x "$BIN_DIR/sing-box"

# Cleanup
echo "Cleaning up..."
rm -rf "$TEMP_DIR"

echo ""
echo "========================================"
echo "  sing-box downloaded successfully!"
echo "========================================"
echo "Location: $BIN_DIR/sing-box"
echo ""
echo "You can now build and run OneProxy:"
echo "  go build -o oneproxy ./cmd/oneproxy"
echo "  ./oneproxy"
echo ""
