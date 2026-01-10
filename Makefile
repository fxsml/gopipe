.PHONY: help test test-race test-coverage build clean lint fmt vet tidy check install-tools version docs-lint merge-feature release

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
	@echo "Running documentation lint..."
	@claude -p "Execute the 'Documentation Lint Procedure' section from docs/procedures/documentation.md. \
		Run Steps 1-6 in order: \
		1. Check broken internal links in docs/ \
		2. Verify ADR index matches files \
		3. Verify procedures index matches files \
		4. Check CHANGELOG structure \
		5. Check plans index matches files \
		6. Output a summary report \
		\
		Fix any issues you find automatically. Report what was checked and what was fixed."

merge-feature: ## Merge feature branch to develop via Claude (BRANCH=xxx)
	@which claude > /dev/null || (echo "claude CLI not installed" && exit 1)
	@if [ -z "$(BRANCH)" ]; then echo "Error: BRANCH is required. Usage: make merge-feature BRANCH=feature/xxx"; exit 1; fi
	@echo "Starting feature merge procedure for $(BRANCH) → develop"
	@claude -p "Execute Phases 1-3 of the Feature Branch Release Procedure from docs/procedures/feature-release.md. \
		Branch: $(BRANCH) \
		\
		This merges the feature branch to develop with proper history cleanup. \
		\
		CRITICAL: You MUST pause and ask for my explicit approval before EVERY: \
		1. Interactive rebase or history rewrite \
		2. Force push \
		3. Merge to develop \
		\
		Show me the current state and what you're about to do, then wait for 'yes' or 'approved' before proceeding. \
		If I say 'skip', skip that phase. If I say 'abort', stop entirely."

release: ## Release develop to main with tags via Claude (VERSION=vX.Y.Z)
	@which claude > /dev/null || (echo "claude CLI not installed" && exit 1)
	@if [ -z "$(VERSION)" ]; then echo "Error: VERSION is required. Usage: make release VERSION=v0.11.0"; exit 1; fi
	@echo "Starting release procedure for develop → main ($(VERSION))"
	@claude -p "Execute Phases 4-6 of the Feature Branch Release Procedure from docs/procedures/feature-release.md. \
		Version: $(VERSION) \
		\
		This releases develop to main with proper tagging. \
		\
		CRITICAL: You MUST pause and ask for my explicit approval before EVERY: \
		1. Merge to main \
		2. Tag creation \
		3. Tag push \
		4. GitHub release creation \
		\
		Show me the current state and what you're about to do, then wait for 'yes' or 'approved' before proceeding. \
		If I say 'skip', skip that phase. If I say 'abort', stop entirely."
