# vault-solver — developer & CI tasks.
#
# Generated code (api/bindings, api/threef) is committed for a hermetic build; the refresh-*
# targets regenerate it from upstream on demand.

SHELL := bash
.DEFAULT_GOAL := help

# Pinned codegen tool versions.
ABIGEN_VERSION           ?= v1.16.1
GOLANGCI_LINT_VERSION    ?= v2.11.4
GENQLIENT_VERSION        ?= v0.8.1
GQLFETCH_VERSION         ?= v0.7.0
GENQLIENT_X_TOOLS_VERSION ?= v0.38.0
# Java openapi-generator (downloaded on demand by hack/openapi-generator-cli.sh). 7.12.0 is the floor:
# it ingests OpenAPI 3.1 (the RFQ backend spec); 5.4.0/7.0.1 fail on it.
OPENAPI_GENERATOR_VERSION ?= 7.12.0
OPENAPI_GENERATOR_SHA256 ?= 33e7dfa7a1f04d58405ee12ae19e2c6fc2a91497cf2e56fa68f1875a95cbf220

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
# LI.FI order-server OpenAPI spec (testnet/dev). The NestJS app serves a Scalar UI at /docs with NO raw
# JSON endpoint — the spec is embedded inline in the page, so refresh-lifi-openapi pulls the HTML and
# extracts it via hack/scalar-openapi-extract.py (see that target).
LIFI_OPENAPI_URL ?= https://order-dev.li.fi/docs
MORPHO_GRAPHQL_URL ?= https://api.morpho.org/graphql

# Contracts whose ABIs are vendored via refresh-abi. ABIS come from the rfq Foundry build; the
# CORE_MIRROR_ABIS (the 3F ThreeFAdapter, LiquidLane adapter, universal delegator, vault/ERC4626
# interfaces) come from the core-mirror build, since they aren't in rfq/out.
ABIS := IRequest IVaultController IWhitelist Executor Reactor
CORE_MIRROR_ABIS := ThreeFAdapter LiquidLaneAdapter IVaultV2 IERC4626
# api/abi/UniversalDelegator.json is hand-vendored to a minimal {limitOf} ABI (the full contract has
# an overloaded deallocateAll that abigen rejects, and the solver only reads limitOf) — like Multicall3.

# Contract:relpath mapping for Go bindings. Each contract gets its own package (the leaf dir) so
# shared ABI structs (e.g. the `Offer` tuple in both the adapter and IRequest) don't collide.
# Integration-specific bindings are grouped per integration (3f/, rfq/, oev/); contracts SHARED by more
# than one integration get a neutral group (e.g. the LiquidLane adapter under liquidlane/, used by both
# rfq and redstone-oev) so no integration owns another's surface; shared infra (vaultv2, multicall3)
# stays top-level.
#
# BINDINGS_V2 uses abigen --v2 (typed PackXxx/UnpackXxx/UnpackXxxEvent), so an ABI change breaks the build
# at the call site, not at runtime.
BINDINGS_V2 := ThreeFAdapter:3f/adapter IRequest:3f/request \
            IVaultController:3f/vaultcontroller IWhitelist:3f/whitelist \
            LiquidLaneAdapter:liquidlane/adapter Executor:rfq/executor Reactor:rfq/reactor \
            UniversalDelegator:delegator IVaultV2:vaultv2 IERC4626:erc4626 \
            SymbioticOevSolver:oev/callback RedStoneExecutor:oev/executor Morpho:oev/morpho \
            AdaptiveCurveIrm:oev/irm MorphoOracle:oev/oracle \
            AggregatorV3:oev/aggregator \
            ERC20:erc20 Multicall3:multicall3
# The OEV contracts (Morpho + its AdaptiveCurve IRM + market oracle, RedStone
# Executor, SymbioticOevSolver) plus a minimal ERC20 (decimals() only) aren't in our Foundry build, so their
# ABIs are hand-vendored under api/abi/ (not in ABIS/CORE_MIRROR_ABIS/refresh-abi). RedStoneExecutor avoids
# the rfq Executor name clash; solver ERC-20 reads (asset/balanceOf) reuse erc4626, the generic
# chain.Decimals reader uses erc20.
# Multicall3 is v2 like everything else — api/abi/Multicall3.json is hand-vendored (not a Foundry contract),
# so it's in BINDINGS_V2 but not ABIS. The chain.Multicall transport packs/unpacks aggregate3 and does its
# own eth_call.

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
	@echo "Morpho GraphQL uses gqlfetch + genqlient through go run in the make targets."

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

.PHONY: refresh-lifi-openapi
refresh-lifi-openapi: ## Re-pull the LI.FI order-server OpenAPI spec (LIFI_OPENAPI_URL=...)
	@mkdir -p openapi
	curl -fsSL "$(LIFI_OPENAPI_URL)" | python3 hack/scalar-openapi-extract.py > openapi/lifi-order.openapi.json
	@echo "vendored openapi/lifi-order.openapi.json (extracted from the Scalar /docs page)"

.PHONY: refresh-morpho-graphql-schema
refresh-morpho-graphql-schema: ## Re-pull the live Morpho GraphQL schema SDL (MORPHO_GRAPHQL_URL=...)
	@mkdir -p api/graphql/morpho
	go run github.com/suessflorian/gqlfetch/gqlfetch@$(GQLFETCH_VERSION) \
		-endpoint "$(MORPHO_GRAPHQL_URL)" > api/graphql/morpho/schema.graphql
	@echo "vendored api/graphql/morpho/schema.graphql"

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

# All three OpenAPI clients are generated with the Java openapi-generator (via hack/openapi-generator-cli.sh,
# which downloads the pinned jar on demand — needs a JRE). It is the only generator that ingests the RFQ
# backend's OpenAPI 3.1 spec; we use it for the 3F (3.0) and LI.FI order-server specs too for one toolchain.
# $(OPENAPI_GENERATOR_VERSION) is the floor — 5.4.0/7.0.1 fail on the 3.1 spec. The generated package is
# stdlib-only (no go.mod change); the recipes strip the generator's non-package cruft, keeping just the Go
# client. $(4) is optional extra generator flags — used only by the LI.FI recipe to pass
# --skip-validate-spec (its spec is labelled OpenAPI 3.0.0 but uses 3.1 JSON-Schema constructs — prefixItems /
# propertyNames — and has dangling oneOf $refs; the generator handles them fine but its strict validator
# rejects them). 3f/rfq keep validation on.
define gen_openapi_client
	GO_POST_PROCESS_FILE='gofmt -w' OPENAPI_GENERATOR_VERSION=$(OPENAPI_GENERATOR_VERSION) OPENAPI_GENERATOR_SHA256=$(OPENAPI_GENERATOR_SHA256) bash ./hack/openapi-generator-cli.sh \
		generate --enable-post-process-file $(4) -i ./$(1) -g go -o ./$(2) --package-name $(3)
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

.PHONY: refresh-lifi-client
refresh-lifi-client: ## Generate the LI.FI order-server client (openapi-generator, Go) from the vendored spec
	@rm -f api/lifiorder/*.go
	@# The raw vendored spec has two upstream defects that make the generated Go uncompilable (dangling
	@# oneOf $refs in QuoteDto.order; multi-tag operations that duplicate request structs). We keep the
	@# vendored file raw (contract of record) and generate from a normalized temp copy produced by
	@# hack/lifi-openapi-normalize.py (see that script for the exact, documented fixes). Inlined rather than
	@# using gen_openapi_client so the normalization + temp-file plumbing lives in one shell block;
	@# --skip-validate-spec is still needed (the spec is labelled 3.0.0 but uses 3.1 JSON-Schema constructs).
	tmp="$$(mktemp -p . --suffix=.lifi-normalized.json)"; \
		trap 'rm -f "$$tmp"' EXIT; \
		python3 hack/lifi-openapi-normalize.py < openapi/lifi-order.openapi.json > "$$tmp"; \
		GO_POST_PROCESS_FILE='gofmt -w' OPENAPI_GENERATOR_VERSION=$(OPENAPI_GENERATOR_VERSION) OPENAPI_GENERATOR_SHA256=$(OPENAPI_GENERATOR_SHA256) bash ./hack/openapi-generator-cli.sh \
			generate --enable-post-process-file --skip-validate-spec -i "$$tmp" -g go -o ./api/lifiorder --package-name lifiorder
	cd api/lifiorder && rm -rf go.mod go.sum .gitignore .openapi-generator-ignore .travis.yml git_push.sh README.md api docs test .openapi-generator

.PHONY: refresh-morpho-graphql-client
refresh-morpho-graphql-client: ## Generate the Morpho GraphQL client (genqlient) from the vendored schema + operations
	@mkdir -p api/morphographql
	@tmp="$$(mktemp -d)"; \
		trap 'rm -rf "$$tmp"' EXIT; \
		cd "$$tmp"; \
		go mod init genqlient-runner >/dev/null 2>&1; \
		go get github.com/Khan/genqlient@$(GENQLIENT_VERSION) golang.org/x/tools@$(GENQLIENT_X_TOOLS_VERSION) >/dev/null 2>&1; \
		go run github.com/Khan/genqlient "$(CURDIR)/api/graphql/morpho/genqlient.yaml"
	@gofmt -w api/morphographql/generated.go

.PHONY: openapi-client
openapi-client: refresh-3f-client refresh-rfq-client refresh-lifi-client ## Generate all OpenAPI clients

.PHONY: graphql-client
graphql-client: refresh-morpho-graphql-client ## Generate GraphQL clients

.PHONY: generate
generate: bindings openapi-client graphql-client ## Regenerate all committed codegen

.PHONY: check-generated
check-generated: generate ## Regenerate committed code and fail on drift
	@git diff --exit-code -- api/bindings api/threef api/rfqbackend api/lifiorder api/morphographql api/graphql/morpho/operations.json
	@untracked="$$(git ls-files --others --exclude-standard -- api/bindings api/threef api/rfqbackend api/lifiorder api/morphographql api/graphql/morpho/operations.json)"; \
		test -z "$$untracked" || { echo "untracked generated files:"; echo "$$untracked"; exit 1; }

.PHONY: build
build: ## Build the binary
	@mkdir -p bin
	go build -ldflags "$(LDFLAGS)" -o $(BIN) ./cmd/vault-solver

.PHONY: test
test: ## Run tests with race detector + coverage (hermetic only; fork/live suites are tag-gated out)
	go test -race -cover ./...

# Local-only OEV integration suite — build-tagged, skipped by the default `test` + CI.
.PHONY: test-oev-live
test-oev-live: ## OEV live checks — Morpho API discovery plus optional Sepolia fork payload dump
	go test -tags live -run TestLive -v ./internal/solvers/redstoneoev/...

.PHONY: format
format: ## Run golangci-lint with autofix
	golangci-lint run --fix

.PHONY: lint
lint: ## Run golangci-lint (no autofix; must report 0 issues)
	golangci-lint run

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
