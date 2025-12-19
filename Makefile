.PHONY: help test test-race test-coverage build clean lint fmt vet tidy check install-tools version docs-lint

# Default Go binary
GO := go

# Modules in the workspace
MODULES := ./channel/... ./pipe/... ./message/...

# Coverage output file
COVERAGE_FILE := coverage.out

# Default target
.DEFAULT_GOAL := help

help: ## Display this help message
	@echo "Available targets:"
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

test: ## Run tests
	$(GO) test -v $(MODULES)

test-race: ## Run tests with race detector
	$(GO) test -v -race $(MODULES)

test-coverage: ## Run tests with coverage report
	$(GO) test -v -race -coverprofile=$(COVERAGE_FILE) -covermode=atomic $(MODULES)
	$(GO) tool cover -html=$(COVERAGE_FILE) -o coverage.html
	@echo "Coverage report generated: coverage.html"

build: ## Build the library (validate it compiles)
	$(GO) build -v $(MODULES)

clean: ## Clean build artifacts
	$(GO) clean -testcache
	rm -f $(COVERAGE_FILE) coverage.html

lint: ## Run linter (requires golangci-lint)
	@which golangci-lint > /dev/null || (echo "golangci-lint not installed. Run 'make install-tools' first." && exit 1)
	golangci-lint run --timeout=5m $(MODULES)

fmt: ## Format code
	$(GO) fmt $(MODULES)

vet: ## Run go vet
	$(GO) vet $(MODULES)

tidy: ## Tidy go modules
	cd channel && $(GO) mod tidy
	cd pipe && $(GO) mod tidy
	cd message && $(GO) mod tidy

check: fmt vet test ## Run all checks (fmt, vet, test)

version: ## Show current version using git-semver
	@which git-semver > /dev/null || (echo "git-semver not installed. Run 'make install-tools' first." && exit 1)
	@git-semver

install-tools: ## Install development tools (golangci-lint, git-semver)
	@echo "Installing golangci-lint..."
	@which golangci-lint > /dev/null || \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $$(go env GOPATH)/bin
	@echo "Installing git-semver..."
	@which git-semver > /dev/null || \
		$(GO) install github.com/mdomke/git-semver/v6@v6.10.0

docs-lint: ## Run documentation lint procedure via Claude
	@which claude > /dev/null || (echo "claude CLI not installed" && exit 1)
	@claude -p "Execute the Documentation Lint Procedure from docs/procedures/documentation.md. Run ALL steps. Output a report of changes made."
