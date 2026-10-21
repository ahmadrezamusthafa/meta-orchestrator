.PHONY: all dev run-daemon run-web build build-daemon build-web test clean clean-all help setup install-deps install-go install-web check-prereqs

# Default target
all: build

# Display help commands
help:
	@echo "AI-Driven SDLC Meta-Orchestrator Control Targets:"
	@echo "  make setup        - Install required system tools (Node.js, Git, Claude Code) and dependencies"
	@echo "  make install-deps - Install/update Go and frontend dependencies"
	@echo "  make dev          - Run both Go Daemon (:5181) and Vue 3 Frontend (:5180) (auto-installs dependencies)"
	@echo "  make run-daemon   - Start the Go Daemon server (:5181)"
	@echo "  make run-web      - Start the Vite dev frontend (:5180)"
	@echo "  make build        - Compile Go daemon binary and build frontend production bundle"
	@echo "  make test         - Run all Go test suites and Vue TypeScript typechecks"
	@echo "  make clean        - Remove build artifacts and temporary binaries"
	@echo "  make clean-all    - Remove build artifacts, binaries, and frontend node_modules"

# Verify required runtime tools exist
check-prereqs:
	@command -v go >/dev/null 2>&1 || { echo "Error: 'go' is not installed or not in PATH. Please install Go (1.22+): https://go.dev/dl/" >&2; exit 1; }
	@command -v node >/dev/null 2>&1 || { echo "Error: 'node' is not installed or not in PATH. Please run 'make setup' or install Node.js: https://nodejs.org/" >&2; exit 1; }
	@command -v npm >/dev/null 2>&1 || { echo "Error: 'npm' is not installed or not in PATH. Please install Node.js/npm." >&2; exit 1; }

# Ensure Go dependencies are downloaded
install-go:
	@echo "Checking Go dependencies..."
	@go mod download

# Ensure frontend dependencies are installed
install-web:
	@if [ ! -d "web/node_modules" ] || [ ! -f "web/node_modules/.bin/vite" ]; then \
		echo "Frontend dependencies not found. Installing via 'npm install' in web/..."; \
		(cd web && npm install) || exit 1; \
	fi

# Install all backend and frontend dependencies
install-deps: install-go
	@echo "Installing frontend dependencies..."
	cd web && npm install

# Install every missing required tool from configs/packages.json and all project dependencies
setup:
	@echo "1/3 Checking system & runtime tools (Node.js, Git, Claude Code CLI)..."
	go run ./cmd/daemon packages setup
	@echo "2/3 Downloading Go dependencies..."
	@go mod download
	@echo "3/3 Installing frontend dependencies..."
	cd web && npm install
	@echo "Platform setup complete! You can now run 'make dev'."

# Start Go Daemon (:5181)
run-daemon: check-prereqs install-go
	@echo "Starting Meta-Orchestrator Go Daemon on :5181..."
	go run cmd/daemon/main.go

# Start Frontend Dev Server (:5180)
run-web: check-prereqs install-web
	@echo "Starting Frontend Mission Control on :5180..."
	cd web && npm run dev

# Run development mode (Daemon in background, Web in foreground)
dev: check-prereqs install-go install-web
	@echo "Launching Meta-Orchestrator Platform in Development Mode..."
	@echo "Backend API & WS: http://localhost:5181"
	@echo "Frontend Web:     http://127.0.0.1:5180"
	@trap 'kill 0' EXIT; \
	go run cmd/daemon/main.go & \
	cd web && npm run dev

# Build both backend binary and frontend static bundle
build: build-daemon build-web
	@echo "Full platform successfully built!"

build-daemon: check-prereqs
	@echo "Compiling daemon binary..."
	mkdir -p bin
	go build -ldflags="-s -w" -o bin/daemon cmd/daemon/main.go

build-web: check-prereqs install-web
	@echo "Building frontend production assets..."
	cd web && npm run build

# Run all test suites
test: install-web
	@echo "Running backend test suites..."
	go test -v -race ./...
	@echo "Checking frontend TypeScript types..."
	cd web && npx vue-tsc --noEmit

# Clean build artifacts
clean:
	@echo "Cleaning artifacts..."
	rm -rf bin/
	rm -rf web/dist/

# Clean build artifacts and node_modules
clean-all: clean
	@echo "Cleaning frontend node_modules..."
	rm -rf web/node_modules/
