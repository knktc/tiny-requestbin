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
- `-tls-cert`: Path to the TLS certificate file (enables HTTPS when used with `-tls-key`)
- `-tls-key`: Path to the TLS private key file (enables HTTPS when used with `-tls-cert`)

Example with custom options:

```bash
tiny-requestbin -port 9000 -listen 0.0.0.0 -max 1000 -cli
```

Run with HTTPS:

```bash
tiny-requestbin -port 9443 -listen 127.0.0.1 \
  -tls-cert ./certs/localhost.pem \
  -tls-key ./certs/localhost-key.pem
```

Generate a local certificate with `mkcert`:

```bash
mkdir -p certs
mkcert -cert-file certs/localhost.pem -key-file certs/localhost-key.pem localhost 127.0.0.1 ::1
tiny-requestbin -port 9443 -listen 127.0.0.1 \
  -tls-cert ./certs/localhost.pem \
  -tls-key ./certs/localhost-key.pem
```

Optionally trust the local CA in your system keychain for browser-friendly testing:

```bash
mkcert -install
```

Or verify without changing the system trust store:

```bash
curl --cacert "$(mkcert -CAROOT)/rootCA.pem" https://127.0.0.1:9443/
```

## How It Works

1. Start the server using one of the methods above
2. Send HTTP or HTTPS requests to `http[s]://[listen-address]:[port]/any/path`
3. View captured requests in your browser at `http[s]://[listen-address]:[port]/`
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
