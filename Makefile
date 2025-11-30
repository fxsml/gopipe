.PHONY: help test test-verbose test-race test-coverage build clean lint fmt vet examples tidy bench check install-tools

# Default Go binary
GO := go

# Coverage output file
COVERAGE_FILE := coverage.out

# Default target
.DEFAULT_GOAL := help

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

test: ## Run tests
	$(GO) test -v ./...

test-verbose: ## Run tests with verbose output
	$(GO) test -v -count=1 ./...

test-race: ## Run tests with race detector
	$(GO) test -v -race ./...

test-coverage: ## Run tests with coverage report
	$(GO) test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic ./...
	$(GO) tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report generated: coverage.html"

bench: ## Run benchmarks
	$(GO) test -bench=. -benchmem ./...

build: ## Build the library (validate it compiles)
	$(GO) build -v ./...

examples: ## Build all examples
	@for example in examples/*/; do \
		echo "Building $$example..."; \
		$(GO) build -v "./$$example" || exit 1; \
	done

clean: ## Clean build artifacts and cache
	$(GO) clean -cache -testcache -modcache
	rm -f $(COVERAGE_FILE) coverage.html

lint: ## Run linter (requires golangci-lint)
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run 'make install-tools' first." && exit 1)
	golangci-lint run --timeout=5m

fmt: ## Format code
	$(GO) fmt ./...

vet: ## Run go vet
	$(GO) vet ./...

tidy: ## Tidy go modules
	$(GO) mod tidy
	$(GO) mod verify

check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

install-tools: ## Install development tools
	@echo "Installing golangci-lint..."
	@which golangci-lint > /dev/null || \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin

ci: tidy lint test-race build examples ## Run CI checks locally
