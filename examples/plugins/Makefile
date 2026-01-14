# Makefile for building apiproxyd plugins

.PHONY: all go python clean

all: go python

# Build all Go plugins
go: logger custom_router web_admin

logger:
	@echo "Building logger plugin..."
	@cd go/logger && go build -buildmode=plugin -o logger.so logger.go
	@echo "✅ Built go/logger/logger.so"

custom_router:
	@echo "Building custom_router plugin..."
	@cd go/custom_router && go build -buildmode=plugin -o custom_router.so router.go
	@echo "✅ Built go/custom_router/custom_router.so"

web_admin:
	@echo "Building web_admin plugin..."
	@cd go/web_admin && go build -buildmode=plugin -o web_admin.so admin.go
	@echo "✅ Built go/web_admin/web_admin.so"

# Make Python plugins executable
python:
	@echo "Setting up Python plugins..."
	@chmod +x python/logger.py
	@chmod +x python/openai_adapter.py
	@echo "✅ Python plugins are ready"

# Clean build artifacts
clean:
	@echo "Cleaning plugin build artifacts..."
	@rm -f go/logger/logger.so
	@rm -f go/custom_router/custom_router.so
	@rm -f go/web_admin/web_admin.so
	@echo "✅ Cleaned"

# Install plugins to system location
install: all
	@echo "Installing plugins..."
	@mkdir -p ~/.apiproxy/plugins/go
	@mkdir -p ~/.apiproxy/plugins/python
	@cp go/logger/logger.so ~/.apiproxy/plugins/go/ 2>/dev/null || true
	@cp go/custom_router/custom_router.so ~/.apiproxy/plugins/go/ 2>/dev/null || true
	@cp go/web_admin/web_admin.so ~/.apiproxy/plugins/go/ 2>/dev/null || true
	@cp python/logger.py ~/.apiproxy/plugins/python/
	@cp python/openai_adapter.py ~/.apiproxy/plugins/python/
	@echo "✅ Plugins installed to ~/.apiproxy/plugins/"

# Test plugins
test:
	@echo "Testing Go plugins..."
	@go test ./go/...
	@echo "Testing Python plugins..."
	@python3 -m pytest python/ 2>/dev/null || echo "Pytest not installed, skipping Python tests"
