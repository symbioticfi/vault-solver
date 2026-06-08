# vault-solver — developer & CI tasks.
#
# Generated code (api/bindings, api/threef) is committed for a hermetic build; the refresh-*
# targets regenerate it from upstream on demand.

SHELL := bash
.DEFAULT_GOAL := help

# Pinned codegen tool versions.
ABIGEN_VERSION        ?= v1.16.1
OAPI_CODEGEN_VERSION  ?= v2.4.1
GOLANGCI_LINT_VERSION ?= v2.11.4

# Foundry build output to vendor ABIs from (sibling rfq repo by default).
FORGE_OUT ?= ../rfq/out
# Live 3F OpenAPI spec (Sepolia dev).
OPENAPI_URL ?= https://bf.dev.gcp.3f.xyz/docs/openapi.json
# RFQ backend OpenAPI spec. The backend serves it at /api/v1/openapi.json (hono-openapi, runtime).
# NOTE: the temp railway deployment is currently behind the repo (pre adapter/protocolSignature
# rename); point this at a backend running current code, or regenerate in-repo (see docs/RFQ-PLAN.md).
RFQ_OPENAPI_URL ?= https://backend-production-a0ca.up.railway.app/api/v1/openapi.json

# Contracts whose ABIs are vendored from a Foundry build via refresh-abi.
ABIS := BridgeFacilitatorAdapter IVaultV2 IRequest IVaultController IWhitelist \
        InstantRedemptionAdapter Executor Reactor ICuratorRegistry

# Contract:relpath mapping for Go bindings. Each contract gets its own package (the leaf dir) so
# shared ABI structs (e.g. the `Offer` tuple in both the adapter and IRequest) don't collide.
# Adapter-specific bindings are grouped per integration (3f/, and later rfq/, oev/); shared
# infra (vaultv2, multicall3) stays top-level so every integration reuses it.
# Note: api/abi/Multicall3.json is hand-vendored (not a Foundry contract), so Multicall3 is in
# BINDINGS but not ABIS. aggregate3 is marked `view` there so abigen binds it as a Caller.
BINDINGS := BridgeFacilitatorAdapter:3f/adapter IRequest:3f/request \
            IVaultController:3f/vaultcontroller IWhitelist:3f/whitelist \
            InstantRedemptionAdapter:rfq/adapter Executor:rfq/executor \
            Reactor:rfq/reactor ICuratorRegistry:rfq/curatorregistry \
            IVaultV2:vaultv2 Multicall3:multicall3

BIN     := bin/vault-solver
PKG     := github.com/symbioticfi/vault-solver
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT  ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo none)
DATE    ?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS := -X $(PKG)/internal/version.Version=$(VERSION) \
           -X $(PKG)/internal/version.Commit=$(COMMIT) \
           -X $(PKG)/internal/version.Date=$(DATE)

.PHONY: help
help: ## List available targets
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) | \
		awk 'BEGIN{FS=":.*?## "}{printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'

.PHONY: tools
tools: ## Install pinned codegen + lint tools
	go install github.com/ethereum/go-ethereum/cmd/abigen@$(ABIGEN_VERSION)
	go install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

.PHONY: refresh-abi
refresh-abi: ## Re-vendor ABIs from a Foundry out/ dir (FORGE_OUT=...)
	@mkdir -p api/abi
	@for c in $(ABIS); do \
		src="$(FORGE_OUT)/$$c.sol/$$c.json"; \
		if [[ ! -f "$$src" ]]; then echo "missing $$src (run forge build in $(FORGE_OUT)/..)"; exit 1; fi; \
		jq '.abi' "$$src" > "api/abi/$$c.json"; \
		echo "vendored api/abi/$$c.json"; \
	done

.PHONY: refresh-openapi
refresh-openapi: ## Re-pull the live 3F OpenAPI spec
	@mkdir -p openapi
	curl -fsSL "$(OPENAPI_URL)" | jq . > openapi/3f-bf.openapi.json
	@echo "vendored openapi/3f-bf.openapi.json"

.PHONY: refresh-rfq-openapi
refresh-rfq-openapi: ## Re-pull the RFQ backend OpenAPI spec (RFQ_OPENAPI_URL=...)
	@mkdir -p openapi
	curl -fsSL "$(RFQ_OPENAPI_URL)" | jq . > openapi/rfq-backend.openapi.json
	@echo "vendored openapi/rfq-backend.openapi.json (verify field names — see docs/RFQ-PLAN.md)"

.PHONY: bindings
bindings: ## Generate Go bindings from vendored ABIs (grouped per integration; package = leaf dir)
	@for pair in $(BINDINGS); do \
		c="$${pair%%:*}"; rel="$${pair##*:}"; pkg="$${rel##*/}"; \
		abi="api/abi/$$c.json"; \
		if [[ ! -f "$$abi" ]]; then echo "missing $$abi (run make refresh-abi)"; exit 1; fi; \
		mkdir -p "api/bindings/$$rel"; \
		abigen --abi "$$abi" --pkg "$$pkg" --type "$$c" --out "api/bindings/$$rel/$$c.go"; \
		echo "generated api/bindings/$$rel/$$c.go"; \
	done

.PHONY: openapi-client
openapi-client: ## Generate the 3F API client from the vendored spec
	@mkdir -p api/threef
	oapi-codegen -package threef -generate types,client \
		-o api/threef/client.gen.go openapi/3f-bf.openapi.json

.PHONY: generate
generate: bindings openapi-client ## Regenerate all committed codegen

.PHONY: build
build: ## Build the binary
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/vault-solver

.PHONY: test
test: ## Run tests with race detector + coverage
	go test -race -cover ./...

.PHONY: format
format: ## Run golangci-lint
	golangci-lint run --fix

.PHONY: tidy
tidy: ## Tidy and verify go.mod / go.sum
	go mod tidy
	go mod verify

.PHONY: docker
docker: ## Build the container image
	docker build -t vault-solver:$(VERSION) -f deploy/Dockerfile .

.PHONY: clean
clean: ## Remove build artifacts
	rm -rf bin dist
