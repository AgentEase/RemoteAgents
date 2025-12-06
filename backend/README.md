# Remote Agent Terminal - Backend

Go backend service for the Remote Agent Web Terminal platform.

## Requirements

- Go 1.21+
- GCC (for CGO/SQLite)
- Make (optional, for using Makefile)

## Quick Start

```bash
# Download dependencies
make deps

# Build for current platform
make build

# Run the server
make run
```

## Project Structure

```
backend/
├── cmd/
│   └── server/          # Application entry point
├── internal/
│   ├── auth/            # Authentication middleware
│   ├── buffer/          # Ring buffer implementation
│   ├── db/              # Database initialization
│   ├── driver/          # AgentDriver implementations
│   ├── logger/          # Asciinema logger
│   ├── model/           # Data models
│   ├── pty/             # PTY management
│   ├── repository/      # Data access layer
│   ├── session/         # Session management
│   └── ws/              # WebSocket handling
├── api/
│   └── handlers/        # HTTP API handlers
├── pkg/                 # Shared utilities
├── config/              # Configuration files
└── scripts/             # Build and utility scripts
```

## Building

### Current Platform
```bash
make build
# or
./scripts/build.sh current
```

### Cross-Platform
```bash
# All platforms
make build-all

# Specific platform
make build-linux
make build-windows
make build-darwin
```

## Testing

```bash
# Run all tests
make test

# Run tests with coverage
make test-coverage
```

## Configuration

Copy `config/config.example.yaml` to `config/config.yaml` and modify as needed.

## API Endpoints

- `GET /health` - Health check
- `POST /api/sessions` - Create session
- `GET /api/sessions` - List sessions
- `GET /api/sessions/:id` - Get session details
- `DELETE /api/sessions/:id` - Delete session
- `GET /api/sessions/:id/logs` - Download session logs
- `WS /api/sessions/:id/attach` - WebSocket terminal connection
