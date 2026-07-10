BINARY      := prunejuice
CMD         := ./cmd/prunejuice
BIN_DIR     := bin
VERSION     ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS     := -s -w -X main.Version=$(VERSION)

.PHONY: all build install test race lint fmt vet tidy clean run-once print-config docker-build docker-up docker-logs help

all: lint test build ## Lint, test and build

build: ## Build the binary into ./bin
	@mkdir -p $(BIN_DIR)
	go build -ldflags "$(LDFLAGS)" -o $(BIN_DIR)/$(BINARY) $(CMD)

install: ## go install the CLI
	go install -ldflags "$(LDFLAGS)" $(CMD)

test: ## Run tests with the race detector
	go test -race -count=1 ./...

lint: fmt vet ## Formatting + go vet (+ golangci-lint if present)
	@command -v golangci-lint >/dev/null 2>&1 && golangci-lint run || echo "golangci-lint not installed, skipping"

fmt: ## Check formatting
	@out=$$(gofmt -l .); if [ -n "$$out" ]; then echo "gofmt needed:"; echo "$$out"; exit 1; fi

vet: ## go vet
	go vet ./...

tidy: ## Tidy modules
	go mod tidy

clean: ## Remove build artifacts
	rm -rf $(BIN_DIR)

run-once: build ## Build and run a single check with the example config
	$(BIN_DIR)/$(BINARY) -config configs/config.example.yaml

print-config: build ## Print the default config
	$(BIN_DIR)/$(BINARY) -print-config

docker-build: ## Build the Docker image (prunejuice:latest)
	docker build --build-arg VERSION=$(VERSION) -t prunejuice:$(VERSION) -t prunejuice:latest .

docker-up: ## Build & start the container via docker compose (needs prunejuice.env)
	docker compose up -d --build

docker-logs: ## Follow the container logs
	docker compose logs -f

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'
