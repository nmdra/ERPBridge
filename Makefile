# Variables
BINARY_SERVER=erpbridge-server
BINARY_CLI=bridgectl
BUILD_DIR=dist
DB_PATH=data/erpbridge.db
MOCK_ERP_IMAGE ?= ghcr.io/nmdra/mockerp:0.2.1
MOCK_ERP_VERSION ?= 0.2.1
MOCK_ERP_PORT=8081
MOCK_ERP_OPENAPI_URL ?= https://raw.githubusercontent.com/nmdra/mockerp/v$(MOCK_ERP_VERSION)/openapi.yaml
MOCK_ERP_OPENAPI_FILE ?= /tmp/mockerp-openapi-v$(MOCK_ERP_VERSION).yaml
SERVER_PORT=8080

.PHONY: all build clean test bench stress lint dev-up run-mock run-server generate-tools setup test-plugin-integration web-install web-build web-test web-lint

all: build

# Build both server and CLI
build: web-build
	@echo "Building binaries..."
	@go build -o $(BINARY_SERVER) ./services/erpbridge-server/main.go
	@go build -o $(BINARY_CLI) ./tools/bridgectl/main.go

# Clean up binaries and data
clean:
	@echo "Cleaning up..."
	@rm -f $(BINARY_SERVER) $(BINARY_CLI)
	@rm -rf $(BUILD_DIR)
	@rm -rf internal/web/prebuilt/build/assets internal/web/prebuilt/build/index.html
	@rm -f $(DB_PATH)
	@rm -rf schemas/erp

# Run frontend checks without a browser
web-install:
	@npm ci --prefix web

web-build: web-install
	@npm run build --prefix web

web-test: web-install
	@npm run typecheck --prefix web
	@npm test --prefix web -- --run
	@npm run format-check --prefix web

web-lint: web-install
	@npm run lint --prefix web

# Run tests
test:
	@echo "Running tests..."
	@go test ./...

# Run deterministic in-process Go microbenchmarks without ordinary tests.
bench:
	@go test -run '^$$' -bench . -benchmem ./internal/cache ./internal/mcp

# Stress the in-memory cache-hit path with concurrent callers.
stress:
	@go test -run '^$$' -bench '^BenchmarkCacheMiddleware/hit_parallel$$' -benchmem -cpu=1,8 -benchtime=5s ./internal/mcp

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# Run the isolated external-plugin black-box integration test
test-plugin-integration:
	@./scripts/test-plugin-integration.sh

# Start the local Compose stack with safe ephemeral credentials by default
dev-up:
	@./scripts/dev-stack.sh

# Start the pinned Mock ERP image
run-mock:
	@echo "Starting Mock ERP image $(MOCK_ERP_IMAGE) on port $(MOCK_ERP_PORT)..."
	@env MOCK_ERP_IMAGE="$(MOCK_ERP_IMAGE)" docker compose up -d mock-erp

# Start the ERPBridge server
run-server: build
	@echo "Starting ERPBridge Server on port $(SERVER_PORT)..."
	@env DATABASE_PATH="$(DB_PATH)" ./$(BINARY_SERVER)

# Setup development environment
setup:
	@echo "Setting up development environment..."
	@go mod tidy

# Generate one temporary draft manifest and apply it once for the ERP module
generate-tools: build
	@set -eu; \
	openapi_file="$(MOCK_ERP_OPENAPI_FILE)"; \
	manifest_file=$$(mktemp "$${TMPDIR:-/tmp}/erpbridge-generated.XXXXXX.yaml"); \
	trap 'rm -f "$$openapi_file" "$$manifest_file"' EXIT HUP INT TERM; \
	echo "Fetching Mock ERP OpenAPI $(MOCK_ERP_VERSION)..."; \
	curl --fail --location --silent --show-error "$(MOCK_ERP_OPENAPI_URL)" -o "$$openapi_file"; \
	echo "Generating and applying one draft manifest from Mock ERP OpenAPI..."; \
	./$(BINARY_CLI) api register --name erp --url http://localhost:$(MOCK_ERP_PORT) --module erp --description "Mock ERP"; \
	./$(BINARY_CLI) tool generate --api erp --openapi "$$openapi_file" -o yaml > "$$manifest_file"; \
	env BRIDGE_MCP_SERVER="$${BRIDGE_MCP_SERVER:-http://localhost:8080}" ./$(BINARY_CLI) tool apply -f "$$manifest_file"; \
	echo "Tools applied successfully."

# Help
help:
	@echo "Available targets:"
	@echo "  build           Build server and CLI binaries"
	@echo "  clean           Remove binaries and data"
	@echo "  test            Run Go tests"
	@echo "  bench           Run Go microbenchmarks"
	@echo "  stress          Stress concurrent in-memory cache hits"
	@echo "  lint            Run Go linter"
	@echo "  dev-up          Start the local Compose stack safely"
	@echo "  run-mock        Start the pinned Mock ERP Docker image"
	@echo "  run-server      Start ERPBridge Server"
	@echo "  setup           Install Go dependencies"
	@echo "  generate-tools  Fetch versioned OpenAPI, generate, and apply tools"
	@echo "  test-plugin-integration  Run the isolated external-plugin integration test"
	@echo "  web-install     Install frontend dependencies"
	@echo "  web-build       Build embedded frontend assets"
	@echo "  web-test        Run frontend typecheck and tests"
	@echo "  web-lint        Run frontend lint"
