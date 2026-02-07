#!/bin/bash

# Remnawave Node Go - One-click Installation Script
# Usage: curl -fsSL https://raw.githubusercontent.com/Mikimiya/remnawave-node/main/install.sh | bash
# Or: wget -qO- https://raw.githubusercontent.com/Mikimiya/remnawave-node/main/install.sh | bash

# Check if running with bash
if [ -z "$BASH_VERSION" ]; then
    echo "Error: This script requires bash. Please run with: bash $0 $*"
    exit 1
fi

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
CYAN='\033[0;36m'
NC='\033[0m' # No Color

# Configuration
GITHUB_REPO="Mikimiya/remnawave-node"
BINARY_NAME="remnawave-node"
INSTALL_DIR="/usr/local/bin"
CONFIG_DIR="/etc/remnawave-node"
DATA_DIR="/var/lib/remnawave-node"
SERVICE_NAME="remnawave-node"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

# Print banner
print_banner() {
    echo -e "${CYAN}"
    echo "╔═══════════════════════════════════════════════════════════╗"
    echo "║                                                           ║"
    echo "║           Remnawave Node Go Installer                     ║"
    echo "║                                                           ║"
    echo "╚═══════════════════════════════════════════════════════════╝"
    echo -e "${NC}"
}

# Print colored messages
info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Check if running as root
check_root() {
    if [[ $EUID -ne 0 ]]; then
        error "This script must be run as root. Please use 'sudo' or run as root user."
    fi
}

# Detect OS and architecture
detect_platform() {
    OS=$(uname -s | tr '[:upper:]' '[:lower:]')
    ARCH=$(uname -m)

    case "$OS" in
        linux)
            OS="linux"
            ;;
        darwin)
            OS="darwin"
            ;;
        *)
            error "Unsupported operating system: $OS"
            ;;
    esac

    case "$ARCH" in
        x86_64|amd64)
            ARCH="amd64"
            ;;
        aarch64|arm64)
            ARCH="arm64"
            ;;
        armv7l|armv7)
            ARCH="armv7"
            ;;
        *)
            error "Unsupported architecture: $ARCH"
            ;;
    esac

    PLATFORM="${OS}_${ARCH}"
    info "Detected platform: ${PLATFORM}"
}

# Check required commands
check_dependencies() {
    local missing_deps=()

    for cmd in curl jq tar; do
        if ! command -v "$cmd" &> /dev/null; then
            missing_deps+=("$cmd")
        fi
    done

    if [[ ${#missing_deps[@]} -gt 0 ]]; then
        info "Installing missing dependencies: ${missing_deps[*]}"
        
        if command -v apt-get &> /dev/null; then
            apt-get update -qq
            apt-get install -y -qq "${missing_deps[@]}"
        elif command -v yum &> /dev/null; then
            yum install -y -q "${missing_deps[@]}"
        elif command -v dnf &> /dev/null; then
            dnf install -y -q "${missing_deps[@]}"
        elif command -v pacman &> /dev/null; then
            pacman -Sy --noconfirm "${missing_deps[@]}"
        elif command -v apk &> /dev/null; then
            apk add --no-cache "${missing_deps[@]}"
        else
            error "Could not install dependencies. Please install manually: ${missing_deps[*]}"
        fi
    fi
}

# Get latest release version from GitHub
get_latest_version() {
    info "Fetching latest release version..."
    
    LATEST_VERSION=$(curl -fsSL "https://api.github.com/repos/${GITHUB_REPO}/releases/latest" | jq -r '.tag_name')
    
    if [[ -z "$LATEST_VERSION" || "$LATEST_VERSION" == "null" ]]; then
        error "Failed to get latest version from GitHub"
    fi
    
    info "Latest version: ${LATEST_VERSION}"
}

# Download and install binary
download_and_install() {
    local version="${1:-$LATEST_VERSION}"
    local download_url="https://github.com/${GITHUB_REPO}/releases/download/${version}/${BINARY_NAME}_${version#v}_${PLATFORM}.tar.gz"
    local temp_dir=$(mktemp -d)
    local archive_file="${temp_dir}/${BINARY_NAME}.tar.gz"

    info "Downloading ${BINARY_NAME} ${version} for ${PLATFORM}..."
    info "URL: ${download_url}"

    if ! curl -fsSL -o "$archive_file" "$download_url"; then
        rm -rf "$temp_dir"
        error "Failed to download binary. Please check if the release exists for your platform."
    fi

    info "Extracting archive..."
    tar -xzf "$archive_file" -C "$temp_dir"

    # Find the binary (it might be in a subdirectory or have a suffix)
    local binary_path=$(find "$temp_dir" -name "${BINARY_NAME}*" -type f -not -name "*.tar.gz" -executable 2>/dev/null | head -1)
    
    if [[ -z "$binary_path" ]]; then
        # Try without executable flag (in case permissions aren't set)
        binary_path=$(find "$temp_dir" -name "${BINARY_NAME}*" -type f -not -name "*.tar.gz" 2>/dev/null | head -1)
    fi

    if [[ -z "$binary_path" ]]; then
        # List contents for debugging
        warning "Archive contents:"
        ls -la "$temp_dir"
        rm -rf "$temp_dir"
        error "Binary not found in archive"
    fi

    # Stop service if running
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        info "Stopping existing service..."
        systemctl stop "$SERVICE_NAME"
    fi

    # Install binary
    info "Installing binary to ${INSTALL_DIR}..."
    chmod +x "$binary_path"
    mv "$binary_path" "${INSTALL_DIR}/${BINARY_NAME}"

    # Cleanup
    rm -rf "$temp_dir"

    success "Binary installed successfully"
}

# Download geo files
download_geo_files() {
    # Get latest geoip release
    info "Fetching latest geoip version..."
    local geoip_version=$(curl -fsSL "https://api.github.com/repos/v2fly/geoip/releases/latest" | jq -r '.tag_name' 2>/dev/null)
    
    if [[ -n "$geoip_version" && "$geoip_version" != "null" ]]; then
        info "Downloading geoip.dat (${geoip_version})..."
        if curl -fsSL -o "${INSTALL_DIR}/geoip.dat" "https://github.com/v2fly/geoip/releases/download/${geoip_version}/geoip.dat"; then
            success "geoip.dat downloaded"
        else
            warning "Failed to download geoip.dat"
        fi
    else
        warning "Failed to get geoip version, using fallback..."
        if curl -fsSL -o "${INSTALL_DIR}/geoip.dat" "https://github.com/v2fly/geoip/releases/latest/download/geoip.dat"; then
            success "geoip.dat downloaded"
        else
            warning "Failed to download geoip.dat"
        fi
    fi

    # Get latest geosite release
    info "Fetching latest geosite version..."
    local geosite_version=$(curl -fsSL "https://api.github.com/repos/v2fly/domain-list-community/releases/latest" | jq -r '.tag_name' 2>/dev/null)
    
    if [[ -n "$geosite_version" && "$geosite_version" != "null" ]]; then
        info "Downloading geosite.dat (${geosite_version})..."
        if curl -fsSL -o "${INSTALL_DIR}/geosite.dat" "https://github.com/v2fly/domain-list-community/releases/download/${geosite_version}/dlc.dat"; then
            success "geosite.dat downloaded"
        else
            warning "Failed to download geosite.dat"
        fi
    else
        warning "Failed to get geosite version, using fallback..."
        if curl -fsSL -o "${INSTALL_DIR}/geosite.dat" "https://github.com/v2fly/domain-list-community/releases/latest/download/dlc.dat"; then
            success "geosite.dat downloaded"
        else
            warning "Failed to download geosite.dat"
        fi
    fi
}

# Create directories
create_directories() {
    info "Creating directories..."
    
    mkdir -p "$CONFIG_DIR"
    mkdir -p "$DATA_DIR"
    mkdir -p "$DATA_DIR/config"
    mkdir -p /var/log/remnawave-node
    
    chmod 755 "$CONFIG_DIR"
    chmod 755 "$DATA_DIR"
}

# Create systemd service
create_service() {
    info "Creating systemd service..."

    cat > "$SERVICE_FILE" << 'EOF'
[Unit]
Description=Remnawave Node Go
Documentation=https://github.com/Mikimiya/remnawave-node
After=network.target network-online.target
Wants=network-online.target

[Service]
Type=simple
User=root
Group=root
ExecStart=/usr/local/bin/remnawave-node
Restart=always
RestartSec=5
LimitNOFILE=65535
StandardOutput=journal
StandardError=journal
SyslogIdentifier=remnawave-node

# Environment file (optional)
EnvironmentFile=-/etc/remnawave-node/env

# Security hardening (optional, can be enabled if needed)
# NoNewPrivileges=true
# ProtectSystem=strict
# ProtectHome=true
# PrivateTmp=true
# ReadWritePaths=/var/lib/remnawave-node

[Install]
WantedBy=multi-user.target
EOF

    # Create default environment file if not exists
    if [[ ! -f "${CONFIG_DIR}/env" ]]; then
        cat > "${CONFIG_DIR}/env" << 'EOF'
# Remnawave Node Go Environment Configuration
# Uncomment and modify as needed

# Node payload (base64 encoded, from panel) - REQUIRED
# SECRET_KEY=

# API Port (default: 3000)
# NODE_PORT=3000

# Log level: debug, info, warn, error
# LOG_LEVEL=info

# Disable hash check for config comparison
# DISABLE_HASHED_SET_CHECK=false

# Port mapping for NAT machines (optional)
# Format: originalPort:mappedPort,originalPort:mappedPort
# Example: 443:10000,80:10001,8443:10002
# PORT_MAP=
EOF
    fi

    # Reload systemd
    systemctl daemon-reload
    
    success "Systemd service created"
}

# Create helper script for configuration
create_config_helper() {
    local helper_script="${INSTALL_DIR}/remnawave-node-config"
    
    cat > "$helper_script" << 'SCRIPT'
#!/bin/bash

CONFIG_DIR="/etc/remnawave-node"
ENV_FILE="${CONFIG_DIR}/env"

show_help() {
    echo "Remnawave Node Configuration Helper"
    echo ""
    echo "Usage: remnawave-node-config <command> [options]"
    echo ""
    echo "Commands:"
    echo "  set-secret-key <key>        Set the SECRET_KEY (base64 string from panel)"
    echo "  set-port-map <mapping>      Set port mapping for NAT machines"
    echo "                              Format: originalPort:mappedPort,..."
    echo "                              Example: 443:10000,80:10001,8443:10002"
    echo "  remove-port-map             Remove port mapping configuration"
    echo "  set-cert <cert> <key>       Set SSL certificate paths"
    echo "  show                        Show current configuration"
    echo "  edit                        Edit configuration file"
    echo "  status                      Show service status"
    echo "  logs                        Show service logs"
    echo "  restart                     Restart the service"
    echo ""
}

set_secret_key() {
    local secret_key="$1"
    if [[ -z "$secret_key" ]]; then
        echo "Error: SECRET_KEY is required"
        exit 1
    fi
    
    # Update or add SECRET_KEY in env file
    if grep -q "^SECRET_KEY=" "$ENV_FILE" 2>/dev/null; then
        sed -i "s|^SECRET_KEY=.*|SECRET_KEY=${secret_key}|" "$ENV_FILE"
    else
        echo "SECRET_KEY=${secret_key}" >> "$ENV_FILE"
    fi
    
    echo "SECRET_KEY updated. Restart service to apply: systemctl restart remnawave-node"
}

set_cert() {
    local cert="$1"
    local key="$2"
    
    if [[ -z "$cert" || -z "$key" ]]; then
        echo "Error: Both certificate and key paths are required"
        exit 1
    fi
    
    # Update cert paths
    if grep -q "^SSL_CERT_PATH=" "$ENV_FILE" 2>/dev/null; then
        sed -i "s|^SSL_CERT_PATH=.*|SSL_CERT_PATH=${cert}|" "$ENV_FILE"
    else
        echo "SSL_CERT_PATH=${cert}" >> "$ENV_FILE"
    fi
    
    if grep -q "^SSL_KEY_PATH=" "$ENV_FILE" 2>/dev/null; then
        sed -i "s|^SSL_KEY_PATH=.*|SSL_KEY_PATH=${key}|" "$ENV_FILE"
    else
        echo "SSL_KEY_PATH=${key}" >> "$ENV_FILE"
    fi
    
    echo "Certificate paths updated. Restart service to apply: systemctl restart remnawave-node"
}

set_port_map() {
    local mapping="$1"
    if [[ -z "$mapping" ]]; then
        echo "Error: Port mapping is required"
        echo "Format: originalPort:mappedPort,originalPort:mappedPort"
        echo "Example: 443:10000,80:10001,8443:10002"
        exit 1
    fi
    
    # Validate format
    IFS=',' read -ra pairs <<< "$mapping"
    for pair in "${pairs[@]}"; do
        pair=$(echo "$pair" | xargs) # trim whitespace
        if ! [[ "$pair" =~ ^[0-9]+:[0-9]+$ ]]; then
            echo "Error: Invalid mapping format: '$pair'"
            echo "Expected format: originalPort:mappedPort"
            exit 1
        fi
    done
    
    # Update or add PORT_MAP in env file
    if grep -q "^PORT_MAP=" "$ENV_FILE" 2>/dev/null; then
        sed -i "s|^PORT_MAP=.*|PORT_MAP=${mapping}|" "$ENV_FILE"
    else
        echo "PORT_MAP=${mapping}" >> "$ENV_FILE"
    fi
    
    echo "Port mapping updated: ${mapping}"
    echo "Restart service to apply: systemctl restart remnawave-node"
}

remove_port_map() {
    if grep -q "^PORT_MAP=" "$ENV_FILE" 2>/dev/null; then
        sed -i '/^PORT_MAP=/d' "$ENV_FILE"
        echo "Port mapping removed. Restart service to apply: systemctl restart remnawave-node"
    else
        echo "No port mapping configured."
    fi
}

show_config() {
    echo "Current configuration (${ENV_FILE}):"
    echo "----------------------------------------"
    if [[ -f "$ENV_FILE" ]]; then
        grep -v "^#" "$ENV_FILE" | grep -v "^$"
    else
        echo "(No configuration file found)"
    fi
}

case "$1" in
    set-secret-key)
        set_secret_key "$2"
        ;;
    set-port-map)
        set_port_map "$2"
        ;;
    remove-port-map)
        remove_port_map
        ;;
    set-cert)
        set_cert "$2" "$3"
        ;;
    show)
        show_config
        ;;
    edit)
        ${EDITOR:-nano} "$ENV_FILE"
        ;;
    status)
        systemctl status remnawave-node
        ;;
    logs)
        journalctl -u remnawave-node -f
        ;;
    restart)
        systemctl restart remnawave-node
        ;;
    *)
        show_help
        ;;
esac
SCRIPT

    chmod +x "$helper_script"
    success "Configuration helper created: ${helper_script}"
}

# Print post-installation instructions
print_instructions() {
    echo ""
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}    Installation Complete!${NC}"
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "${CYAN}Next steps:${NC}"
    echo ""
    echo "1. Configure your node (get SECRET_KEY from Remnawave panel):"
    echo -e "   ${YELLOW}remnawave-node-config set-secret-key <your-secret-key>${NC}"
    echo ""
    echo "2. Start the service:"
    echo -e "   ${YELLOW}systemctl start remnawave-node${NC}"
    echo ""
    echo "3. Enable auto-start on boot:"
    echo -e "   ${YELLOW}systemctl enable remnawave-node${NC}"
    echo ""
    echo -e "${CYAN}Useful commands:${NC}"
    echo "  - Check status:    systemctl status remnawave-node"
    echo "  - View logs:       journalctl -u remnawave-node -f"
    echo "  - Restart:         systemctl restart remnawave-node"
    echo "  - Config helper:   remnawave-node-config --help"
    echo ""
    echo -e "${CYAN}Installed components:${NC}"
    echo "  - Remnawave Node:  ${INSTALL_DIR}/${BINARY_NAME} (with embedded Xray-core)"
    echo ""
    echo -e "${CYAN}Files:${NC}"
    echo "  - Config:          ${CONFIG_DIR}/env"
    echo "  - Data:            ${DATA_DIR}"
    echo "  - Service:         ${SERVICE_FILE}"
    echo ""
}

# Uninstall function
uninstall() {
    warning "Uninstalling Remnawave Node Go..."
    
    # Stop and disable service
    if systemctl is-active --quiet "$SERVICE_NAME" 2>/dev/null; then
        systemctl stop "$SERVICE_NAME"
    fi
    if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
        systemctl disable "$SERVICE_NAME"
    fi
    
    # Remove files
    rm -f "${INSTALL_DIR}/${BINARY_NAME}"
    rm -f "${INSTALL_DIR}/remnawave-node-config"
    rm -f "$SERVICE_FILE"
    
    # Reload systemd
    systemctl daemon-reload
    
    success "Uninstallation complete"
    info "Configuration directory ${CONFIG_DIR} was preserved"
    info "To remove all data: rm -rf ${CONFIG_DIR} ${DATA_DIR} /var/log/remnawave-node"
}

# Update function
update() {
    info "Updating Remnawave Node Go..."
    
    get_latest_version
    
    # Check current version
    if [[ -x "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        CURRENT_VERSION=$("${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+' | head -1 || echo "unknown")
        info "Current version: ${CURRENT_VERSION}"
    fi
    
    download_and_install
    download_geo_files
    
    # Restart service if it was running
    if systemctl is-enabled --quiet "$SERVICE_NAME" 2>/dev/null; then
        info "Restarting service..."
        systemctl restart "$SERVICE_NAME"
    fi
    
    success "Update complete!"
}

# Update geo files only
update_geo() {
    info "Updating GeoIP and GeoSite data files..."
    
    # Get latest geoip release
    info "Fetching latest geoip version..."
    GEOIP_LATEST=$(curl -fsSL "https://api.github.com/repos/v2fly/geoip/releases/latest" | jq -r '.tag_name' 2>/dev/null)
    if [[ -n "$GEOIP_LATEST" && "$GEOIP_LATEST" != "null" ]]; then
        info "Latest geoip version: ${GEOIP_LATEST}"
        info "Downloading geoip.dat..."
        if curl -fsSL -o "${INSTALL_DIR}/geoip.dat" "https://github.com/v2fly/geoip/releases/download/${GEOIP_LATEST}/geoip.dat"; then
            success "geoip.dat updated"
        else
            warning "Failed to download geoip.dat"
        fi
    else
        warning "Failed to get latest geoip version"
    fi

    # Get latest geosite release
    info "Fetching latest geosite version..."
    GEOSITE_LATEST=$(curl -fsSL "https://api.github.com/repos/v2fly/domain-list-community/releases/latest" | jq -r '.tag_name' 2>/dev/null)
    if [[ -n "$GEOSITE_LATEST" && "$GEOSITE_LATEST" != "null" ]]; then
        info "Latest geosite version: ${GEOSITE_LATEST}"
        info "Downloading geosite.dat..."
        if curl -fsSL -o "${INSTALL_DIR}/geosite.dat" "https://github.com/v2fly/domain-list-community/releases/download/${GEOSITE_LATEST}/dlc.dat"; then
            success "geosite.dat updated"
        else
            warning "Failed to download geosite.dat"
        fi
    else
        warning "Failed to get latest geosite version"
    fi

    success "Geo files update complete!"
}

# Show Xray core version info
show_xray_version() {
    info "Checking Xray core version..."
    
    if [[ -x "${INSTALL_DIR}/${BINARY_NAME}" ]]; then
        echo ""
        echo -e "${CYAN}Current Remnawave Node Info:${NC}"
        "${INSTALL_DIR}/${BINARY_NAME}" --version 2>/dev/null || echo "Unable to get version"
        echo ""
    else
        warning "Remnawave Node is not installed"
    fi
    
    # Check latest Xray-core version on GitHub
    info "Fetching latest Xray-core version from GitHub..."
    XRAY_LATEST=$(curl -fsSL "https://api.github.com/repos/XTLS/Xray-core/releases/latest" | jq -r '.tag_name' 2>/dev/null)
    if [[ -n "$XRAY_LATEST" && "$XRAY_LATEST" != "null" ]]; then
        echo -e "${CYAN}Latest Xray-core release: ${GREEN}${XRAY_LATEST}${NC}"
    else
        warning "Failed to get latest Xray-core version"
    fi
    
    echo ""
    echo -e "${YELLOW}Note: Xray-core is embedded in Remnawave Node.${NC}"
    echo -e "${YELLOW}To update Xray-core, you need to update the Remnawave Node binary.${NC}"
    echo ""
    echo "Run: $0 update"
}

# Prompt for SECRET_KEY and configure
prompt_secret_key() {
    echo ""
    echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}    SECRET_KEY Configuration${NC}"
    echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "${YELLOW}Please enter your SECRET_KEY (base64 string from Remnawave panel).${NC}"
    echo -e "${YELLOW}Press Enter to skip if you want to configure later.${NC}"
    echo ""
    read -r -p "SECRET_KEY: " USER_SECRET_KEY

    if [[ -n "$USER_SECRET_KEY" ]]; then
        remnawave-node-config set-secret-key "$USER_SECRET_KEY"
        success "SECRET_KEY configured successfully"
    else
        warning "SECRET_KEY skipped. You can set it later with: remnawave-node-config set-secret-key <your-secret-key>"
    fi
}

# Prompt for port mapping (for NAT machines)
prompt_port_map() {
    echo ""
    echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
    echo -e "${CYAN}    Port Mapping Configuration (NAT Machine)${NC}"
    echo -e "${CYAN}════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "${YELLOW}Are you running on a NAT machine with limited ports?${NC}"
    echo -e "${YELLOW}If yes, you can configure port mapping to remap Xray inbound ports.${NC}"
    echo ""
    read -r -p "Configure port mapping? (y/N): " CONFIGURE_PORT_MAP

    if [[ "$CONFIGURE_PORT_MAP" =~ ^[Yy]$ ]]; then
        echo ""
        echo -e "${CYAN}Enter port mappings in the format: originalPort:mappedPort${NC}"
        echo -e "${CYAN}Example: 443:10000,80:10001,8443:10002${NC}"
        echo ""
        echo -e "${YELLOW}This means:${NC}"
        echo -e "  Panel port 443  -> NAT port 10000"
        echo -e "  Panel port 80   -> NAT port 10001"
        echo -e "  Panel port 8443 -> NAT port 10002"
        echo ""
        read -r -p "Port mapping: " USER_PORT_MAP

        if [[ -n "$USER_PORT_MAP" ]]; then
            remnawave-node-config set-port-map "$USER_PORT_MAP"
            success "Port mapping configured successfully"
        else
            warning "Port mapping skipped."
        fi
    else
        info "Port mapping skipped (not a NAT machine or not needed)."
    fi
}

# Main installation function
install() {
    print_banner
    check_root
    detect_platform
    check_dependencies
    get_latest_version
    download_and_install
    download_geo_files
    create_directories
    create_service
    create_config_helper
    print_instructions

    # 1. Enable service to start on boot
    info "Enabling service to start on boot..."
    systemctl enable "$SERVICE_NAME"
    success "Service enabled for auto-start"

    # 2. Prompt for SECRET_KEY input
    prompt_secret_key

    # 3. Prompt for port mapping (NAT machines)
    prompt_port_map

    # 4. Start the service after configuration is done
    info "Starting remnawave-node service..."
    systemctl start "$SERVICE_NAME"
    success "Service started"

    echo ""
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo -e "${GREEN}    All done! Remnawave Node is running.${NC}"
    echo -e "${GREEN}════════════════════════════════════════════════════════════${NC}"
    echo ""
    echo -e "  Check status:  ${YELLOW}systemctl status remnawave-node${NC}"
    echo -e "  View logs:     ${YELLOW}journalctl -u remnawave-node -f${NC}"
    echo ""
}

# Parse command line arguments
case "${1:-install}" in
    install)
        install
        ;;
    update|upgrade)
        check_root
        detect_platform
        check_dependencies
        update
        ;;
    update-geo|geo)
        check_root
        check_dependencies
        update_geo
        ;;
    xray-version|xray|version)
        check_dependencies
        show_xray_version
        ;;
    uninstall|remove)
        check_root
        uninstall
        ;;
    --help|-h)
        echo "Remnawave Node Go Installer"
        echo ""
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  install       Install Remnawave Node Go (default)"
        echo "  update        Update to latest version (includes Xray-core)"
        echo "  update-geo    Update GeoIP and GeoSite data files only"
        echo "  xray-version  Show current and latest Xray-core version info"
        echo "  uninstall     Remove Remnawave Node Go"
        echo "  --help        Show this help message"
        echo ""
        echo "One-line install:"
        echo "  curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.sh | bash"
        echo ""
        echo "Update commands:"
        echo "  curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.sh | bash -s update"
        echo "  curl -fsSL https://raw.githubusercontent.com/${GITHUB_REPO}/main/install.sh | bash -s update-geo"
        ;;
    *)
        error "Unknown command: $1. Use --help for usage."
        ;;
esac
