.PHONY: build install test clean run help

# Build variables
BINARY_NAME=apiproxy
VERSION?=0.1.0
COMMIT=$(shell git rev-parse --short HEAD 2>/dev/null || echo "dev")
BUILD_DATE=$(shell date -u +"%Y-%m-%dT%H:%M:%SZ")
LDFLAGS=-ldflags "-X main.version=$(VERSION) -X main.commit=$(COMMIT) -X main.date=$(BUILD_DATE)"

help: ## Show this help message
	@echo 'Usage: make [target]'
	@echo ''
	@echo 'Available targets:'
	@awk 'BEGIN {FS = ":.*?## "} /^[a-zA-Z_-]+:.*?## / {printf "  %-15s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build the binary
	@echo "Building $(BINARY_NAME)..."
	go build $(LDFLAGS) -o $(BINARY_NAME) main.go
	@echo "✅ Built: $(BINARY_NAME)"

build-all: ## Build for all platforms
	@echo "Building for multiple platforms..."
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-linux-amd64 main.go
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-amd64 main.go
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o $(BINARY_NAME)-darwin-arm64 main.go
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o $(BINARY_NAME)-windows-amd64.exe main.go
	@echo "✅ Built all platforms"

install: build ## Install to /usr/local/bin
	@echo "Installing to /usr/local/bin..."
	sudo mv $(BINARY_NAME) /usr/local/bin/
	@echo "✅ Installed: /usr/local/bin/$(BINARY_NAME)"

test: ## Run tests
	@echo "Running tests..."
	go test -v ./...

test-integration: build ## Run integration tests
	@echo "Running integration tests..."
	./$(BINARY_NAME) test

run: build ## Build and run daemon
	@echo "Starting daemon..."
	./$(BINARY_NAME) daemon start

clean: ## Clean build artifacts
	@echo "Cleaning..."
	rm -f $(BINARY_NAME)
	rm -f $(BINARY_NAME)-*
	rm -f ~/.apiproxy/cache.db
	@echo "✅ Cleaned"

deps: ## Download dependencies
	@echo "Downloading dependencies..."
	go mod download
	go mod tidy
	@echo "✅ Dependencies updated"

docker-build: ## Build Docker image
	@echo "Building Docker image..."
	docker build -t apiproxyd:$(VERSION) .
	@echo "✅ Built: apiproxyd:$(VERSION)"

docker-run: ## Run Docker container
	@echo "Running Docker container..."
	docker run -p 9002:9002 -v $(PWD)/config.json:/app/config.json:ro apiproxyd:$(VERSION)

init-config: ## Initialize default config
	@echo "Creating config.json..."
	cp config.json.example config.json
	@echo "✅ Created config.json - edit with your API key"

dev: ## Run in development mode
	@echo "Running in development mode..."
	go run main.go daemon start --debug
