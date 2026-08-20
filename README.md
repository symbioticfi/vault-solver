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
  Multicall3 reads, a pluggable signer, and a nonce-serialized transaction broadcaster that shares
  one unresolved signed lifecycle across solvers.
- **`api/`** — committed codegen: contract `bindings/` (abigen) and protocol API clients, each
  refreshable from upstream.

State is intentionally minimal — positions, liquidity, and readiness are read from on-chain views and
the relevant protocol API on each tick; no database.

## Solvers

Solvers are listed in config under `solvers:` — one or more, **at most one entry per solver type**.
Every solver shares the chain client and signer. Transaction-sending solvers also share the single
nonce-serialized `txManager`, so multiple solvers on one EOA never race on nonces. Solvers whose
settlement is submitted externally do not start it. Each entry's `config` block is typed and validated
by its own solver. Adding a solver touches **no** framework code — see the recipe in
[`CLAUDE.md`](./CLAUDE.md).

Sharing is deliberately process-scoped. Deploy solvers that use a different signer, read-RPC set, or private
write endpoint as a separate process with its own config subset and txmanager. Assign each scrape target a
unique Prometheus `instance` (and optionally a stable `lane` target label). One EOA must never be configured
in two processes: independent txmanagers would race on its nonce even when their RPC URLs differ. Solvers
that share an EOA belong in one process so they retain one serialized nonce lane.

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
Pareto's mainnet `AA_FalconXUSDC` tranche (`0xC26A…f99C`) uses this existing generic path and needs no
token-specific solver code. The production deployment config already includes it in
`permissionedTokens` with a one-token `minAmountsIn` floor. A solver instance can route it only after
its configured LiquidLane adapter has onboarded the token. Execution also requires either direct
`owner`/`marketMaker`/`isFiller` authorization or a live signed discount in internal mode.
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
required for the external `webhook` strategy and optional for the built-in `default`. The common `gas:`
block is optional, and its shared oracle facts are passed to the selected strategy. The built-in strategy
uses them for after-cost economics; without them, it selects gross-profitable bundles while retaining the
signed gas-price cap and native funding checks. When `gas:` is configured, startup requires a feed for the
resolved adapter loan asset and a readable initial oracle snapshot. Design, config, and roadmap:
[`docs/OEV-PLAN.md`](docs/OEV-PLAN.md) · example
[`config/redstone-oev.example.yaml`](config/redstone-oev.example.yaml).

### LI.FI Same-Chain Intents — `lifi-samechain`

A same-chain LI.FI Intents solver for LiquidLane-backed RWA → underlying routes. It publishes standing quotes
from current adapter liquidity with optional gas accounting and receives matched, already-opened escrow orders over the
LI.FI WebSocket feed. On startup and reconnect it catches up active matches through `GET /orders` before
publishing quotes; while disconnected it suspends renewal and retries expiry of known curves. Before each fill it
rechecks the canonical order status, adapter state, configured gas cost, and strategy decision, then atomically claims
the input, redeems it through LiquidLane, and fills the output via
`LiquidLaneLifiExecutor`. Capacity reserved by already-submitted fills is deducted from both later fill
decisions and standing quotes until those transactions complete. Each token pair advertises the full currently
available capacity even when several pairs share one vault; accepting a fill reserves its shared `CapacityID`
and immediately refreshes every affected quote. The reservation remains until the shared tx manager returns a
terminal result. Receipted fills, reverts, and cancellations wait for the configured confirmation depth;
pre-sign or definitive broadcast failures end earlier and release the reservation without a receipt.
Orders
that the built-in strategy proves fillable without, but blocked by, pending reservations enter a bounded FIFO
without blocking later deliveries. The worker retries them after every reservation release and returns a still-
blocked order to the tail. During startup/reconnect recovery, quote publication remains suspended until each
recovered order leaves the FIFO, either resolved or returned to the recovery sweep. Overflow drops the newest
retry. A webhook `null` decision and an order-specific `400`/`422` fill rejection stay terminal; other
strategy failures get at most three attempts per order during each recovery session. On graceful
shutdown the solver keeps the feed alive while it expires active curves with the configured order-server HTTP
timeout, then stops accepting orders and waits for already-accepted fills until completion or the finite process
hard stop.
If a newly opened order reaches the feed before the RPC endpoint exposes its deposit, the worker retries the
status-`None` read with bounded exponential backoff capped at 5 seconds until the 30-second window or earlier
order deadline. The final scheduled read is clamped to 250 milliseconds before that boundary. Duplicate
deliveries are coalesced during the wait; claimed, refunded, and unknown statuses remain terminal. Stopping
intake drops these unaccepted retries immediately.
The published quote ladder is not replayed at fill time: the
solver greedily rebuilds the best current route plan, and redeemed output above the order requirement remains
executor surplus. The default strategy trims an uneconomic range prefix to the first input whose conservative
floor yields a positive output, then prices the published suffix by running the shared LiquidLane exact-input
quote solver at both endpoints. It caps the lower of the two endpoint rates by that floor for interior route
transitions, rounding, and, when configured, worst-case route gas.
`strategy.config.rangeCount` sets the geometric curve resolution (default `8`, maximum `16`).

Omitting LI.FI's `gas:` block disables gas accounting in quote/fill decisions and skips gas-state and
Chainlink reads; the tx manager still prices and pays the actual transaction gas.

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
scheduling are out of scope. Dutch (`0x01`) and exclusive Dutch (`0xe1`) orders are ignored at order-feed
admission and logged as unsupported. Fully valid feed orders routed to another origin or output chain are
expected noise and logged at info; malformed payloads and target-chain contract mismatches remain errors.
`solverMode: external` serves direct filler-authorized adapters.
`solverMode: internal` also enables signed private discounts through the shared backend. `tokensToQuote` uses the same `all`,
`permissioned`, and `permissionless` scopes as RFQ; permissioned inputs must execute through one physical
route. The order-server REST/WS endpoints are explicit required config. When `gas:` is configured, each
Chainlink feed has its own required max age. The default strategy evaluates bounded geometric exact-input ranges across
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
discount-only. Every fill is simulated again immediately before submission. The wall-clock anchor for
a fill is captured before reading chain time, so RPC and planning latency consume the order's remaining
validity instead of extending it.

The quote path is stateless and uses a refreshed on-chain inventory snapshot so it stays within Uniswap's
response deadline. Each request is priced once for its concrete amount: the strategy returns one
`amountIn`/`amountOut` pair after price buffer and, when configured, estimated fill gas, with no precomputed
ladders, amount ranges, or quote-time route reservation. Omitting the entire `gas:` block disables gas
accounting in both quote and fill decisions and skips gas-state and Chainlink reads. The tx manager still
prices and pays actual transaction gas, so that cost is then subsidized by the solver. Uniswap deliberately
makes indicative and hard RFQ requests
indistinguishable, so the solver echoes `quoteId` but does not guess the phase. As soon as a polled order is
admitted to the fill queue, quote publication and `GET /ready` pause. They remain paused during planning and,
once the submission occupies the shared nonce lane, while it holds that queued or admitted lifecycle,
including receipt confirmation. The fill's capacity reservation still protects already-awarded orders for
the same period; it does not reopen quoting. Every posted order gets a fresh route plan from the current chain
state and is simulated before sending. On completion the quote snapshot is invalidated before capacity is
released, and that capacity is not advertised again until a fresh post-fill chain snapshot is published.
A quote is returned only if its snapshot epoch and every blocking condition are unchanged after the strategy
finishes. Quoting fails closed during startup warmup, stale or unknown exclusive-order delivery, fill
planning, a queued or admitted txmanager lifecycle, an unavailable nonce lane, an active Uniswap
`blockUntilTimestamp`, or the configured local fade breaker. A claimed order is requeued before chain reads,
signed-discount resolution, calldata construction, or preflight while the nonce lane is paused. A txmanager
result rejected before the worker lifecycle does not count toward the local fill breaker and is reported as
`solver_bot_txmanager_admission_rejections_total{label="uniswapx-fill"}` rather than a terminal fill
failure. `GET /ready` exposes that state and
also returns not-ready when the latest snapshot has no quotable inventory;
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
  current capacity or do not cover the order plus configured gas. UniswapX delegates each concrete quote to
  `POST /decide-quote` and each current fill plan to `POST /decide-fill` under the configured webhook URL.

This is the seam for customizing a solver without forking. Contract and trust model:
[`docs/strategy-plan.md`](docs/strategy-plan.md).

When used, the shared `txManager` owns one unresolved signed nonce lifecycle at a time. Later
submissions are neither accepted nor signed until the active lifecycle has a terminal receipt. Every
`replacementIntervalMs` it attempts a replacement using fresh fees and at least a 12.5% bump over the
previous attempt; if fresh fees are unavailable, it bumps the cached fees. When a submission returns an
ambiguous transport error, the first replacement tick instead rebroadcasts those exact signed bytes once
without changing the hash or fees; a later tick may fee-bump it. Cancellation deadlines and shutdown bypass
that grace retry. At `pendingTimeoutMs` (or the request's earlier deadline), replacements switch to a
same-nonce cancellation. Each submission RPC is bounded independently by `broadcastTimeoutMs` (5 seconds
by default), so a short replacement cadence does not prematurely time out a private write RPC. Every
`accountPollIntervalMs` (30 seconds by default), an active txmanager refreshes the sender's native balance
plus latest and pending nonces through the write endpoint; a failed refresh retains the last complete snapshot.
Account identity and snapshot series are absent when no configured solver starts txmanager, and the values from
each successful refresh are exported as one scrape-consistent snapshot.

After lifecycle admission and immediately before signing, requests without an explicit gas limit run
`eth_estimateGas` against their exact sender, target, value, and calldata. The manager adds 5% headroom
to that estimate. Normal replacements reuse the admitted gas limit; same-nonce cancellations use 21,000.

The transaction lane is ready for new external commitments only while it has no queued or admitted
lifecycle and nonce ownership is certain. While the lane is occupied or conflicted, UniswapX and RFQ
decline new quotes, LI.FI retires its active standing curves, and 3F stops posting new offers.
Reconciliation and already-accepted work continue. A normal submission that races a nonce conflict waits
without signing until exact-hash reconciliation restores the lane, its request deadline expires, or shutdown
begins; non-blocking admission declines immediately. This lets the process recover without abandoning an
immutable order that has already been accepted from an upstream protocol.

During graceful shutdown the manager remains alive while solvers stop external commitments and drain
already-accepted work. The solver drain is bounded by its preparation timeout plus `pendingTimeoutMs`
and `replacementIntervalMs`. When manager shutdown begins, new admission stops and it requests
same-nonce cancellation when nonce ownership is not conflicted. It keeps draining exact signed attempts
for at most `shutdownTimeoutMs`; if no terminal receipt is available by then, callers receive a
shutdown-deadline error and the process exits instead of hanging indefinitely. Configure the
orchestrator's SIGTERM grace to cover the sum of those bounds.

The required `maxFeeGwei` is the global EIP-1559 fee cap, including cancellation. Normal transactions
stay one 12.5% bump below it so cancellation has headroom, and the initial send reserves another bump
inside its normal cap for a replacement. A solver-supplied request cap applies to the original call
and its replacements; cancellation may exceed that request cap but never `maxFeeGwei`. A positive
`tipGwei` is the only mandatory priority-fee floor. A higher node suggestion is advisory and is clamped
to the fee cap's available headroom instead of blocking an otherwise valid send. Startup rejects a
positive floor that leaves no base-fee headroom after both reserved bumps, and runtime submission fails
when the current base fee leaves insufficient room for that floor. With `tipGwei: 0` (or the field omitted),
txmanager instead uses the minimum gas-weighted p25 priority reward from the latest five blocks, matching
the observed behavior of Etherscan Gas Tracker's Fast tier, and likewise clamps it to available headroom.
Invalid or unavailable `eth_feeHistory` fails new submissions closed; setting a positive floor provides the
operator-controlled fallback.

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

The registry also includes standard Go/process collectors,
`solver_bot_build_info{version,commit}`, and `solver_bot_solver_info{solver}`. The first identifies the exact
binary behind a sample; the second exposes bounded config-time process membership so fleet dashboards can
map each scrape instance/execution lane to its solvers without inferring ownership from traffic.

| Scope | Metric family | Labels | What it shows and why it is useful |
|---|---|---|---|
| Framework | `solver_bot_service_ready` | — | `1` exactly when the shared `/readyz` gate admits work, otherwise `0`. This is process/nonce-lane readiness, not a claim that every solver upstream is healthy; combine it with solver freshness and connectivity. |
| Framework | `solver_bot_solver_info` | `solver` | Constant `1` for each solver configured in this process. Prometheus target labels such as `instance`/`lane` make process membership explicit without adding deployment-specific labels in application code. |
| Framework | `solver_bot_external_operation_duration_seconds` | `solver`, `strategy`, `operation`, `outcome` | Count and latency of allowlisted recurring solver operations such as polls and authoritative refreshes. Outcomes are bounded to `success`, `degraded`, `skipped`, or `error`; errors and request-derived values never become labels. |
| RPC | `solver_bot_rpc_requests_total` | `role`, `method`, `outcome` | Logical HTTP JSON-RPC calls. Roles are `read`, `write`, or `shared`; methods and outcomes are bounded, with transport, HTTP, rate-limit, decode, context, and JSON-RPC errors separated. |
| RPC | `solver_bot_rpc_attempts_total` | `role`, `endpoint`, `method`, `outcome` | Per-endpoint attempts, including failed primary and successful fallback attempts. `endpoint` is only a role-local ordinal (`0`, `1`, …); configured URLs and error text are never labels. |
| RPC | `solver_bot_rpc_inflight` | `role` | Calls whose response bodies have not completed; a sustained value exposes a hung endpoint or consumer. |
| RPC | `solver_bot_rpc_request_duration_seconds` | `role`, `method`, `outcome` | End-to-end HTTP JSON-RPC latency through response-body consumption. |
| RPC | `solver_bot_rpc_last_successful_request_timestamp` | `role` | Last successful logical call by endpoint role. |
| RPC | `solver_bot_rpc_last_successful_attempt_timestamp` | `role`, `endpoint` | Last successful endpoint attempt, so an idle or dead fallback can be distinguished from a healthy primary. |
| Txmanager | `solver_bot_txmanager_requests_total` | `label`, `outcome` | Terminal results of logical on-chain operations. This is the primary confirmed/reverted/submission-failure funnel for every solver. |
| Txmanager | `solver_bot_txmanager_inflight` | `label` | Requests accepted by the txmanager worker and still awaiting a terminal result; sustained values expose stuck transactions or nonce congestion. |
| Txmanager | `solver_bot_txmanager_gas_used_total` | `label`, `outcome` | Receipt gas for mined transactions, including reverts. Divide by the matching request count for average gas; this is gas units, not native-token cost. |
| Txmanager | `solver_bot_txmanager_fee_paid_wei_total` | `label`, `outcome` | Actual native-token fee paid by mined transactions, calculated from receipt `gasUsed × effectiveGasPrice`, including reverted and mined cancellation transactions. |
| Txmanager | `solver_bot_txmanager_replacements_total` | `label`, `kind` | Successfully broadcast replacements and cancellations. Spikes expose fee-policy or congestion problems that terminal outcomes alone cannot show. |
| Txmanager | `solver_bot_txmanager_admission_rejections_total` | `label`, `reason` | Requests rejected before the signed worker lifecycle. Reasons are `manager_stopped`, `nonce_conflict`, `deadline_exceeded`, `caller_cancelled`, or bounded fallback `other`; an expected busy `TrySend` probe is excluded. |
| Txmanager | `solver_bot_txmanager_admission_wait_duration_seconds` | `label`, `outcome` | Time from a real send request until worker admission or a terminal pre-admission outcome. `outcome` is `admitted` or one of the bounded rejection reasons; expected busy `TrySend` probes are excluded. |
| Txmanager | `solver_bot_txmanager_lifecycle_duration_seconds` | `label`, `outcome` | Time from worker admission through broadcast and terminal tracking. It excludes pre-admission nonce-lane wait, so use admission rejections alongside its latency distribution. |
| Txmanager | `solver_bot_txmanager_phase_duration_seconds` | `label`, `phase`, `outcome` | Time spent in each reached worker phase: `prebroadcast`, `pending`, or `confirming`. Reorgs may return a lifecycle to `pending`; the emitted sample contains the cumulative time spent in that phase. |
| Txmanager | `solver_bot_txmanager_account_info` | `address` | Constant `1` identifying the active public transaction-sender address; absent when no configured solver starts txmanager. Private key material is never exposed. |
| Txmanager | `solver_bot_txmanager_account_balance_wei` | — | Last complete native-token balance snapshot of the sender; absent until the first successful complete refresh. |
| Txmanager | `solver_bot_txmanager_account_latest_nonce` | — | Mined nonce from the same complete snapshot; absent until the first successful refresh. |
| Txmanager | `solver_bot_txmanager_account_pending_nonce` | — | Pending nonce from the same complete snapshot; compare with latest nonce to detect unknown pending work. It is absent until the first successful refresh. |
| Txmanager | `solver_bot_txmanager_account_refreshes_total` | `outcome` | Complete periodic account snapshots classified as `success` or `error`; failed reads retain the previous scrape-consistent snapshot. |
| Txmanager | `solver_bot_txmanager_account_last_successful_refresh_timestamp` | — | Freshness of the retained balance and nonce snapshot; absent until the first successful refresh. |
| Workflow | `solver_bot_workflow_events_total` | `solver`, `strategy`, `event`, `outcome` | Bounded solver events. Event/outcome pairs are fixed at construction; request data and errors cannot create labels. |
| Workflow | `solver_bot_workflow_last_event_timestamp` | `solver`, `strategy`, `event`, `outcome` | Last occurrence of the matching bounded event, including successful fills, quotes, wins, settlements, and refreshes. |
| Workflow | `solver_bot_workflow_amount_atomic_units_total` | `solver`, `strategy`, `event`, `asset`, `kind` | Event amounts in asset atomic units. Never aggregate unlike `asset` values; `planned_surplus` is gross planning output, not realized PnL. |
| Workflow | `solver_bot_workflow_observed_items` | `solver`, `strategy`, `view` | Last complete authoritative item count for a bounded state view. |
| Workflow | `solver_bot_workflow_last_observation_timestamp` | `solver`, `strategy`, `view` | Freshness paired with each retained workflow state count. |
| RFQ | `rfq_filler_http_request_duration_seconds` | `method`, `route`, `status` | Quote-server request count (`_count`), status funnel, and latency. Routes are allowlisted and methods are normalized to `GET`, `POST`, or `other` to bound cardinality. |
| RFQ | `rfq_active_orders` | — | Current queued, submitting, or submitted obligations awaiting terminal backend state. |
| RFQ | `rfq_oldest_active_order_age_seconds` | — | Age of the oldest active obligation; catches a single stuck order that a count-only alert can miss. |
| LI.FI | `lifi_active_quotes` | — | Process-local quote count from the last successful publication or suspension reconciliation. It can remain nonzero after the remote quotes expire at `quoteTtl`, so use it with refresh freshness rather than as backend state. |
| LI.FI | `lifi_active_quote_ranges` | — | Number of currently active standing-quote ranges from the last successful reconciliation. |
| LI.FI | `lifi_active_quote_max_input_atomic_units` | `token_in`, `token_out`, `token_in_decimals`, `token_out_decimals` | Largest currently advertised input range ceiling per token pair. Alternative curves are maxed rather than summed, so the gauge does not double-count shared capacity. |
| LI.FI | `lifi_last_successful_refresh_timestamp` | — | Freshness of standing-quote publication or suspension reconciliation; distinguishes an authoritative zero from a dead reconciliation loop. |
| LI.FI | `lifi_order_feed_connected` | — | `1` only while the order-feed loop owns an established WebSocket; `0` while disconnected, dialing, or backing off. |
| LI.FI | `lifi_order_recovery_ready` | — | `1` only when the current established order-feed connection has completed convergent REST recovery; every disconnect or reconnect resets it to `0`. |
| LI.FI | `lifi_order_backlog` | `stage` | Current process-local orders waiting in `inbox`, `recovery_retry`, `capacity_retry`, or `deposit_retry`. An item actively being processed is not queued. |
| LI.FI | `lifi_order_nearest_deadline_timestamp` | `stage` | Nearest protocol order deadline among work waiting in each stage; `0` when that stage is empty or its queued orders have no deadline. |
| UniswapX | `uniswapx_quote_duration_seconds` | — | End-to-end quote-handler latency across all request outcomes. |
| UniswapX | `uniswapx_exclusive_obligations_outstanding` | — | Live-observed or recovered obligations still awaiting terminal classification. |
| UniswapX | `uniswapx_exclusive_nearest_deadline_timestamp` | — | Nearest outstanding exclusivity deadline; alerts on urgent or stuck obligations. |
| UniswapX | `uniswapx_block_until_timestamp` | — | Maximum deadline among remote, local-fill, exclusive-fade, and startup-warmup time-based quote blockers. |
| UniswapX | `uniswapx_ready` | — | Scrape-time availability: `1` only when current quote state, breakers, exclusive delivery, and the transaction nonce lane permit quoting. |
| UniswapX | `uniswapx_last_quote_refresh_timestamp` | — | Last atomic quote-state publication. A successful publication may contain no inventory, so freshness alone is not readiness. |
| UniswapX | `uniswapx_last_exclusive_poll_timestamp` | — | Last successful exclusive poll plus recovery/obligation reconciliation. |
| UniswapX | `uniswapx_pending_fills` | — | Admitted fills holding LiquidLane capacity while awaiting a txmanager terminal result. |
| OEV | `oev_won_inflight` | `strategy` | Locally observed winning bids still awaiting settlement. |
| OEV | `oev_oldest_won_inflight_age_seconds` | `strategy` | Age since the oldest still-inflight win was locally observed; `0` when no locally won reservation remains. |
| OEV | `oev_hotpath_seconds` | `strategy` | End-to-end handling latency for parsed auction frames against the auction's short decision budget. |
| OEV | `oev_deposit_wei` | `strategy` | Executor deposit from the last complete state refresh; use it to monitor settlement runway. |
| OEV | `oev_feed_connected` | `strategy` | `1` only after the WebSocket is connected and every configured subscription frame has been sent; `0` before dial, during backoff, and from teardown onward. |
| 3F | `threef_backlog_nonempty_since_timestamp` | `view` | Process-local timestamp when complete authoritative snapshots first began continuously reporting a non-empty `active_requests` or `redeemable` backlog. `0` also means no authoritative non-empty observation has occurred yet, so pair it with view freshness. It resets on restart and is deliberately not presented as an individual request age. |

Bounded workflow dimensions:

| Solver | Events and outcomes | Amount/state dimensions |
|---|---|---|
| RFQ | `quote/<decision>`, `order/won`, `order_poll/success`, `fill/success` | `quote/{input,output}` and `fill/{input,output,planned_surplus}` by asset |
| LI.FI | `order_processing/<result>`, `queue_drop/<stage>`, `fill/success` | Fill amounts by asset and kind |
| UniswapX | `quote/<decision>`, `{exclusive,public}_order_poll/{ok,failed}`, `exclusive_obligation/{won,settled_in_time,missed}`, `fill/success` | Quote and fill amounts by asset and kind |
| OEV | `auction/<decision>`, `bid/{enqueued,won,settled_success,settled_failed,unresolved}`, `breaker/failure`, `state_refresh/success` | Native bid amounts use `asset="native"`; `kind` is the bid stage |
| 3F | `offer/{success,error}`, `redeem/success`; state views are `targets`, `offers`, `active_requests`, `redeemable` | Offer `principal` and `expected_yield` by deposit asset |

Event timestamps reset to `0` on restart; use `max_over_time(...[$__range])` when a dashboard should
retain a pre-restart observation inside its selected range.

External-operation labels are fixed at construction: 3F exposes `target_refresh`, `offer_refresh`,
`active_request_refresh`, and `redeemable_refresh`; RFQ exposes `order_poll`; LI.FI exposes
`quote_refresh`, `quote_suspend`, and `order_recovery`; UniswapX exposes `quote_refresh`,
`exclusive_order_poll`, and `public_order_poll`; OEV exposes `state_refresh`. `degraded` means a safe
partial or last-known-good path remained usable, while `skipped` means a deliberate gate, stale-plan
discard, or shutdown cancellation. Transaction sends are outside these timers.

Txmanager `label` values are stable operation names (`redeem`, `rfq-fill`, `lifi-fill`,
`uniswapx-fill`). Terminal outcomes are `confirmed`, `included_unconfirmed`, `reverted`, `cancelled`,
`submission_error`, and `tracking_stopped`. LiquidLane counters include successful receipts reported as
`included_unconfirmed`; they are operational telemetry rather than an accounting ledger, and amounts for
different token labels must not be added without price/decimal normalization.

### Grafana dashboards

Six native Grafana Dashboard Schema v2 templates are committed under
[`dashboards/`](dashboards/): a fleet-safe Runtime dashboard and single-instance dashboards for 3F,
RFQ, LI.FI, UniswapX, and OEV. Each JSON file is a bare `DashboardSpec` using a selectable
`${datasource}` and Kubernetes target labels `namespace`, `kubernetes_pod`, `job`, and `instance`.
Deployment resources and provisioning remain separate from these templates.

## Configuration

Config is YAML with a two-stage decode: the framework reads `solver.name` to select the
implementation and hands the opaque `solver.config` block to that solver to type. Each solver has its
own fully annotated example under `config/` (see the *Example config* column above) — every field,
including the applicable shared `chain`/`signer`/`txManager`/`observability` blocks, is documented
inline there.

The `chain` block takes a primary `rpcUrl` plus optional `rpcFallbackUrls` — HTTP(S) endpoints tried
in order for reads when the primary is unavailable. Signed broadcasts and both startup nonce reads
are pinned to `writeRpcUrl`, or the primary `rpcUrl` when it is omitted, and never fall over across
endpoints. Receipt confirmation does not rely on endpoint affinity: it requires a stable head and proves
that the receipt block belongs to that head by following hash-addressed parent headers. Each request keeps
normal read fallback behavior. A non-final endpoint's JSON-RPC `null` receipt or header result falls through
to the next read endpoint; the final endpoint's `null` remains the ordinary not-found result. Unavailable
multi-read snapshots retry on a later poll; OEV compares both number and hash around each latest-state
snapshot and retries a changed head once immediately. A second crossing fails startup or retains the runtime's
last-known-good snapshot until the next poll. An explicit write endpoint must report the same chain ID as the
read endpoint.

For transaction-sending solvers, startup fails closed when the write endpoint's pending nonce differs
from its latest mined nonce because `txManager` cannot recover an unknown signed lifecycle. The EOA
must be exclusive to this process: standard nonce reads cannot reveal a future transaction queued
beyond a gap. Before upgrading from a build that allowed several unresolved signed nonces, drain that
EOA's write-endpoint pool. After an unclean exit, nonce equality alone cannot rule out a private
submission hidden by its relay. The packaged Docker Compose deployment restarts automatically with
`unless-stopped`, so it can resume and reuse that nonce before the hidden submission becomes visible. If
the old attempt later consumes the nonce, `txManager` pauses admissions and readiness and remains
fail-closed for operator investigation; automatic restart does not recover the lost in-memory ownership.
For controlled maintenance, stop the service and reconcile outstanding private submissions before bringing
the EOA back.

At runtime, a post-signing `nonce too low` makes `txManager` check every exact signed attempt. During a
replacement of an already tracked lifecycle, a receipt proven canonical against a stable head resolves
ownership immediately. The lane remains non-ready only because that owned lifecycle is still active until
its confirmation depth is reached, not because ownership is uncertain. An initial-broadcast collision, or a
replacement with no owned canonical receipt, keeps new transactions and readiness paused until terminal
reconciliation or operator action; a later receipt reorg restores that pause. The calldata is not re-signed
at another nonce solely from that response. LiquidLane state reads always use RPC `latest`; an archive node
is not required.

**Never commit a real key or live config** — keys are supplied via env/file behind the `Signer`
interface; `*.local.*` and `.env` are gitignored.

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
