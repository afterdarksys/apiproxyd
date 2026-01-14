#!/usr/bin/env python3
"""
apiproxyd Installer
Automated installation script for apiproxyd on Linux, macOS, and Windows.
"""

import os
import sys
import subprocess
import platform
import shutil
from pathlib import Path

VERSION = "0.1.0"
COLORS = {
    'green': '\033[92m',
    'yellow': '\033[93m',
    'red': '\033[91m',
    'blue': '\033[94m',
    'end': '\033[0m',
}


def print_colored(text, color='green'):
    """Print colored text."""
    if platform.system() == 'Windows':
        print(text)
    else:
        print(f"{COLORS.get(color, '')}{text}{COLORS['end']}")


def print_header():
    """Print installation header."""
    print_colored("\n" + "=" * 60, 'blue')
    print_colored("  apiproxyd Installer v" + VERSION, 'blue')
    print_colored("  On-Premises API Caching Daemon", 'blue')
    print_colored("=" * 60 + "\n", 'blue')


def check_requirements():
    """Check system requirements."""
    print_colored("Checking system requirements...", 'blue')

    # Check OS
    os_type = platform.system()
    print(f"  OS: {os_type} {platform.release()}")

    # Check Go
    try:
        result = subprocess.run(['go', 'version'], capture_output=True, text=True)
        go_version = result.stdout.strip()
        print(f"  ✓ Go: {go_version}")
    except FileNotFoundError:
        print_colored("  ✗ Go not found!", 'red')
        print_colored("  Please install Go 1.21+ from https://go.dev/dl/", 'yellow')
        return False

    # Check Git
    try:
        subprocess.run(['git', '--version'], capture_output=True, text=True)
        print("  ✓ Git: Installed")
    except FileNotFoundError:
        print_colored("  ✗ Git not found!", 'red')
        print_colored("  Please install Git", 'yellow')
        return False

    return True


def build_binary():
    """Build the apiproxy binary."""
    print_colored("\nBuilding apiproxy binary...", 'blue')

    try:
        # Get build info
        try:
            commit = subprocess.run(
                ['git', 'rev-parse', '--short', 'HEAD'],
                capture_output=True, text=True
            ).stdout.strip()
        except:
            commit = 'dev'

        from datetime import datetime
        build_date = datetime.utcnow().strftime('%Y-%m-%dT%H:%M:%SZ')

        # Build command
        ldflags = f'-ldflags=-X main.version={VERSION} -X main.commit={commit} -X main.date={build_date}'

        result = subprocess.run(
            ['go', 'build', ldflags, '-o', 'apiproxy', 'main.go'],
            capture_output=True, text=True
        )

        if result.returncode != 0:
            print_colored(f"  ✗ Build failed: {result.stderr}", 'red')
            return False

        print_colored("  ✓ Binary built successfully", 'green')
        return True

    except Exception as e:
        print_colored(f"  ✗ Build error: {e}", 'red')
        return False


def install_binary():
    """Install the binary to system path."""
    print_colored("\nInstalling binary...", 'blue')

    # Determine installation path
    if platform.system() == 'Windows':
        install_dir = Path(os.environ.get('ProgramFiles', 'C:\\Program Files')) / 'apiproxy'
    else:
        # Try /usr/local/bin first, fall back to ~/.local/bin
        install_dir = Path('/usr/local/bin')
        if not os.access(install_dir, os.W_OK):
            install_dir = Path.home() / '.local' / 'bin'
            install_dir.mkdir(parents=True, exist_ok=True)

    binary_name = 'apiproxy.exe' if platform.system() == 'Windows' else 'apiproxy'
    source = Path('apiproxy')
    destination = install_dir / binary_name

    try:
        # Create directory if needed
        install_dir.mkdir(parents=True, exist_ok=True)

        # Copy binary
        shutil.copy2(source, destination)

        # Make executable on Unix
        if platform.system() != 'Windows':
            os.chmod(destination, 0o755)

        print_colored(f"  ✓ Installed to: {destination}", 'green')

        # Check if in PATH
        if platform.system() != 'Windows':
            path_dirs = os.environ.get('PATH', '').split(':')
            if str(install_dir) not in path_dirs:
                print_colored(f"\n  ⚠ Add to PATH: export PATH=\"{install_dir}:$PATH\"", 'yellow')

        return True

    except PermissionError:
        print_colored(f"  ✗ Permission denied. Try with sudo:", 'red')
        print_colored(f"    sudo python3 install.py", 'yellow')
        return False

    except Exception as e:
        print_colored(f"  ✗ Installation failed: {e}", 'red')
        return False


def create_config():
    """Create default configuration."""
    print_colored("\nCreating configuration...", 'blue')

    config_dir = Path.home() / '.apiproxy'
    config_file = config_dir / 'config.json'

    # Create directory
    config_dir.mkdir(parents=True, exist_ok=True)

    # Check if config already exists
    if config_file.exists():
        print_colored("  ⚠ Config already exists, skipping", 'yellow')
        return True

    # Copy example config if available
    example_config = Path('config.json.example')
    if example_config.exists():
        shutil.copy2(example_config, config_file)
        print_colored(f"  ✓ Config created: {config_file}", 'green')
        print_colored(f"  ⚠ Edit config and add your API key!", 'yellow')
        return True
    else:
        # Create minimal config
        import json
        config = {
            "server": {
                "host": "127.0.0.1",
                "port": 9002,
                "read_timeout": 15,
                "write_timeout": 15
            },
            "entry_point": "https://api.apiproxy.app",
            "api_key": "apx_live_YOUR_API_KEY_HERE",
            "cache": {
                "backend": "sqlite",
                "path": str(config_dir / "cache.db"),
                "ttl": 86400
            },
            "offline_endpoints": ["/health"],
            "whitelisted_endpoints": ["/v1/*"]
        }

        with open(config_file, 'w') as f:
            json.dump(config, f, indent=2)

        print_colored(f"  ✓ Config created: {config_file}", 'green')
        print_colored(f"  ⚠ Edit config and add your API key!", 'yellow')
        return True


def verify_installation():
    """Verify the installation."""
    print_colored("\nVerifying installation...", 'blue')

    try:
        result = subprocess.run(['apiproxy', '--version'], capture_output=True, text=True)
        if result.returncode == 0:
            print_colored(f"  ✓ {result.stdout.strip()}", 'green')
            return True
        else:
            print_colored("  ✗ apiproxy command not found in PATH", 'red')
            return False
    except FileNotFoundError:
        print_colored("  ✗ apiproxy command not found", 'red')
        print_colored("  Make sure the installation directory is in your PATH", 'yellow')
        return False


def print_next_steps():
    """Print next steps for user."""
    print_colored("\n" + "=" * 60, 'blue')
    print_colored("  Installation Complete!", 'green')
    print_colored("=" * 60, 'blue')

    print("\nNext steps:")
    print("  1. Edit config:    nano ~/.apiproxy/config.json")
    print("  2. Add API key:    (get from api.apiproxy.app)")
    print("  3. Login:          apiproxy login")
    print("  4. Start daemon:   apiproxy daemon start")
    print("  5. Test:           apiproxy test")

    print("\nUseful commands:")
    print("  apiproxy --help              Show help")
    print("  apiproxy config show         View configuration")
    print("  apiproxy daemon status       Check daemon status")
    print("  apiproxy api GET /v1/...     Make API request")

    print("\nDocumentation:")
    print("  README.md        - Getting started")
    print("  INSTALL.md       - Detailed installation guide")
    print("  DEPLOYMENT.md    - Production deployment")
    print("  ARCHITECTURE.md  - System architecture")

    print_colored("\nFor support: https://github.com/afterdarktech/apiproxyd/issues\n", 'blue')


def main():
    """Main installation function."""
    print_header()

    # Check if running from correct directory
    if not Path('main.go').exists():
        print_colored("Error: Please run this script from the apiproxyd directory", 'red')
        sys.exit(1)

    # Check requirements
    if not check_requirements():
        print_colored("\nInstallation aborted due to missing requirements.", 'red')
        sys.exit(1)

    # Build binary
    if not build_binary():
        print_colored("\nInstallation failed during build.", 'red')
        sys.exit(1)

    # Install binary
    if not install_binary():
        print_colored("\nInstallation failed during install.", 'red')
        sys.exit(1)

    # Create config
    create_config()

    # Verify
    verify_installation()

    # Print next steps
    print_next_steps()


if __name__ == '__main__':
    try:
        main()
    except KeyboardInterrupt:
        print_colored("\n\nInstallation cancelled by user.", 'yellow')
        sys.exit(1)
    except Exception as e:
        print_colored(f"\n\nUnexpected error: {e}", 'red')
        sys.exit(1)
