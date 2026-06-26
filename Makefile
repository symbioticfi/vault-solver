# vault-solver — developer & CI tasks.
#
# Generated code (api/bindings, api/threef) is committed for a hermetic build; the refresh-*
# targets regenerate it from upstream on demand.

SHELL := bash
.DEFAULT_GOAL := help

# Pinned codegen tool versions.
ABIGEN_VERSION           ?= v1.16.1
GOLANGCI_LINT_VERSION    ?= v2.11.4
# Java openapi-generator (downloaded on demand by hack/openapi-generator-cli.sh). 7.12.0 is the floor:
# it ingests OpenAPI 3.1 (the RFQ backend spec); 5.4.0/7.0.1 fail on it.
OPENAPI_GENERATOR_VERSION ?= 7.12.0

# Foundry build output to vendor ABIs from (sibling rfq repo by default).
FORGE_OUT ?= ../rfq/out
# core-mirror is a separate Foundry project (vendored as an rfq submodule). The LiquidLane adapter,
# the universal delegator, and the vault/ERC4626 interfaces live there and are built standalone
# (`cd ../rfq/lib/core-mirror && forge build`).
CORE_MIRROR_OUT ?= ../rfq/lib/core-mirror/out
# Live 3F OpenAPI spec (Sepolia dev).
OPENAPI_URL ?= https://bf.dev.gcp.3f.xyz/docs/openapi.json
# RFQ backend OpenAPI spec. The backend serves it at /api/v1/openapi.json (hono-openapi, runtime).
# NOTE: the temp railway deployment is currently behind the repo (pre adapter/protocolSignature
# rename); point this at a backend running current code, or regenerate in-repo (see docs/RFQ-PLAN.md).
RFQ_OPENAPI_URL ?= https://backend-production-a0ca.up.railway.app/api/v1/openapi.json

# Contracts whose ABIs are vendored via refresh-abi. ABIS come from the rfq Foundry build; the
# CORE_MIRROR_ABIS (LiquidLane adapter, universal delegator, vault/ERC4626 interfaces) come from the
# core-mirror build, since nothing in rfq/src imports them so they aren't in rfq/out.
ABIS := BridgeFacilitatorAdapter IRequest IVaultController IWhitelist Executor Reactor
CORE_MIRROR_ABIS := LiquidLaneAdapter IVaultV2 IERC4626
# api/abi/UniversalDelegator.json is hand-vendored to a minimal {limitOf} ABI (the full contract has
# an overloaded deallocateAll that abigen rejects, and the solver only reads limitOf) — like Multicall3.

# Contract:relpath mapping for Go bindings. Each contract gets its own package (the leaf dir) so
# shared ABI structs (e.g. the `Offer` tuple in both the adapter and IRequest) don't collide.
# Adapter-specific bindings are grouped per integration (3f/, and later rfq/, oev/); shared
# infra (vaultv2, multicall3) stays top-level so every integration reuses it.
# Leaf-contract bindings use abigen --v2, which emits typed, backend-free PackXxx/UnpackXxx helpers.
# The on-chain read paths build their Multicall3 sub-calls and decode the return blobs through those
# helpers (see the chainreaders), so an ABI change that renames a method or alters a signature breaks
# the build at the call site instead of panicking at runtime — no stringly-typed abi.Pack("method").
BINDINGS_V2 := BridgeFacilitatorAdapter:3f/adapter IRequest:3f/request \
            IVaultController:3f/vaultcontroller IWhitelist:3f/whitelist \
            LiquidLaneAdapter:rfq/adapter Executor:rfq/executor Reactor:rfq/reactor \
            UniversalDelegator:delegator IVaultV2:vaultv2 IERC4626:erc4626
# Note: api/abi/Multicall3.json is hand-vendored (not a Foundry contract), so Multicall3 is in
# BINDINGS_V1 but not ABIS. It stays on the v1 generator: it's the transport (chain.Multicall binds
# its Aggregate3 caller), where v2's pure pack/unpack helpers buy nothing. aggregate3 is marked `view`
# there so abigen binds it as a Caller.
BINDINGS_V1 := Multicall3:multicall3

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
	go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)
	@echo "OpenAPI clients use the Java openapi-generator via hack/openapi-generator-cli.sh (needs a JRE; jar auto-downloaded)."

.PHONY: refresh-abi
refresh-abi: ## Re-vendor ABIs from the rfq + core-mirror Foundry builds (FORGE_OUT=..., CORE_MIRROR_OUT=...)
	@mkdir -p api/abi
	@for c in $(ABIS); do \
		src="$(FORGE_OUT)/$$c.sol/$$c.json"; \
		if [[ ! -f "$$src" ]]; then echo "missing $$src (run forge build in $(FORGE_OUT)/..)"; exit 1; fi; \
		jq '.abi' "$$src" > "api/abi/$$c.json"; \
		echo "vendored api/abi/$$c.json"; \
	done
	@for c in $(CORE_MIRROR_ABIS); do \
		src="$(CORE_MIRROR_OUT)/$$c.sol/$$c.json"; \
		if [[ ! -f "$$src" ]]; then echo "missing $$src (run forge build in $(CORE_MIRROR_OUT)/..)"; exit 1; fi; \
		jq '.abi' "$$src" > "api/abi/$$c.json"; \
		echo "vendored api/abi/$$c.json (core-mirror)"; \
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
	@for pair in $(BINDINGS_V2); do \
		c="$${pair%%:*}"; rel="$${pair##*:}"; pkg="$${rel##*/}"; \
		abi="api/abi/$$c.json"; \
		if [[ ! -f "$$abi" ]]; then echo "missing $$abi (run make refresh-abi)"; exit 1; fi; \
		mkdir -p "api/bindings/$$rel"; \
		abigen --v2 --abi "$$abi" --pkg "$$pkg" --type "$$c" --out "api/bindings/$$rel/$$c.go"; \
		echo "generated api/bindings/$$rel/$$c.go (v2)"; \
	done
	@for pair in $(BINDINGS_V1); do \
		c="$${pair%%:*}"; rel="$${pair##*:}"; pkg="$${rel##*/}"; \
		abi="api/abi/$$c.json"; \
		if [[ ! -f "$$abi" ]]; then echo "missing $$abi (run make refresh-abi)"; exit 1; fi; \
		mkdir -p "api/bindings/$$rel"; \
		abigen --abi "$$abi" --pkg "$$pkg" --type "$$c" --out "api/bindings/$$rel/$$c.go"; \
		echo "generated api/bindings/$$rel/$$c.go (v1)"; \
	done

# Both OpenAPI clients are generated with the Java openapi-generator (via hack/openapi-generator-cli.sh,
# which downloads the pinned jar on demand — needs a JRE). It is the only generator that ingests the RFQ
# backend's OpenAPI 3.1 spec; we use it for the 3F (3.0) spec too for one toolchain. $(OPENAPI_GENERATOR_VERSION)
# is the floor — 5.4.0/7.0.1 fail on the 3.1 spec. The generated package is stdlib-only (no go.mod change);
# the recipes strip the generator's non-package cruft, keeping just the Go client.
define gen_openapi_client
	GO_POST_PROCESS_FILE='gofmt -w' OPENAPI_GENERATOR_VERSION=$(OPENAPI_GENERATOR_VERSION) bash ./hack/openapi-generator-cli.sh \
		generate --enable-post-process-file -i ./$(1) -g go -o ./$(2) --package-name $(3)
	cd $(2) && rm -rf go.mod go.sum .gitignore .openapi-generator-ignore .travis.yml git_push.sh README.md api docs test .openapi-generator
endef

.PHONY: refresh-3f-client
refresh-3f-client: ## Generate the 3F API client (openapi-generator, Go) from the vendored spec
	@rm -f api/threef/*.go
	$(call gen_openapi_client,openapi/3f-bf.openapi.json,api/threef,threef)

.PHONY: refresh-rfq-client
refresh-rfq-client: ## Generate the RFQ backend client (openapi-generator, Go) from the vendored spec
	@rm -f api/rfqbackend/*.go
	$(call gen_openapi_client,openapi/rfq-backend.openapi.json,api/rfqbackend,rfqbackend)

.PHONY: openapi-client
openapi-client: refresh-3f-client refresh-rfq-client ## Generate both OpenAPI clients

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
