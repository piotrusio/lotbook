GOLANGCI_LINT_VERSION := v2.15.0
BIN := $(CURDIR)/bin
GOLANGCI_LINT := $(BIN)/golangci-lint

CYAN  := \033[36m
GREEN := \033[32m
RESET := \033[0m

.DEFAULT_GOAL := help

# ============================================================
#  SETUP
# ============================================================

.PHONY: setup
setup: tools hooks  ## Initial setup — installs tooling and git hooks
	@printf "%b\n" "$(GREEN)✓ Setup complete.$(RESET)"

# golangci-lint is pinned and installed into ./bin rather than tracked as a
# go tool: its dependencies would otherwise be resolved together with ours.
# lefthook and govulncheck are go tools, pinned in go.mod.
.PHONY: tools
tools: $(GOLANGCI_LINT)  ## Install pinned tooling into ./bin

$(GOLANGCI_LINT):
	@printf "%b\n" "$(CYAN)→ Installing golangci-lint $(GOLANGCI_LINT_VERSION)...$(RESET)"
	GOBIN=$(BIN) go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: hooks
hooks:  ## Register git hooks
	go tool lefthook install

.PHONY: hooks-run
hooks-run:  ## Run the pre-commit hooks against all files
	go tool lefthook run pre-commit --all-files

# ============================================================
#  BUILD & RUN
# ============================================================

.PHONY: build
build:  ## Build the lotbook binary into ./bin
	go build -o $(BIN)/lotbook ./cmd/lotbook

.PHONY: run
run:  ## Run lotbook
	go run ./cmd/lotbook

# ============================================================
#  GATES
# ============================================================

.PHONY: check
check: lint test vuln tidy-check  ## Everything CI runs

.PHONY: test
test:  ## Run tests with the race detector
	go test -race -count=1 ./...

.PHONY: lint
lint: $(GOLANGCI_LINT)  ## Run golangci-lint
	$(GOLANGCI_LINT) run ./...

# The test step of the flow commits tests that do not compile yet — in Go that
# is what red looks like before the implementation exists. This lints the
# implementation alone, so that step can still be held to a clean bar. It is
# the hooks' target, not a substitute for lint.
.PHONY: lint-impl
lint-impl: $(GOLANGCI_LINT)  ## Run golangci-lint, implementation only (ignores _test.go)
	$(GOLANGCI_LINT) run --tests=false ./...

.PHONY: fix
fix: $(GOLANGCI_LINT)  ## Auto-fix lint issues (formatting, godot, misspell…)
	$(GOLANGCI_LINT) run --fix ./...

.PHONY: fmt
fmt:  ## Format code
	gofmt -w .

.PHONY: vuln
vuln:  ## Check dependencies and the toolchain for known vulnerabilities
	go tool govulncheck ./...

.PHONY: tidy-check
tidy-check:  ## Fail if go.mod or go.sum is not tidy
	go mod tidy -diff

# ============================================================
#  HELP
# ============================================================

.PHONY: help
help:  ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  $(CYAN)%-12s$(RESET) %s\n", $$1, $$2}'
