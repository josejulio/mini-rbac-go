.PHONY: build run test clean migrate dev db-start db-stop db-logs db-shell db-status build-release

# Version and build information
VERSION ?= dev
COMMIT ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo "unknown")
BUILD_TIME ?= $(shell date -u '+%Y-%m-%d_%H:%M:%S')

# Go build flags
LDFLAGS = -X main.version=$(VERSION) -X main.commit=$(COMMIT)

# Container runtime detection (docker or podman)
CONTAINER_BIN ?= $(shell command -v podman 2>/dev/null || command -v docker 2>/dev/null)

# Database container configuration
DB_CONTAINER_NAME ?= mini-rbac-postgres
DB_IMAGE ?= postgres:15-alpine
DB_PORT ?= 15432
DB_USER ?= postgres
DB_PASSWORD ?= postgres
DB_NAME ?= rbac

# Build the application
build:
	@echo "Building mini-rbac-go..."
	@go build -ldflags "$(LDFLAGS)" -o bin/mini-rbac-go ./cmd/server/main.go
	@echo "✅ Build complete: bin/mini-rbac-go"
	@echo "   Version: $(VERSION)"
	@echo "   Commit: $(COMMIT)"

# Build release version
build-release:
	@echo "Building mini-rbac-go release..."
	@VERSION=$(VERSION) COMMIT=$(COMMIT) $(MAKE) build

# Run the application
run: build
	@echo "Starting mini-rbac-go..."
	@./bin/mini-rbac-go -config config/config.yaml

# Run in development mode (with hot reload would require additional tools)
dev:
	@go run cmd/server/main.go -config config/config.yaml

# Run tests (placeholder for Ginkgo tests)
test:
	@echo "Running tests..."
	@go test ./... -v

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	@rm -rf bin/
	@echo "✅ Clean complete"

# Format code
fmt:
	@echo "Formatting code..."
	@go fmt ./...
	@echo "✅ Format complete"

# Run linter
lint:
	@echo "Running linter..."
	@golangci-lint run ./...

# Tidy dependencies
tidy:
	@echo "Tidying dependencies..."
	@go mod tidy
	@echo "✅ Tidy complete"

# Initialize database (requires PostgreSQL running)
db-init:
	@echo "Initializing database..."
	@psql -U postgres -h localhost -p 15432 -c "CREATE DATABASE rbac;" || echo "Database may already exist"

# Drop database
db-drop:
	@echo "Dropping database..."
	@psql -U postgres -h localhost -p 15432 -c "DROP DATABASE IF EXISTS rbac;"

# Reset database
db-reset: db-drop db-init
	@echo "✅ Database reset complete"

# Start PostgreSQL container for development
db-start:
	@if [ -z "$(CONTAINER_BIN)" ]; then \
		echo "❌ Error: Neither docker nor podman found in PATH"; \
		echo "   Please install docker or podman, or set CONTAINER_BIN"; \
		exit 1; \
	fi
	@echo "🐘 Starting PostgreSQL container with $(CONTAINER_BIN)..."
	@if $(CONTAINER_BIN) ps -a --format "{{.Names}}" | grep -q "^$(DB_CONTAINER_NAME)$$"; then \
		echo "   Container $(DB_CONTAINER_NAME) already exists"; \
		if $(CONTAINER_BIN) ps --format "{{.Names}}" | grep -q "^$(DB_CONTAINER_NAME)$$"; then \
			echo "   Container is already running"; \
		else \
			echo "   Starting existing container..."; \
			$(CONTAINER_BIN) start $(DB_CONTAINER_NAME); \
		fi; \
	else \
		echo "   Creating new container..."; \
		$(CONTAINER_BIN) run -d \
			--name $(DB_CONTAINER_NAME) \
			-e POSTGRES_USER=$(DB_USER) \
			-e POSTGRES_PASSWORD=$(DB_PASSWORD) \
			-e POSTGRES_DB=$(DB_NAME) \
			-p $(DB_PORT):5432 \
			$(DB_IMAGE); \
		echo "   Waiting for PostgreSQL to be ready..."; \
		sleep 3; \
	fi
	@echo "✅ PostgreSQL is running on localhost:$(DB_PORT)"
	@echo "   Database: $(DB_NAME)"
	@echo "   User: $(DB_USER)"
	@echo "   Password: $(DB_PASSWORD)"

# Stop PostgreSQL container
db-stop:
	@if [ -z "$(CONTAINER_BIN)" ]; then \
		echo "❌ Error: Neither docker nor podman found in PATH"; \
		exit 1; \
	fi
	@echo "🛑 Stopping PostgreSQL container..."
	@if $(CONTAINER_BIN) ps --format "{{.Names}}" | grep -q "^$(DB_CONTAINER_NAME)$$"; then \
		$(CONTAINER_BIN) stop $(DB_CONTAINER_NAME); \
		echo "✅ Container stopped"; \
	else \
		echo "   Container is not running"; \
	fi

# Remove PostgreSQL container (stops and deletes)
db-clean:
	@if [ -z "$(CONTAINER_BIN)" ]; then \
		echo "❌ Error: Neither docker nor podman found in PATH"; \
		exit 1; \
	fi
	@echo "🗑️  Removing PostgreSQL container..."
	@if $(CONTAINER_BIN) ps -a --format "{{.Names}}" | grep -q "^$(DB_CONTAINER_NAME)$$"; then \
		$(CONTAINER_BIN) rm -f $(DB_CONTAINER_NAME); \
		echo "✅ Container removed"; \
	else \
		echo "   Container does not exist"; \
	fi

# Show PostgreSQL container logs
db-logs:
	@if [ -z "$(CONTAINER_BIN)" ]; then \
		echo "❌ Error: Neither docker nor podman found in PATH"; \
		exit 1; \
	fi
	@$(CONTAINER_BIN) logs -f $(DB_CONTAINER_NAME)

# Open psql shell in the container
db-shell:
	@if [ -z "$(CONTAINER_BIN)" ]; then \
		echo "❌ Error: Neither docker nor podman found in PATH"; \
		exit 1; \
	fi
	@echo "🐘 Connecting to PostgreSQL..."
	@$(CONTAINER_BIN) exec -it $(DB_CONTAINER_NAME) psql -U $(DB_USER) -d $(DB_NAME)

# Show database container status
db-status:
	@if [ -z "$(CONTAINER_BIN)" ]; then \
		echo "❌ Error: Neither docker nor podman found in PATH"; \
		exit 1; \
	fi
	@echo "📊 PostgreSQL Container Status:"
	@if $(CONTAINER_BIN) ps -a --format "{{.Names}}" | grep -q "^$(DB_CONTAINER_NAME)$$"; then \
		$(CONTAINER_BIN) ps -a --filter name=$(DB_CONTAINER_NAME) --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"; \
	else \
		echo "   Container does not exist. Run 'make db-start' to create it."; \
	fi

# Show help
help:
	@echo "Mini RBAC Go - Makefile commands:"
	@echo ""
	@echo "Build & Run:"
	@echo "  make build          - Build the application"
	@echo "  make build-release  - Build with version info (VERSION=x.y.z)"
	@echo "  make run            - Build and run the application"
	@echo "  make dev            - Run in development mode"
	@echo "  make test           - Run tests"
	@echo "  make clean          - Clean build artifacts"
	@echo "  make fmt            - Format code"
	@echo "  make tidy           - Tidy dependencies"
	@echo ""
	@echo "Database (Container):"
	@echo "  make db-start   - Start PostgreSQL container"
	@echo "  make db-stop    - Stop PostgreSQL container"
	@echo "  make db-clean   - Remove PostgreSQL container"
	@echo "  make db-logs    - Show PostgreSQL logs"
	@echo "  make db-shell   - Open psql shell"
	@echo "  make db-status  - Show container status"
	@echo ""
	@echo "Database (Legacy):"
	@echo "  make db-init    - Initialize database (requires local psql)"
	@echo "  make db-reset   - Reset database (requires local psql)"
	@echo ""
	@echo "Container Runtime:"
	@echo "  Detected: $(CONTAINER_BIN)"
	@echo "  Override with: make db-start CONTAINER_BIN=podman"
	@echo ""
