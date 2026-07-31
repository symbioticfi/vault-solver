# Vault Solver

A Go service that monitors a configured selection of [Symbiotic](https://symbiotic.fi) vaults and
runs a pluggable **solver** against them. A solver is a self-contained integration with an external
protocol that sources, prices, or routes liquidity on top of a Symbiotic vault adapter; the bot
handles discovery, pricing/signing, on-chain reads, reconciliation, and settlement for it.

The framework is **solver-agnostic**: each integration lives in its own package, registers itself,
and is selected by config — adding one never touches the generic engine. The available integrations
are listed under [Solvers](#solvers).

> **Status:** early build. Engineering guidelines: [`CLAUDE.md`](./CLAUDE.md). Per-solver scope,
> architecture, and roadmap live under [`docs/`](docs).

## Architecture at a glance

- **`cmd/vault-solver`** — process bootstrap: flags, logging, signal-driven shutdown.
- **`internal/solver`** — generic `Solver` interface, registry, and engine.
- **`internal/solvers/<name>/`** — one self-contained package per integration; all protocol-specific
  logic lives here.
- **`internal/{config,chain,signer,txmanager}`** — solver-agnostic infra: two-stage config, vault /
  Multicall3 reads, a pluggable signer, and a nonce-serialized transaction broadcaster with independent
  receipt waits, shared across solvers.
- **`api/`** — committed codegen: contract `bindings/` (abigen) and protocol API clients, each
  refreshable from upstream.

State is intentionally minimal — positions, liquidity, and readiness are read from on-chain views and
the relevant protocol API on each tick; no database.

## Solvers

Solvers are listed in config under `solvers:` — one or more, **at most one entry per solver type**.
Every solver in the process shares the chain client, signer, and the single nonce-serialized
`txManager`, so multiple solvers on one EOA never race on nonces. Each entry's `config` block is typed
and validated by its own solver. Adding a solver touches **no** framework code — see the recipe in
[`CLAUDE.md`](./CLAUDE.md).

| `solver.name` | Integration | Docs | Example config |
|---|---|---|---|
| `3f-bridge-facilitator` | 3F (Grunt) bridge-loan auctions | [plan](docs/3F-PLAN.md) | [yaml](config/3f.example.yaml) |
| `rfq-filler` | Symbiotic RFQ quoting + order filling | [plan](docs/RFQ-PLAN.md) | [yaml](config/rfq.example.yaml) |
| `redstone-oev` | RedStone OEV liquidations | [plan](docs/OEV-PLAN.md) | [yaml](config/redstone-oev.example.yaml) |
| `lifi-samechain` | LI.FI same-chain intents over LiquidLane | [plan](docs/LIFI-PLAN.md) | [yaml](config/lifi.example.yaml) |
| `uniswapx-filler` | UniswapX V2 RFQ quoting and LiquidLane filling | [plan](docs/UNISWAPX-PLAN.md) | [yaml](config/uniswapx.example.yaml) |

All solvers expose a pluggable
**strategy** — the built-in `default` or an external `webhook` you run; see
[Strategies](#strategies).

### 3F Bridge Facilitator — `3f-bridge-facilitator`

Acts as a Bridge Facilitator in **[3F (Grunt)](https://3f.xyz)**'s bridge-loan auctions, on top of one
or more Symbiotic `BridgeFacilitatorAdapter`s. 3F auctions the right to front a bridge loan; this solver bids on behalf
of its adapters, funds the loans it wins just-in-time, and permissionlessly redeems repaid loans back
to the vault with yield.

It holds no API key: each adapter is registered with 3F by its vault creator, who authorizes this
solver's signer as the adapter's offer signer — directly (an EOA) or via an EIP-1271 contract signer —
so offers are authorized by signature alone. Design, config,
and roadmap: [`docs/3F-PLAN.md`](docs/3F-PLAN.md). When `adapters` is present, the solver operates only
on that explicit list. Otherwise it discovers all entries of the configured on-chain `IAdapterFactory`,
refreshing before each auction-discovery pass with a hard 2,000-entity safety limit; a larger reported
count is an error. Either source is filtered to non-zero vault/asset targets that authorize this
solver's signer (validated via the adapter's ERC-1271 `isValidSignature`). An empty factory is valid and
is polled until eligible adapters appear. Example:
[`config/3f.example.yaml`](config/3f.example.yaml).

### RFQ Filler — `rfq-filler`

An externally-owned solver/executor for **[Symbiotic RFQ](https://symbiotic.fi)**, on top of per-vault
`LiquidLaneAdapter`s. It runs a `POST /quote` server that prices swaps for the RFQ backend and a poller
that fills the orders it is awarded, settling on-chain through the adapter.

It runs either in `external` mode (the open-source filler; quoting and filling scoped to the operator's
own adapters) or `internal` mode (Symbiotic-internal; adds the private discounts flow). The caller EOA
must be an authorized caller of the RFQ `Executor` (its `setCallers` allowlist, granted by the owner).
External mode also fails startup unless that executor has direct `owner`/`marketMaker`/`isFiller`
authorization on every configured adapter; the fatal startup log includes the executor, configured adapters,
and underlying authorization error.
When `tokensToQuote: permissioned`, admitted inputs are never aggregated: the selected strategy must
use one candidate route. Other scopes keep the existing multi-route behavior.
`minAmountsIn` adds an optional per-input-token floor on request size (base units, decimal strings):
a request below its token's minimum is not quoted (HTTP 204), while an amount equal to the minimum
still quotes; unlisted tokens have no floor.
When an exact-input request exceeds the advertised adapter capacity, the default strategy caps the
quoted output at the available `maxAssets` instead of declining in every token scope; the excess input
is reflected as worse execution price and price impact. Awarded orders are planned again from current
LiquidLane state at fill time; the solver does not retain quote-time route plans.
Design, config, and roadmap:
[`docs/RFQ-PLAN.md`](docs/RFQ-PLAN.md) · example
[`config/rfq.example.yaml`](config/rfq.example.yaml).

### RedStone OEV — `redstone-oev`

An off-chain bidder for **[RedStone Atom OEV](https://docs.redstone.finance/docs/oev)** auctions. When a
price update makes a **[Morpho Blue](https://morpho.org)** position liquidatable, RedStone runs a
sub-second WebSocket auction for the right to be the liquidator; this solver bids, and on winning, its
signed payload is bundled atomically with the price update and the liquidation.

On settlement it liquidates the position and exits the seized collateral through a single Symbiotic
`LiquidLaneAdapter`, realizing the spread and paying its bid. It signs and bids but never submits the
settlement transaction — RedStone's auctioneer does. The solver config owns the RedStone Executor,
LiquidLane adapter, and callback address; the selected strategy owns the callback-specific
`operationData`. Operators can set `maxBidWei` as a per-auction spend ceiling over any strategy; it is
required for the external `webhook` strategy and optional for the built-in `default`. Design, config,
and roadmap:
[`docs/OEV-PLAN.md`](docs/OEV-PLAN.md) · example
[`config/redstone-oev.example.yaml`](config/redstone-oev.example.yaml).

### LI.FI Same-Chain Intents — `lifi-samechain`

A same-chain LI.FI Intents solver for LiquidLane-backed RWA → underlying routes. It publishes gas-aware
standing quotes from current adapter liquidity and receives matched, already-opened escrow orders over the
LI.FI WebSocket feed. Before each fill it rechecks the canonical order status, adapter state, gas cost, and
strategy decision, then atomically claims the input, redeems it through LiquidLane, and fills the output via
`LiquidLaneLifiExecutor`. Capacity reserved by already-submitted fills is deducted from both later fill
decisions and standing quotes until those transactions complete. Each token pair advertises the full currently
available capacity even when several pairs share one vault; accepting a fill reserves its shared `CapacityID`
and immediately refreshes every affected quote. A fill remains pending until the shared tx manager reaches
the configured confirmation depth; only then is its reservation released and quote refresh requested.
The published quote ladder is not replayed at fill time: the
solver greedily rebuilds the best current route plan, and redeemed output above the order requirement remains
executor surplus. The default strategy trims an uneconomic range prefix to the first input whose conservative
floor yields a positive output, then prices the published suffix by running the shared LiquidLane exact-input
quote solver at both endpoints. It caps the lower of the two endpoint rates by that floor for interior route
transitions, worst-case route gas, and rounding.
`strategy.config.rangeCount` sets the geometric curve resolution (default `8`, maximum `16`).

The executor contract is the registered LI.FI solver account. It is registered once through EIP-1271 using
a caller signature bound to the executor's EIP-712 domain, appears as `exclusiveFor` in quotes, and calls the
settler's direct finalise path. The framework signer is an authorized executor caller and transaction sender;
fills do not carry a per-order `AllowOpen` signature.
The owner manages callers, while ERC-1271 validates domain-separated registration signatures against the
current callers.

Our deployment convention is one LI.FI API key per registered executor contract. LI.FI can register
multiple accounts under one key, but this deployment deliberately does not share a key across executors.
All processes using one executor therefore share its API key and LI.FI reputation; active/active operation
also requires external order coordination. The API key, executor owner key, and caller transaction key are
distinct credentials.

Only on-chain escrow orders are supported; gasless Compact, Permit2/3009, Dutch auctions, and future-order
scheduling are out of scope. Dutch (`0x01`) and exclusive Dutch (`0xe1`) orders are ignored at WebSocket
admission and logged as unsupported. `solverMode: external` serves direct filler-authorized adapters.
`solverMode: internal` also enables signed private discounts through the shared backend. `tokensToQuote` uses the same `all`,
`permissioned`, and `permissionless` scopes as RFQ; permissioned inputs must execute through one physical
route. The order-server REST/WS endpoints are explicit required config, and each Chainlink gas feed has
its own required max age. The default strategy evaluates bounded geometric exact-input ranges across
available capacity. See the plan for settlement, pricing, concurrency, and onboarding details:
[`docs/LIFI-PLAN.md`](docs/LIFI-PLAN.md) · example
[`config/lifi.example.yaml`](config/lifi.example.yaml).

The opened-order settler must report `governanceFee() == 0`. The solver checks this at startup and again for
every admitted order. Startup fails closed; at runtime an unreadable or non-zero fee skips the order with an
error log before planning or submission.

The implementation is ready for the opened-order path. The next live E2E requires deploying the current
executor build, registering it with LI.FI, and granting it filler authorization on the target adapter.

### UniswapX Quoter + Filler — `uniswapx-filler`

An Ethereum-mainnet UniswapX solver backed by LiquidLane routes. It serves the RFQ `POST /quote`
webhook, polls the Uniswap order API for exclusive and public V2 orders, resolves
their Dutch amounts from current chain time, and fills profitable orders through a configured
`LiquidLaneUniswapXExecutor`. The executor uses the same owner-managed caller list as the RFQ executor and
remains the Reactor-facing filler. Before serving traffic, the solver validates executor bytecode, finds the
tx-sending EOA in the executor's indexed `callers` list, and, in external mode, checks every configured
route's direct authorization. Failures log the relevant executor, caller, or adapters and the underlying
reason before startup returns. The executor ABI has no Reactor getter, so matching the configured Reactor to
the deployed immutable remains a deployment assertion. `solverMode: external` is the default, requires a
non-empty `adapters` list plus direct authorization, and forbids the discounts block. `solverMode: internal`
requires that block; direct routes are authorization-filtered from each snapshot while valid signed-discount
routes remain usable. In internal
mode `adapters` is optional: a non-empty list scopes quotes and direct fills, while fill-time signed-discount
recovery may use any adapter advertised by the backend. Without a list the solver quotes and fills
discount-only. Every fill is simulated again immediately before submission.

The quote path is stateless and uses a refreshed on-chain inventory snapshot so it stays within Uniswap's
response deadline. Each request is priced once for its concrete amount: the strategy returns one
`amountIn`/`amountOut` pair after price buffer and, when configured, estimated fill gas, with no precomputed
ladders, amount ranges, or quote-time route reservation. Omitting the entire `gas:` block disables gas
accounting in both quote and fill decisions and skips gas-state and Chainlink reads. The tx manager still
prices and pays actual transaction gas, so that cost is then subsidized by the solver. Uniswap deliberately
makes indicative and hard RFQ requests
indistinguishable, so the solver echoes `quoteId` but does not guess the phase. As soon as a polled order is
admitted to the fill queue, quote publication pauses until planning either rejects it or atomically hands
capacity ownership to an accepted transaction reservation. Every posted order gets a fresh route plan from
the current chain state and is simulated before sending. The reservation remains effective while txmanager waits
for the configured confirmations. On completion the quote snapshot is invalidated before capacity is
released, and that capacity is not advertised again until a fresh post-fill chain snapshot is published.
A quote is returned only if its snapshot epoch and every blocking condition are unchanged after the strategy
finishes. Quoting fails closed during startup warmup, stale or unknown exclusive-order delivery, fill
planning, an active Uniswap `blockUntilTimestamp`, or the configured local fade breaker. `GET /ready`
exposes that state and also returns not-ready when the latest snapshot has no quotable inventory;
`GET /health` and its probe-friendly alias `GET /healthz` remain liveness-only.

Every valid exclusive order assigned to the executor is tracked through `decayStartTime`. After that
deadline, tracked hashes are reconciled in batches against the order API and canonical transaction receipts.
A successful on-chain fill at or before the deadline clears the obligation, including another filler's soft
override. A fill by any filler only after the deadline—including our executor—or any known non-filled
terminal state for an obligation observed live or recovered after a runtime poll gap opens the separate
local fade breaker, matching Uniswap's
[fade definition](https://developers.uniswap.org/docs/liquidity/uniswapx/filling/faq#fade-mechanics).
An already-terminal miss found only by initial startup history reconciliation is logged and terminalized
without opening a fresh local breaker.
If terminal status or receipt time cannot be established, quoting stops without opening the breaker until
reconciliation succeeds.

In internal mode, advertised LiquidLane routes are resolved on-chain and checked against their advertised
asset and decimals, current physical capacity/rate, adapter minimum discount, token policy, and configured
gas feeds. Configured adapters scope quoting when present; fill-time discount recovery remains unrestricted,
matching RFQ solver-mode semantics. A selected discount is resolved again immediately before simulation and
encoded as a typed `discountSwap`; its adapter, token, output floor, signatures, and expiry window are
checked fail-closed.

The order API key is required and read indirectly through `orderServer.apiKeyEnv`. Uniswap's public quote
contract specifies source-IP allowlisting rather than an application header, so restrict the quote endpoint
to the published Beta/production source IPs at the ingress. The order API URL must use HTTPS except for
loopback development servers. Each V2 order carries its swapper-authorized cosigner; the solver verifies its
cosignature directly, so there is no static cosigner setting to rotate. Exclusive V2 polling is mandatory
while the quote server is enabled; public V2 filling remains independently opt-in. Legacy V1 limit orders
are not supported. The generated order client follows upstream order-service spec version 2.0.0 and decodes
the current `DutchV2OrderEntity`, including nested `cosignerData`, `cosignature`, and `createdAt`.
Native-asset outputs are currently declined because the supported LiquidLane routes settle ERC-20 vault
assets.
Exact-input and exact-output Dutch auctions are supported. Exact-output quotes directly size enough input
for the requested output, buffer, and gas; rounding or execution output above that requirement remains
executor surplus. If a Dutch exact-output input grows between planning and execution, the executor consumes
the planned route input and retains the positive input difference as filler surplus. The Reactor atomically
enforces the order's aggregate outputs. Multiple outputs are supported when every output uses the same
ERC-20; mixed-token outputs fail closed because one
LiquidLane route produces one vault asset. Quote webhook protocols `v1` and `v2` are accepted, while V3
orders and secondary-DEX routes are not supported. Design,
config, onboarding, and deployment prerequisites:
[`docs/UNISWAPX-PLAN.md`](docs/UNISWAPX-PLAN.md) · example
[`config/uniswapx.example.yaml`](config/uniswapx.example.yaml).

### Strategies

The solvers split protocol plumbing (reads, signing, submission — fixed) from the
**decision** — how to size, price, and select — which is a pluggable *strategy*, chosen in config:

- **`default`** — the built-in in-process strategy for that solver.
- **`webhook`** — delegates each decision to an **external HTTP service you run**: the solver sends it
  the raw facts as JSON and executes the validated plan it returns, so your service owns the logic.
  LI.FI and UniswapX own separate strategy contracts and independently reject returned fills that exceed
  current capacity or do not cover the order plus gas. UniswapX delegates each concrete quote to
  `POST /decide-quote` and each current fill plan to `POST /decide-fill` under the configured webhook URL.

This is the seam for customizing a solver without forking. Contract and trust model:
[`docs/strategy-plan.md`](docs/strategy-plan.md).

The shared `txManager` fee-bumps pending transactions on `replacementIntervalMs`. After
`pendingTimeoutMs`, it cancels only the lowest unresolved nonce before allowing later queued nonces
to proceed. The required `maxFeeGwei` is the absolute ceiling; normal sends reserve one fee bump
inside that ceiling so cancellation still has headroom.

## Requirements

- Go (toolchain version pinned in [`go.mod`](./go.mod); auto-fetched by recent Go releases).
- For regenerating codegen: `make tools` (installs pinned `abigen`, `golangci-lint`). OpenAPI clients use
  the Java openapi-generator, downloaded on demand by `hack/openapi-generator-cli.sh` (needs a JRE).
- A reachable EVM RPC endpoint and a signing key (see Configuration).

## Quickstart

```bash
make build            # build ./bin/vault-solver
./bin/vault-solver version
make test             # go test -race -cover ./...
make test-txmanager-anvil # real pending replacement/cancellation against local Anvil
make lint             # golangci-lint
./bin/vault-solver run --config config/3f.example.yaml
```

The CLI is built with [Cobra](https://github.com/spf13/cobra); run `vault-solver --help` for the
command list (`run`, `version`). Debug logging is off by default; enable it with
`observability.debug: true` in config or the `--debug` flag (the flag wins):

```bash
./bin/vault-solver run --config config/3f.example.yaml --debug
```

## Observability

The observability listener (default `:9090`) serves `/metrics`, `/healthz`, and `/readyz`. No extra
config is required for the collectors below.

### Metrics

The registry also includes standard Go/process collectors and
`solver_bot_build_info{version,commit}`. The latter identifies the exact binary behind a sample.

| Scope | Metric family | Labels | What it shows and why it is useful |
|---|---|---|---|
| Txmanager | `solver_bot_txmanager_requests_total` | `label`, `outcome` | Terminal results of logical on-chain operations. This is the primary confirmed/reverted/submission-failure funnel for every solver. |
| Txmanager | `solver_bot_txmanager_inflight` | `label` | Requests accepted by the txmanager worker and still awaiting a terminal result; sustained values expose stuck transactions or nonce congestion. |
| Txmanager | `solver_bot_txmanager_gas_used_total` | `label`, `outcome` | Receipt gas for mined transactions, including reverts. Divide by the matching request count for average gas; this is gas units, not native-token cost. |
| Txmanager | `solver_bot_txmanager_replacements_total` | `label`, `kind` | Successfully broadcast replacements and cancellations. Spikes expose fee-policy or congestion problems that terminal outcomes alone cannot show. |
| LiquidLane | `liquidlane_fill_amount_atomic_units_total` | `solver`, `token`, `kind` | Order input, required output, and positive planning-time gross surplus for successful receipts. `kind` is `input`, `output`, or `planned_surplus`; values are monitoring counters in token atomic units, not realized PnL. |
| RFQ | `rfq_filler_http_request_duration_seconds` | `method`, `route`, `status` | Quote-server request count (`_count`), status funnel, and latency. Routes are allowlisted and methods are normalized to `GET`, `POST`, or `other` to bound cardinality. |
| RFQ | `rfq_wins_total` | — | Orders first observed assigned to this filler. A win is counted even if planning or transaction submission later fails. |
| RFQ | `rfq_active_orders` | — | Current queued, submitting, or submitted obligations awaiting terminal backend state. |
| RFQ | `rfq_oldest_active_order_age_seconds` | — | Age of the oldest active obligation; catches a single stuck order that a count-only alert can miss. |
| RFQ | `rfq_last_successful_order_poll_timestamp` | — | Unix timestamp of the last complete successful backend open-order poll, including an authoritative empty result; failed polls leave it unchanged. |
| LI.FI | `lifi_active_quotes` | — | Process-local quote count from the last successful reconciliation. It can remain nonzero after the remote quotes expire at `quoteTtl`, so use it with refresh freshness rather than as backend state. |
| LI.FI | `lifi_last_successful_refresh_timestamp` | — | Freshness of standing-quote reconciliation; distinguishes an authoritative zero from a dead refresh loop. |
| LI.FI | `lifi_order_feed_connected` | — | `1` only while the order-feed loop owns an established WebSocket; `0` while disconnected, dialing, or backing off. |
| UniswapX | `uniswapx_quote_requests_total` | `outcome` | Quote webhook decisions: `invalid`, `breaker-notification`, `error`, `declined`, or `quoted`. `quoted` means a response was selected for writing, not that a user signed it. |
| UniswapX | `uniswapx_quote_duration_seconds` | — | End-to-end quote-handler latency across all request outcomes. |
| UniswapX | `uniswapx_order_polls_total` | `source`, `outcome` | Successful and failed `exclusive-v2`/`public-v2` poll cycles. An exclusive success includes recovery and obligation reconciliation. |
| UniswapX | `uniswapx_exclusive_wins_total` | — | Exclusive delivery obligations first observed by live polling. Startup and runtime recovery restore safety state without replaying this counter; only startup-only terminal misses avoid the breaker. |
| UniswapX | `uniswapx_exclusive_obligations_outstanding` | — | Live-observed or recovered obligations still awaiting terminal classification. |
| UniswapX | `uniswapx_exclusive_nearest_deadline_timestamp` | — | Nearest outstanding exclusivity deadline; alerts on urgent or stuck obligations. |
| UniswapX | `uniswapx_exclusive_obligation_outcomes_total` | `outcome` | Live-observed obligations classified as `settled_in_time` or `missed`. Timely delivery may be by another filler; this is a delivery SLO, not a self-fill counter. |
| UniswapX | `uniswapx_block_until_timestamp` | — | Maximum deadline among remote, local-fill, exclusive-fade, and startup-warmup time-based quote blockers. |
| UniswapX | `uniswapx_ready` | — | Scrape-time availability: `1` only when current quote state, breakers, and exclusive delivery permit quoting. |
| UniswapX | `uniswapx_last_quote_refresh_timestamp` | — | Last atomic quote-state publication. A successful publication may contain no inventory, so freshness alone is not readiness. |
| UniswapX | `uniswapx_last_exclusive_poll_timestamp` | — | Last successful exclusive poll plus recovery/obligation reconciliation. |
| UniswapX | `uniswapx_pending_fills` | — | Admitted fills holding LiquidLane capacity while awaiting a txmanager terminal result. |
| OEV | `oev_auction_decisions_total` | `strategy`, `outcome` | One bounded terminal decision per parsed auction, including `enqueued`, `would_bid`, and skip reasons. Summing outcomes gives processed auction traffic and each outcome explains why no bid was enqueued. |
| OEV | `oev_bid_wei_total` | `strategy`, `stage` | Bid value at `enqueued`, `won`, `settled_success`, and `settled_failed` stages. It provides an amount-weighted funnel, not realized profit or proof of server receipt. |
| OEV | `oev_wins_total` | `strategy` | Wins matched to an active local bid reservation. |
| OEV | `oev_breaker_failures_total` | `strategy` | Failed liquidation frames for our callback recorded by the rolling breaker, including failures received after local reservation reconciliation. Replays are deduplicated when the frame has an ID or transaction hash. |
| OEV | `oev_settlements_total` | `strategy`, `result` | Successful/failed settlement frames matched while the local bid reservation is active. |
| OEV | `oev_won_inflight` | `strategy` | Locally observed winning bids still awaiting settlement. |
| OEV | `oev_unresolved_wins_total` | `strategy` | Winning reservations removed after timeout without settlement or nonce proof; this is the missed-obligation signal. |
| OEV | `oev_hotpath_seconds` | `strategy` | End-to-end handling latency for parsed auction frames against the auction's short decision budget. |
| OEV | `oev_deposit_wei` | `strategy` | Executor deposit from the last complete state refresh; use it to monitor settlement runway. |
| 3F | `threef_offer_submissions_total` | `result` | `createOffer` API successes and errors after local validation; these are submissions, not wins. |
| 3F | `threef_offer_amount_atomic_units_total` | `token`, `kind` | Principal and quoted expected yield in API-successful offers. `kind` is `principal` or `expected_yield`; neither is a realized settlement amount. |
| 3F | `threef_observed_items` | `view` | Last complete count for `offers`, `active_requests`, or `redeemable`. The views show offer coverage, outstanding exposure, and redeem backlog. |
| 3F | `threef_last_successful_observation_timestamp` | `view` | Independent freshness for each 3F view, so retained last-known-good counts cannot silently look current. |

Txmanager `label` values are stable operation names (`redeem`, `rfq-fill`, `lifi-fill`,
`uniswapx-fill`). Terminal outcomes are `confirmed`, `included_unconfirmed`, `reverted`, `cancelled`,
`submission_error`, and `tracking_stopped`. LiquidLane counters include successful receipts reported as
`included_unconfirmed`; they are operational telemetry rather than an accounting ledger, and amounts for
different token labels must not be added without price/decimal normalization.

Migration from removed metrics:

| Removed | How to migrate |
|---|---|
| `rfq_filler_http_requests_total` | `rfq_filler_http_request_duration_seconds_count` with the same labels |
| `uniswapx_fills_total{outcome="filled"}` | `solver_bot_txmanager_requests_total{label="uniswapx-fill",outcome=~"confirmed\|included_unconfirmed"}` |
| `uniswapx_fills_total{outcome="failed"}` | `solver_bot_txmanager_requests_total{label="uniswapx-fill",outcome=~"reverted\|cancelled\|submission_error\|tracking_stopped"}` |
| `oev_auctions_total`, `oev_bids_total`, `oev_skips_total` | Query explicit `oev_auction_decisions_total{outcome}` values. Their old semantics are intentionally consolidated; the new total also classifies `feed_ignored`. |
| `oev_bid_wei_total{stage="submitted"}` | `oev_bid_wei_total{stage="enqueued"}` |
| `oev_failed_liquidations_total` | `oev_breaker_failures_total` |
| `oev_deposit_below_floor` | `oev_deposit_wei < bool 10000000000000` |
| `threef_live_offers`, `threef_active_requests`, `threef_redeemable_requests` | `threef_observed_items{view=~"offers\|active_requests\|redeemable"}` or the corresponding exact `view` |

## Configuration

Config is YAML with a two-stage decode: the framework reads `solver.name` to select the
implementation and hands the opaque `solver.config` block to that solver to type. Each solver has its
own fully annotated example under `config/` (see the *Example config* column above) — every field,
including the shared `chain`/`signer`/`txManager`/`observability` blocks, is documented inline there.
The `chain` block takes a primary `rpcUrl` plus optional `rpcFallbackUrls` — HTTP(S) endpoints tried
in order when the primary is unavailable. LiquidLane state reads always use RPC `latest`; an archive
node is not required. **Never commit a real key or live config** — keys are
supplied via env/file behind the `Signer` interface; `*.local.*` and `.env` are gitignored.

## Code generation

Generated code is committed for hermetic builds; refresh from upstream on demand:

```bash
make refresh-abi FORGE_OUT=../rfq/out   # re-vendor contract ABIs from a Foundry build
make refresh-openapi                    # re-pull the live 3F OpenAPI spec
make refresh-rfq-openapi                # re-pull the RFQ backend OpenAPI spec
make generate                           # regenerate bindings + API client
```

## Contributing

Engineering conventions — the modular framework/integration boundary, config-driven configuration,
modern Go 1.26 style, the required test/lint/format gate, and secure-coding rules — are in
[`CLAUDE.md`](./CLAUDE.md) (`AGENTS.md` is a symlink to it). Every change must keep
`make format && make test && make lint` green and unit-test new logic.
