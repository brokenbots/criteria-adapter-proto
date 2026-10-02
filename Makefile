# Makefile — criteria-adapter-proto
#
# Build/test plus the security & dependency-freshness tooling required by the
# Criteria supply-chain policy (mirrors the monorepo's WS49/WS50). See
# docs/dependency-policy.md. Tool versions are pinned (no floating @latest) so CI
# and local runs resolve the same version.

GO ?= go

OSV_SCANNER_VERSION     := v2.3.8
GO_MOD_OUTDATED_VERSION := v0.9.0
GOMAJOR_VERSION         := v0.15.0
BUF_VERSION             := v1.54.0
# BREAK_TAG is the tag proto-lint runs buf breaking against (additive-only
# waves must stay wire-compatible with the last released contract).
BREAK_TAG               := v0.6.0

.PHONY: help build test vet tidy vuln-scan deps-outdated deps-majors proto proto-lint

help: ## List targets
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  %-16s %s\n", $$1, $$2}'

build: ## Build the Go bindings
	$(GO) build ./...

test: ## Run Go tests
	$(GO) test ./...

vet: ## go vet
	$(GO) vet ./...

tidy: ## go mod tidy
	$(GO) mod tidy

# --- Security gate (WS49) -----------------------------------------------------

vuln-scan: ## Scan for known vulnerabilities (osv-scanner; local parity with CI osv-scan)
	$(GO) run github.com/google/osv-scanner/v2/cmd/osv-scanner@$(OSV_SCANNER_VERSION) scan source -r .

# --- Dependency freshness (WS50) ---------------------------------------------

deps-outdated: ## Report direct Go deps behind their latest minor/patch (go-mod-outdated)
	$(GO) list -u -m -json all | $(GO) run github.com/psampaz/go-mod-outdated@$(GO_MOD_OUTDATED_VERSION) -update -direct

deps-majors: ## List available Go major-version (/vN) upgrades (gomajor)
	$(GO) run github.com/icholy/gomajor@$(GOMAJOR_VERSION) list

# --- Proto contract gates -----------------------------------------------------

# BUF runs via `go run` (pinned) so CI and locals resolve the same buf without
# a standalone binary install, matching the security-gate pattern above.
BUF := $(GO) run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION)

proto: ## Regenerate the committed Go bindings (criteria/v2) from proto/criteria/v2
	$(BUF) generate --template buf.gen.go.yaml

proto-lint: ## buf lint + wire-compat check against the last released tag
	$(BUF) lint
	$(BUF) breaking --against ".git#tag=$(BREAK_TAG)"
