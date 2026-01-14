# Installation Guide

This guide covers different installation methods for `apiproxyd`.

## Table of Contents

- [Requirements](#requirements)
- [Quick Install](#quick-install)
- [Manual Installation](#manual-installation)
- [Platform-Specific Instructions](#platform-specific-instructions)
- [Docker Installation](#docker-installation)
- [Verification](#verification)
- [Next Steps](#next-steps)

## Requirements

### Minimum Requirements
- **Go**: 1.21 or higher (for building from source)
- **OS**: Linux, macOS, or Windows
- **Memory**: 50MB RAM minimum
- **Disk**: 100MB free space
- **Network**: HTTPS access to api.apiproxy.app

### Optional Requirements
- **SQLite**: Built-in (no installation needed)
- **PostgreSQL**: 12+ (only if using PostgreSQL cache backend)
- **Docker**: 20.10+ (for containerized deployment)

## Quick Install

### Using Python Installer (Recommended)

```bash
# Download and run installer
curl -sSL https://raw.githubusercontent.com/afterdarktech/apiproxyd/main/install.py | python3 -

# Or clone and run
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd
python3 install.py
```

The installer will:
1. Check system requirements
2. Build the binary
3. Install to `/usr/local/bin` (or user-specified location)
4. Create default configuration
5. Verify installation

### Using Go Install

```bash
go install github.com/afterdarktech/apiproxyd@latest
```

This installs the binary to `$GOPATH/bin/apiproxy`.

### Using Makefile

```bash
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd
make install
```

This builds and installs to `/usr/local/bin/apiproxy`.

## Manual Installation

### Step 1: Clone Repository

```bash
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd
```

### Step 2: Install Dependencies

```bash
go mod download
go mod tidy
```

### Step 3: Build Binary

```bash
# Standard build
go build -o apiproxy main.go

# Optimized build with version info
make build
```

### Step 4: Install Binary

```bash
# System-wide installation (requires sudo)
sudo mv apiproxy /usr/local/bin/

# Or user installation
mkdir -p ~/.local/bin
mv apiproxy ~/.local/bin/
export PATH="$HOME/.local/bin:$PATH"
```

### Step 5: Create Configuration

```bash
# Create config directory
mkdir -p ~/.apiproxy

# Copy example config
cp config.json.example ~/.apiproxy/config.json

# Or create new config
apiproxy config init
```

### Step 6: Edit Configuration

```bash
# Edit with your API key
nano ~/.apiproxy/config.json

# Or in current directory
nano config.json
```

Update the `api_key` field:
```json
{
  "api_key": "apx_live_your_api_key_here",
  ...
}
```

## Platform-Specific Instructions

### macOS

#### Using Homebrew (Coming Soon)
```bash
brew install afterdarktech/tap/apiproxyd
```

#### Manual Installation
```bash
# Install Go (if not installed)
brew install go

# Clone and build
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd
make build

# Install
sudo make install
```

#### macOS Service (launchd)

Create `~/Library/LaunchAgents/com.afterdarktech.apiproxyd.plist`:

```xml
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
    <key>Label</key>
    <string>com.afterdarktech.apiproxyd</string>
    <key>ProgramArguments</key>
    <array>
        <string>/usr/local/bin/apiproxy</string>
        <string>daemon</string>
        <string>start</string>
    </array>
    <key>RunAtLoad</key>
    <true/>
    <key>KeepAlive</key>
    <true/>
    <key>StandardOutPath</key>
    <string>/tmp/apiproxyd.log</string>
    <key>StandardErrorPath</key>
    <string>/tmp/apiproxyd.error.log</string>
</dict>
</plist>
```

Enable service:
```bash
launchctl load ~/Library/LaunchAgents/com.afterdarktech.apiproxyd.plist
launchctl start com.afterdarktech.apiproxyd
```

### Linux

#### Ubuntu/Debian
```bash
# Install Go
sudo apt update
sudo apt install golang-go git

# Clone and build
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd
make build
sudo make install
```

#### RHEL/CentOS/Fedora
```bash
# Install Go
sudo dnf install golang git

# Clone and build
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd
make build
sudo make install
```

#### systemd Service

Create `/etc/systemd/system/apiproxyd.service`:

```ini
[Unit]
Description=API Proxy Cache Daemon
After=network.target

[Service]
Type=simple
User=apiproxy
Group=apiproxy
WorkingDirectory=/etc/apiproxy
ExecStart=/usr/local/bin/apiproxy daemon start
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

Setup:
```bash
# Create user
sudo useradd -r -s /bin/false apiproxy

# Create directories
sudo mkdir -p /etc/apiproxy /var/lib/apiproxy
sudo chown apiproxy:apiproxy /var/lib/apiproxy

# Copy config
sudo cp config.json /etc/apiproxy/
sudo chown apiproxy:apiproxy /etc/apiproxy/config.json

# Enable service
sudo systemctl daemon-reload
sudo systemctl enable apiproxyd
sudo systemctl start apiproxyd
sudo systemctl status apiproxyd
```

### Windows

#### Manual Installation
1. Download Go from https://go.dev/dl/
2. Install Go
3. Open PowerShell:

```powershell
# Clone repository
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd

# Build
go build -o apiproxy.exe main.go

# Move to Program Files
Move-Item apiproxy.exe "C:\Program Files\apiproxy\"

# Add to PATH
$env:Path += ";C:\Program Files\apiproxy"
```

#### Windows Service (NSSM)
1. Download NSSM: https://nssm.cc/download
2. Install service:

```cmd
nssm install apiproxyd "C:\Program Files\apiproxy\apiproxy.exe" daemon start
nssm set apiproxyd AppDirectory "C:\Program Files\apiproxy"
nssm start apiproxyd
```

## Docker Installation

### Using Docker

```bash
# Build image
git clone https://github.com/afterdarktech/apiproxyd.git
cd apiproxyd
docker build -t apiproxyd:latest .

# Run container
docker run -d \
  --name apiproxyd \
  -p 9002:9002 \
  -v $(pwd)/config.json:/app/config.json:ro \
  -v apiproxy-cache:/var/lib/apiproxy \
  apiproxyd:latest
```

### Using Docker Compose

Create `docker-compose.yml`:

```yaml
version: '3.8'

services:
  apiproxyd:
    build: .
    ports:
      - "9002:9002"
    volumes:
      - ./config.json:/app/config.json:ro
      - cache-data:/var/lib/apiproxy
    environment:
      - APIPROXY_API_KEY=${API_KEY}
    restart: unless-stopped

volumes:
  cache-data:
```

Run:
```bash
docker-compose up -d
```

### Using Pre-built Image (Coming Soon)

```bash
docker pull afterdarktech/apiproxyd:latest
docker run -d -p 9002:9002 afterdarktech/apiproxyd:latest
```

## Verification

### Test Installation

```bash
# Check version
apiproxy --version

# Run diagnostics
apiproxy test

# Check help
apiproxy --help
```

Expected output:
```
apiproxyd 0.1.0 (commit: abc123, built: 2026-01-13T12:00:00Z)
```

### Test Authentication

```bash
# Login with API key
apiproxy login --api-key apx_live_xxxxx

# Verify authentication
apiproxy config show
```

### Test Daemon

```bash
# Start daemon
apiproxy daemon start

# Check status
apiproxy daemon status

# Test health endpoint
curl http://localhost:9002/health
```

### Test API Request

```bash
# Make test request
apiproxy api GET /v1/darkapi/ip/8.8.8.8

# Via HTTP proxy
curl http://localhost:9002/api/v1/darkapi/ip/8.8.8.8 \
  -H "X-API-Key: apx_live_xxxxx"
```

## Troubleshooting Installation

### Go Build Fails

```bash
# Ensure Go version is correct
go version  # Should be 1.21+

# Clean build cache
go clean -cache
go clean -modcache

# Rebuild dependencies
go mod download
go build -o apiproxy main.go
```

### Permission Denied

```bash
# On Linux/macOS, use sudo for system install
sudo make install

# Or install to user directory
mkdir -p ~/.local/bin
cp apiproxy ~/.local/bin/
```

### Binary Not Found

```bash
# Check PATH
echo $PATH

# Add to PATH (bash)
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.bashrc
source ~/.bashrc

# Add to PATH (zsh)
echo 'export PATH="$HOME/.local/bin:$PATH"' >> ~/.zshrc
source ~/.zshrc
```

### Configuration Errors

```bash
# Validate config
apiproxy test

# Check config file exists
ls -la ~/.apiproxy/config.json

# Use example config
cp config.json.example config.json
```

## Next Steps

After installation:

1. **Configure**: Edit `config.json` with your settings
2. **Authenticate**: Run `apiproxy login`
3. **Start Daemon**: Run `apiproxy daemon start`
4. **Test**: Make API requests
5. **Deploy**: Set up systemd/launchd service for production

See [DEPLOYMENT.md](DEPLOYMENT.md) for production deployment guide.

## Uninstallation

### Remove Binary

```bash
# System installation
sudo rm /usr/local/bin/apiproxy

# User installation
rm ~/.local/bin/apiproxy
```

### Remove Configuration and Cache

```bash
# Remove all data
rm -rf ~/.apiproxy/

# Or keep config, remove cache only
rm ~/.apiproxy/cache.db
```

### Remove Service

```bash
# systemd (Linux)
sudo systemctl stop apiproxyd
sudo systemctl disable apiproxyd
sudo rm /etc/systemd/system/apiproxyd.service
sudo systemctl daemon-reload

# launchd (macOS)
launchctl unload ~/Library/LaunchAgents/com.afterdarktech.apiproxyd.plist
rm ~/Library/LaunchAgents/com.afterdarktech.apiproxyd.plist
```

## Getting Help

- **Documentation**: [README.md](README.md), [ARCHITECTURE.md](ARCHITECTURE.md)
- **Issues**: https://github.com/afterdarktech/apiproxyd/issues
- **Support**: support@apiproxy.app
