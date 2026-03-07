#!/bin/bash
set -e

# Proxera Agent Installation Script
# Usage: curl -sSL https://your-api-host:8080/install.sh | sudo bash -s -- <agent_id> <panel_url>
# The API key is read from PROXERA_API_KEY env var or prompted interactively.
# This avoids exposing the key in shell history.

AGENT_ID="$1"
PANEL_URL="${2:-https://your-api-host:8080}"

# Read API key from env var (preferred) or fall back to positional arg for backward compat
if [ -n "$PROXERA_API_KEY" ]; then
    API_KEY="$PROXERA_API_KEY"
elif [ -n "$2" ] && echo "$2" | grep -qv '^https\?://'; then
    # Second arg is not a URL — treat as API key (legacy mode)
    API_KEY="$2"
    PANEL_URL="${3:-https://your-api-host:8080}"
fi

# Cleanup temp files on exit
cleanup() {
    rm -f "/tmp/proxera-$$"
}
trap cleanup EXIT

# Validate PANEL_URL format (must start with http:// or https://)
if ! echo "$PANEL_URL" | grep -qE '^https?://[a-zA-Z0-9._-]+'; then
    echo "Error: Invalid panel URL format: $PANEL_URL"
    exit 1
fi

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${GREEN}╔═══════════════════════════════════════╗${NC}"
echo -e "${GREEN}║     Proxera Agent Installation        ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════╝${NC}"
echo ""

# Check if running as root
if [ "$EUID" -ne 0 ]; then
    echo -e "${RED}Error: This script must be run as root${NC}"
    echo "Please run: sudo bash install.sh"
    exit 1
fi

# Validate arguments
if [ -z "$AGENT_ID" ]; then
    echo -e "${RED}Error: Missing agent ID${NC}"
    echo "Usage: PROXERA_API_KEY=<key> curl -sSL https://your-api-host:8080/install.sh | sudo bash -s -- <agent_id> [panel_url]"
    exit 1
fi

if [ -z "$API_KEY" ]; then
    echo -e "${RED}Error: Missing API key${NC}"
    echo "Set PROXERA_API_KEY environment variable before running:"
    echo "  export PROXERA_API_KEY=<your_api_key>"
    echo "  curl -sSL https://your-api-host:8080/install.sh | sudo bash -s -- <agent_id> [panel_url]"
    exit 1
fi

# Detect OS and architecture
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m)

case "$ARCH" in
    x86_64)
        ARCH="amd64"
        ;;
    aarch64|arm64)
        ARCH="arm64"
        ;;
    *)
        echo -e "${RED}Error: Unsupported architecture: $ARCH${NC}"
        exit 1
        ;;
esac

# Determine binary name
BINARY_NAME="proxera-${OS}-${ARCH}"
if [ "$OS" = "windows" ]; then
    BINARY_NAME="${BINARY_NAME}.exe"
fi

echo -e "${YELLOW}→${NC} Detected OS: $OS"
echo -e "${YELLOW}→${NC} Detected Architecture: $ARCH"
echo ""

# Download agent
echo -e "${YELLOW}→${NC} Downloading Proxera agent..."
DOWNLOAD_URL="${PANEL_URL}/api/agent/download/${BINARY_NAME}"
TMP_FILE="/tmp/proxera-$$"

if command -v curl &> /dev/null; then
    curl -fsSL --max-time 60 "$DOWNLOAD_URL" -o "$TMP_FILE"
elif command -v wget &> /dev/null; then
    wget -q --timeout=60 "$DOWNLOAD_URL" -O "$TMP_FILE"
else
    echo -e "${RED}Error: Neither curl nor wget found. Please install one of them.${NC}"
    exit 1
fi

if [ ! -f "$TMP_FILE" ]; then
    echo -e "${RED}Error: Failed to download agent binary${NC}"
    exit 1
fi

echo -e "${GREEN}✓${NC} Agent downloaded"

# Make executable and move to system path
echo -e "${YELLOW}→${NC} Installing agent..."
chmod +x "$TMP_FILE"
mv "$TMP_FILE" /usr/local/bin/proxera

if [ ! -f /usr/local/bin/proxera ]; then
    echo -e "${RED}Error: Failed to install agent${NC}"
    exit 1
fi

echo -e "${GREEN}✓${NC} Agent installed to /usr/local/bin/proxera"

# Run setup (install and configure nginx)
echo -e "${YELLOW}→${NC} Setting up nginx..."
proxera -setup

if [ $? -ne 0 ]; then
    echo -e "${YELLOW}Warning: Nginx setup encountered issues. Continuing...${NC}"
fi

# Create config directory
CONFIG_DIR="/etc/proxera"
mkdir -p "$CONFIG_DIR"

# Initialize config
echo -e "${YELLOW}→${NC} Creating configuration..."
cd "$CONFIG_DIR"
proxera -init

# Update config with credentials
CONFIG_FILE="$CONFIG_DIR/proxera.yaml"
if [ -f "$CONFIG_FILE" ]; then
    # Add agent credentials to config
    cat >> "$CONFIG_FILE" << EOF

# Agent credentials (added by installer)
agent_id: $AGENT_ID
api_key: $API_KEY
panel_url: $PANEL_URL
EOF
    echo -e "${GREEN}✓${NC} Configuration created at $CONFIG_FILE"
else
    echo -e "${RED}Error: Failed to create configuration file${NC}"
    exit 1
fi

# Create systemd service
echo -e "${YELLOW}→${NC} Creating systemd service..."
cat > /etc/systemd/system/proxera.service << EOF
[Unit]
Description=Proxera Agent
After=network.target

[Service]
Type=simple
User=root
WorkingDirectory=$CONFIG_DIR
ExecStart=/usr/local/bin/proxera -connect -config $CONFIG_FILE
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

# Enable and start service
systemctl daemon-reload
systemctl enable proxera
systemctl start proxera

if systemctl is-active --quiet proxera; then
    echo -e "${GREEN}✓${NC} Proxera agent service started"
else
    echo -e "${YELLOW}Warning: Service may not be running. Check with: systemctl status proxera${NC}"
fi

# Show status
echo ""
echo -e "${GREEN}╔═══════════════════════════════════════╗${NC}"
echo -e "${GREEN}║     Installation Complete! 🎉         ║${NC}"
echo -e "${GREEN}╚═══════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}✓${NC} Agent installed and connected to panel"
echo -e "${GREEN}✓${NC} Service running in background"
echo ""
echo "Next steps:"
echo "  1. View agent status: sudo systemctl status proxera"
echo "  2. View agent logs:   sudo journalctl -u proxera -f"
echo "  3. Edit config:       sudo nano $CONFIG_FILE"
echo "  4. Restart agent:     sudo systemctl restart proxera"
echo ""
echo "The agent is now connected to your Proxera panel!"
