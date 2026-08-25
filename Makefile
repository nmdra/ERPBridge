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

.PHONY: all build clean test lint run-mock run-server generate-tools setup web-install web-build web-test web-lint

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

# Run linter (requires golangci-lint)
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

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

# Generate and apply tools for the ERP module
generate-tools: build
	@echo "Fetching Mock ERP OpenAPI $(MOCK_ERP_VERSION)..."
	@curl --fail --location --silent --show-error "$(MOCK_ERP_OPENAPI_URL)" -o "$(MOCK_ERP_OPENAPI_FILE)"
	@echo "Generating and applying tools from Mock ERP OpenAPI..."
	@# Ensure server is running or use a temporary one; this target assumes it is reachable.
	@./$(BINARY_CLI) api register --name erp --url http://localhost:$(MOCK_ERP_PORT) --module erp --description "Mock ERP"
	@mkdir -p schemas/erp
	@./$(BINARY_CLI) tool generate --api erp --openapi $(MOCK_ERP_OPENAPI_FILE) -o yaml > schemas/erp/generated.yaml
	@env BRIDGE_MCP_SERVER="$${BRIDGE_MCP_SERVER:-http://localhost:8080}" ./$(BINARY_CLI) tool apply -f schemas/erp/
	@echo "Tools applied successfully."

# Help
help:
	@echo "Available targets:"
	@echo "  build           Build server and CLI binaries"
	@echo "  clean           Remove binaries and data"
	@echo "  test            Run Go tests"
	@echo "  lint            Run Go linter"
	@echo "  run-mock        Start the pinned Mock ERP Docker image"
	@echo "  run-server      Start ERPBridge Server"
	@echo "  setup           Install Go dependencies"
	@echo "  generate-tools  Fetch versioned OpenAPI, generate, and apply tools"
	@echo "  web-install     Install frontend dependencies"
	@echo "  web-build       Build embedded frontend assets"
	@echo "  web-test        Run frontend typecheck and tests"
	@echo "  web-lint        Run frontend lint"
