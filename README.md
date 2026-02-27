# Tiny RequestBin

*[English](README.md) | [简体中文](README_ZH_CN.md)*

A lightweight HTTP request inspector and debugging tool written in Go. Tiny RequestBin captures and displays HTTP requests for inspection and debugging purposes.

This project was initially created with Gemini and later modified with GitHub Copilot.

[![Go Version](https://img.shields.io/github/go-mod/go-version/knktc/tiny-requestbin)](https://golang.org/)
[![License](https://img.shields.io/github/license/knktc/tiny-requestbin)](LICENSE)

## Features

- 🚀 **Lightweight & Fast**: Simple implementation with minimal dependencies
- 🔍 **Request Inspection**: View detailed information about incoming HTTP requests
- 🌐 **Web UI**: Clean interface to browse captured requests
- 📋 **CLI Mode**: Option to print requests directly to the terminal
- 🔒 **Local Only**: All data stays on your machine, no external services needed

## Installation

### Option 1: Using Docker (Recommended)

```bash
# Pull and run the latest image
docker run -p 8282:8282 knktc/tiny-requestbin

# Or run with custom options
docker run -p 8282:8282 knktc/tiny-requestbin -port 8282 -listen 0.0.0.0 -max 1000

# Using docker-compose
docker-compose up -d
```

### Option 2: Using Helm (Kubernetes)

```bash
# Install with default settings
helm install my-requestbin helm/tiny-requestbin/

# Install with custom options
helm install my-requestbin helm/tiny-requestbin/ \
  --set config.max=1000 \
  --set service.type=NodePort

# Enable HTTPRoute (Gateway API)
helm install my-requestbin helm/tiny-requestbin/ \
  --set httpRoute.enabled=true \
  --set 'httpRoute.parentRefs[0].name=my-gateway' \
  --set 'httpRoute.hostnames[0]=requestbin.example.com'
```

By default the chart deploys with image `knktc/tiny-requestbin:latest`, exposes port `8282` via a `ClusterIP` Service, and sets resource limits to `128Mi` memory / `500m` CPU.

### Option 3: Install using `go install`

```bash
go install github.com/knktc/tiny-requestbin@latest
```

### Option 4: Build from source

```bash
git clone https://github.com/knktc/tiny-requestbin.git
cd tiny-requestbin
go build
```

## Usage

Start the server with default options:

```bash
tiny-requestbin
```

### Command-line Options

- `-port`: Port for the server to listen on (default: 8282)
- `-listen`: Address to listen on (default: 127.0.0.1)
- `-max`: Maximum number of requests to store (default: 100)
- `-cli`: Enable CLI mode to print requests to terminal (default: false)

Example with custom options:

```bash
tiny-requestbin -port 9000 -listen 0.0.0.0 -max 1000 -cli
```

## How It Works

1. Start the server using one of the methods above
2. Send HTTP requests to `http://[listen-address]:[port]/any/path`
3. View captured requests in your browser at `http://[listen-address]:[port]/`
4. If CLI mode is enabled, requests will also be printed to the terminal

## Docker Multi-Architecture Support

The Docker image supports multiple architectures:
- `linux/amd64` (Intel/AMD 64-bit)
- `linux/arm64` (ARM 64-bit, Apple Silicon, etc.)

Multi-architecture images are automatically built and published via GitHub Actions when tags are pushed.

### Local Development

```bash
# Build and test locally (single architecture)
make docker-test

# Build for current platform only
make docker-build
```

## Screenshots

### CLI Mode

When running with the `-cli` flag, HTTP requests are beautifully formatted and displayed directly in the terminal:

![CLI Mode Screenshot](screenshots/run_in_cmd.png)

### Web Interface

The web interface provides a clean and intuitive way to browse and inspect captured requests:

![Web Interface Screenshot](screenshots/webpage.png)

## Contributing

Contributions are welcome! Feel free to submit a Pull Request.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.
