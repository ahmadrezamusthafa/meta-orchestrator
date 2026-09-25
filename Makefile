.PHONY: all dev run-daemon run-web build build-daemon build-web test clean help

# Default target
all: build

# Display help commands
help:
	@echo "AI-Driven SDLC Meta-Orchestrator Control Targets:"
	@echo "  make dev          - Run both Go Daemon (:8080) and Vue 3 Frontend (:5173)"
	@echo "  make run-daemon   - Start the Go Daemon server (:8080)"
	@echo "  make run-web      - Start the Vite dev frontend (:5173)"
	@echo "  make build        - Compile Go daemon binary and build frontend production bundle"
	@echo "  make test         - Run all Go test suites and Vue TypeScript typechecks"
	@echo "  make clean        - Remove build artifacts and temporary binaries"

# Start Go Daemon (:8080)
run-daemon:
	@echo "Starting Meta-Orchestrator Go Daemon on :8080..."
	go run cmd/daemon/main.go

# Start Frontend Dev Server (:5173)
run-web:
	@echo "Starting Frontend Mission Control on :5173..."
	cd web && npm run dev

# Run development mode (Daemon in background, Web in foreground)
dev:
	@echo "Launching Meta-Orchestrator Platform in Development Mode..."
	@echo "Backend API & WS: http://localhost:8080"
	@echo "Frontend Web:     http://localhost:5173"
	@trap 'kill 0' EXIT; \
	go run cmd/daemon/main.go & \
	cd web && npm run dev

# Build both backend binary and frontend static bundle
build: build-daemon build-web
	@echo "Full platform successfully built!"

build-daemon:
	@echo "Compiling daemon binary..."
	mkdir -p bin
	go build -ldflags="-s -w" -o bin/daemon cmd/daemon/main.go

build-web:
	@echo "Building frontend production assets..."
	cd web && npm run build

# Run all test suites
test:
	@echo "Running backend test suites..."
	go test -v -race ./...
	@echo "Checking frontend TypeScript types..."
	cd web && npx vue-tsc --noEmit

# Clean build artifacts
clean:
	@echo "Cleaning artifacts..."
	rm -rf bin/
	rm -rf web/dist/
