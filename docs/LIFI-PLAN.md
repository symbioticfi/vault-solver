# vault-solver — LI.FI / Catalyst same-chain intent filler (plan)

Adding a **`lifi`** solver to `vault-solver` that fills **same-chain** LI.FI Intents (Open Intents
Framework / Catalyst) by redeeming the intent's input RWA through a Symbiotic **LiquidLane adapter** to
produce the output — **atomically, with no held inventory**. Follows the framework boundary and
conventions in [`../CLAUDE.md`](../CLAUDE.md); the strategy layer follows
[`strategy-plan.md`](strategy-plan.md).

> **Status:** planned (design). Spans two repos: an on-chain executor contract in the sibling `rfq`
> repo, and the off-chain Go solver here.

---

## 1. What it does

A user signs an intent: "here is X of RWA token `tokenIn`; pay me ≥ Y of `tokenOut` (the redeemed
underlying)." The LI.FI order server matches that intent to our standing quote and pushes us the
**signed `StandardOrder`**. We settle it on-chain in **one atomic transaction** via the LI.FI escrow
settler's `openForAndFinalise`, which:

1. pulls the user's RWA input (via their permit2/ERC-3009 signature) and hands it to **our executor
   contract** (`destination`),
2. calls back into our executor (`orderFinalised`), where — with the RWA already in hand — the
   executor **redeems it through the LiquidLane adapter** to produce `tokenOut`, pays the user via the
   OutputSettler, and self-attests,
3. verifies the fill and reverts the whole tx if anything fell short.

Because settlement is atomic and the output is produced from the just-received input, the solver holds
**zero output inventory** and carries **no float / FX / rebalancing** risk. Profit is the redemption
surplus: `adapter.getAmountOut(RWA, X) − Y`, retained in the executor and swept by its owner.

This is the same-chain specialization of the cross-chain OIF flow. Same-chain is strictly simpler:
`inputOracle == OutputSettler` (the settler is its own oracle — no cross-chain proof relay), and
open + fill + finalise happen in one tx.

---

## 2. How it maps onto the framework

A new self-contained `internal/solvers/lifi/` implementing `solver.Solver` — no framework edits
(CLAUDE.md modularity rule). Reused as-is:

- **`Run(ctx)`** connects to the LI.FI order server (WebSocket order feed), refreshes standing quotes
  on an interval, and drives the fill loop; blocks until ctx cancels.
- **Fills go through the shared `txmanager`** — the solver builds the `openForAndFinalise` calldata;
  txmanager owns the nonce, send, and receipt/revert. Same nonce-serialized EOA as every other solver.
- **On-chain reads use `chain.Multicall`** — adapter `getAmountOut` / `getMaxAssets` / `getMaxRate`
  batched per quote/price refresh.
- **Signer** — the framework EOA is the registered LI.FI **solver address** and the tx sender. It is
  *not* an on-chain signer for the intent (the user signs that); it only sends `openForAndFinalise`.
- **Config, secrets** — order-server URL + `apiKeyEnv`, settler/executor/adapter addresses via
  `solver.config`; the LI.FI API key via `*Env` indirection.
- **Pluggable strategy** — the fill decision (price, skip, size) is a strategy
  (`default` in-process; `webhook` optional later), per [`strategy-plan.md`](strategy-plan.md).

### Component / repo map

| Piece | Where | Responsibility |
|---|---|---|
| `LiquidLaneLifiExecutor` (Solidity) | `../rfq/src/lifi/` | OIF `IInputCallback` callback: redeem input via adapter → `fill` → `setAttestation`. Contract-of-record. |
| Vendored OIF interfaces/structs | `../rfq/src/lifi/interfaces/` | `IInputCallback`, `MandateOutput`, `StandardOrder`, OutputSettler `fill`/`setAttestation` surface. |
| `lifi` solver (Go) | `internal/solvers/lifi/` | Order-server client, pricing, decision, `openForAndFinalise` calldata, submit. |
| `strategies/{default,webhook}` (Go) | `internal/solvers/lifi/strategies/` | Fill decision (price/skip/size). |
| LI.FI order server | external | Discovery: standing quotes + matched-order WS feed. |
| OIF settlers | on-chain (LI.FI-owned) | Order lifecycle; **we do not deploy these**. |

---

## 3. On-chain executor — `LiquidLaneLifiExecutor` (contract-of-record)

New contract in `../rfq/src/lifi/`, modeled on `src/oev/SymbioticOevSolver.sol` (a self-contained
callback for an external protocol that routes through a LiquidLane adapter). It does **not** reuse the
RFQ `Reactor` — the OIF settlers already own the order/signature/nonce/settlement lifecycle.

### Interface

```solidity
// IInputCallback (vendored from OIF) — the settler calls this on `destination`.
function orderFinalised(uint256[2][] calldata inputs, bytes calldata call) external;
```

`inputs` are the RWA amounts already delivered to the executor. `call` is the ABI payload our Go bot
builds. Proposed encoding:

```solidity
struct FillCall {
    address adapter;         // the LiquidLane adapter to redeem through (must be allowlisted)
    address outputSettler;   // OIF OutputSettler to fill + attest on
    bytes32 orderId;         // OIF order id
    MandateOutput output;    // the single output to satisfy (token, amount, recipient, ...)
    uint48  fillDeadline;    // from the order
    bytes32 solver;          // our registered solver identifier (fillerData / attestation)
}
```

### `orderFinalised` flow (inside the atomic tx)

1. `require(INPUT_SETTLER == msg.sender)` — only the OIF escrow settler may call.
2. Decode `call`; `require(_isAllowedAdapter(fc.adapter))` and `require(fc.outputSettler == OUTPUT_SETTLER)`.
3. Transfer the input RWA to the adapter (`ILiquidLaneAdapter.swap` "assumes tokenIn already
   transferred to the adapter") — `SafeERC20.safeTransfer(tokenIn, fc.adapter, amountIn)`.
4. `fc.adapter.swap(Swap{recipient: address(this), tokenIn, amountIn, amountOut: fc.output.amount})` —
   the redeemed underlying lands in the executor. (`Swap{address recipient; address tokenIn; uint256
   amountIn; uint256 amountOut;}`.)
5. `require(IERC20(outputToken).balanceOf(self) >= fc.output.amount)` — the redemption covered the
   output (belt-and-suspenders; the adapter should deliver `amountOut`).
6. `forceApprove(outputToken, OUTPUT_SETTLER, fc.output.amount)`.
7. `OUTPUT_SETTLER.fill(fc.orderId, fc.output, fc.fillDeadline, abi.encode(fc.solver))` — pays the user
   (`transferFrom(executor → recipient)`).
8. `OUTPUT_SETTLER.setAttestation(fc.orderId, fc.solver, uint32(block.timestamp), fc.output)` — writes
   the local attestation the settler's `_validateFillsNow` reads (same-chain oracle == settler).
9. Surplus (`redeemed − fc.output.amount`) stays in the executor.

### Authorization & safety

- **`INPUT_SETTLER`, `OUTPUT_SETTLER` immutable** (constructor); adapters via an **allowlist**
  (`setAdapters`, owner-only) or an adapter-factory `isEntity` membership check.
- **`onlyOwner` sweep** for accumulated surplus (`sweep(token, to)`); the executor holds no funds
  between txs otherwise.
- The executor must be a **registered filler on each LiquidLane adapter** (adapter `marketMaker` /
  `owner` / delegated `isFiller` == executor) — an onboarding prerequisite, exactly like the RFQ
  `Executor`. `adapter.swap` reverts `InvalidCaller` otherwise.
- Attack surface is bounded: `openForAndFinalise` requires the **user's signature** to open at all, and
  `_validateFillsNow` reverts the whole tx unless the output was paid — so a griefer with a signed
  order can at worst make a valid fill on our behalf (paying gas), never redirect the surplus (it stays
  in the executor, owner-swept).

### Placement & house style

`src/lifi/LiquidLaneLifiExecutor.sol` + `src/lifi/interfaces/ILiquidLaneLifiExecutor.sol` + vendored
`src/lifi/interfaces/{IInputCallback,IOutputSettler,...}.sol` (MIT, mirroring `src/oev/interfaces/`).
solc `0.8.28`, BUSL-1.1 header, `forge fmt` (120-col, tabs, double quotes, `int_types=long`), I-prefixed
interface with full NatSpec, section separators, `callers`/`setCallers`-style patterns. Tests:
`test/lifi/LiquidLaneLifiExecutor.t.sol` (unit, inline mocks à la `test/Reactor.t.sol`) +
`test/lifi/LiquidLaneLifiIntegration.t.sol` (end-to-end same-chain, modeled on
`catalystsystem/lifi-intent/test/integration/InputSettler7683LIFI.samechain.t.sol` and
`test/CoreMirrorIntegration.t.sol`), aiming for 100% line/branch coverage.

---

## 4. The order model & same-chain lifecycle

**`StandardOrder`** (OIF): `{ address user; uint256 nonce; uint256 originChainId; uint32 expires;
uint32 fillDeadline; address inputOracle; uint256[2][] inputs; MandateOutput[] outputs; }`. Inputs are
`[tokenId, amount]` (token as `uint256(uint160(addr))`).

**`MandateOutput`** (OIF): `{ bytes32 oracle; bytes32 settler; uint256 chainId; bytes32 token; uint256
amount; bytes32 recipient; bytes callbackData; bytes context; }`. Same-chain: `oracle == settler ==
OutputSettler`, `chainId == block.chainid`, empty `callbackData`/`context`, and `order.inputOracle ==
OutputSettler`.

**Entrypoint:** `InputSettlerEscrowLIFI.openForAndFinalise(StandardOrder order, address sponsor, bytes
signature, address destination, bytes call)` — `sponsor == order.user`; `signature` = `b1 sigType (0x00
permit2 / 0x01 3009) || sig`; `destination` = our executor (receives inputs, is the solver identity);
`call` = the `FillCall` payload above. Emits `Open(orderId)` then `Finalised(...)`.

**Deployed addresses** (LI.FI-owned; integrate against these — do **not** deploy):
- `InputSettlerEscrowLIFI` (has `openForAndFinalise`): `0x000025c3226C00B2Cdc200005a1600509f4e00C0`
- OutputSettler (LIFI): `0x0000000000eC36B683C2E6AC89e9A75989C22a2e`
- (bare OIF reference set: `InputSettlerEscrow 0x1CC9260E285C2C8AC8D2E7102F3978056Ec1d0a8`,
  `OutputSettlerSimple 0x52602D7cc3D833F5d28ee6D01C7F82C9b2322e10` — deployed at identical addresses on
  Sepolia-family testnets + mainnets via CREATE2. Use the LI.FI addresses for order-server integration;
  confirm the exact settler the order server routes for our chain in P1.)

---

## 5. Off-chain solver design

### 5.1 Discovery (LI.FI order server)

One-time onboarding (self-serve, no KYC): create a solver identity + API key in the solver UI
(prod `intents.li.fi`, testnet `devintents.li.fi`), then register the framework EOA as the solver
address by signing a registration message and `POST /solver-api/account/register` (with `api-key`
header). One address ↔ one API key.

Runtime:
- **Standing quotes** — every `quoteRefresh` interval, compute a price curve for each configured
  RWA→underlying route from `adapter.getMaxRate` / `getAmountOut` / `getMaxAssets`, and
  `POST /quotes/submit` with `exclusiveFor = <our solver address>` so the order server routes matched
  orders exclusively to us (we are the only party with the executor + adapter filler auth).
- **Order feed** — subscribe to the WebSocket `user:vm-order-submit` event; each message carries a
  signed `StandardOrder` matched to one of our quotes. Dedup on `orderId`.

### 5.2 Pricing & decision (the strategy)

For a matched order: read live `adapter.getAmountOut(tokenIn, amountIn)` → `redeemed`, and
`adapter.getMaxAssets(tokenIn)` → capacity. Fill iff:
- `redeemed >= output.amount + minMargin` (profitable after the surplus we keep),
- `amountIn <= capacity` (adapter can absorb the redemption this request),
- the adapter's collateral/asset matches `output.token`, and the order isn't past `fillDeadline` /
  `expires`.

Otherwise skip. The `default` strategy implements this in-process; a `webhook` strategy can delegate it
later (same trusted-strategy model as rfq/3f/oev).

### 5.3 Build & submit

Encode the `FillCall` payload → build `InputSettlerEscrowLIFI.openForAndFinalise(order, order.user,
signature, EXECUTOR, call)` via generated bindings → submit through `txmanager`. One tx per fill; an
on-chain revert (e.g. someone else filled, or price moved) marks the attempt failed and it's dropped
(the order is gone).

### 5.4 Config block (sketch)

```yaml
solvers:
  - name: lifi-samechain
    config:
      strategy: { name: default, config: {} }
      orderServer:
        baseUrl: https://order-dev.li.fi          # order.li.fi in prod
        wsUrl:   wss://order-dev.li.fi/...         # confirm exact WS path in P1
        apiKeyEnv: LIFI_SOLVER_API_KEY
      solverAddress: "0x…"                          # our registered solver EOA (== signer)
      inputSettler:  "0x000025c3226C00B2Cdc200005a1600509f4e00C0"
      outputSettler: "0x0000000000eC36B683C2E6AC89e9A75989C22a2e"
      executor:      "0x…"                          # our deployed LiquidLaneLifiExecutor
      adapters:                                      # LiquidLane adapters (RWA→underlying); vault+asset resolved on-chain
        - "0x…"
      minMarginBps: 10                               # required surplus over the order's output
      intervals: { quoteRefresh: 30s, statePoll: 10s }
```

---

## 6. Data flow (end to end)

```
LI.FI order server ──(WS: signed StandardOrder)──▶ lifi solver
  price: adapter.getAmountOut(RWA, X) → redeemed ; adapter.getMaxAssets → cap
  decide: redeemed ≥ output.amount + margin && X ≤ cap ?  ── no ─▶ skip
     │ yes
  build FillCall + openForAndFinalise(order, user, sig, EXECUTOR, call)
     │
  txmanager ─▶ InputSettlerEscrowLIFI.openForAndFinalise(...)
                 ├─ pull user's RWA (permit2) → EXECUTOR
                 ├─ EXECUTOR.orderFinalised(inputs, call):
                 │     RWA → adapter ; adapter.swap(→ underlying to EXECUTOR)
                 │     OUTPUT_SETTLER.fill(orderId, output, deadline, solver)  // pays user
                 │     OUTPUT_SETTLER.setAttestation(orderId, solver, ts, output)
                 └─ _validateFillsNow ✓  (atomic; reverts all if unfilled)
  surplus (redeemed − output.amount) accrues in EXECUTOR → owner sweeps
```

---

## 7. Error handling & safety

- **Atomic revert-safety** is the backbone: if the redemption under-delivers, the adapter reverts, or
  the output isn't paid, `_validateFillsNow` reverts the entire tx — no partial state, no stuck funds.
- **Pre-submit skips** (never send a doomed tx): unprofitable (`redeemed < output.amount + margin`),
  over-capacity (`amountIn > getMaxAssets`), asset mismatch, past deadline/expiry, adapter paused.
- **Staleness** — price/capacity reads are refreshed on `statePoll`; a matched order is priced against
  a fresh read at decision time, not the quote-time curve.
- **Competition** — same-chain fills are winner-take-all on-chain; `exclusiveFor` on our quotes routes
  matched orders to us, but a late/again-priced fill can still revert (already filled) → drop.
- **No inventory / callback-balance risk** (unlike OEV): nothing is fronted; the only capital at risk
  per tx is gas, and reverts cost only gas.
- **Executor surplus** is the sole standing balance; owner-swept, never user-redirectable.

---

## 8. Development, testing & deployment

**Dev is testnet-first.** We develop against the real order server, real settlers, and a real
LiquidLane adapter on a public testnet, so the dev environment matches production one-for-one. The
local foundry loop (§8.3) is kept only for fast contract-unit iteration, not for integration.

### 8.1 Onboarding (verified, self-serve)
No KYC / approval gate. Testnet UI `devintents.li.fi` + order server `order-dev.li.fi` (both live; prod
is `intents.li.fi` / `order.li.fi`). Create a solver identity → API key → sign a registration message
and `POST /solver-api/account/register` the solver EOA. Secret via `LIFI_SOLVER_API_KEY` env.

### 8.2 Testnet dev environment (primary loop)
Target **Ethereum Sepolia** (chainId 11155111) — the intersection of: LI.FI `order-dev` support, the
canonical OIF settlers (deployed there, §4 addresses), and an existing Symbiotic **LiquidLane adapter**
(the redstone-oev / rfq work already runs on Sepolia LiquidLane adapters). Confirm one adapter that
redeems a testnet RWA → its underlying, or point at/deploy one (an open item, §10).

One-time setup:
1. **Executor** — deploy `LiquidLaneLifiExecutor` to Sepolia (`INPUT_SETTLER`/`OUTPUT_SETTLER` = the
   LI.FI addresses in §4; adapter allowlist = the Sepolia LiquidLane adapter).
2. **Filler auth** — the adapter's vault creator registers our executor as a filler
   (`marketMaker`/`owner`/`isFiller` == executor), or we target an adapter where we already are.
3. **Solver identity** — register the framework EOA on `devintents.li.fi` + `POST .../register`; fund
   it with Sepolia ETH for gas.
4. **Config** — a `config/lifi.sepolia.example.yaml` pointing `orderServer` at `order-dev.li.fi`, the
   §4 settler addresses, our deployed executor, and the Sepolia adapter(s).

The loop, on every change:
1. Run the bot → it submits an **exclusive** standing quote (`exclusiveFor = our solver addr`) for the
   RWA→underlying route to `order-dev.li.fi`.
2. Create a matching **test order** from a second (user) key — easiest via the `lintent.org` reference
   UI in **"Escrow" mode**, or a small script that signs a `StandardOrder` + permit2 and submits it.
3. The order server matches it to our exclusive quote and pushes it over the WS feed → the bot prices
   it, builds `openForAndFinalise`, and settles it atomically on Sepolia.
4. Inspect the tx (redeem → fill → attest), the user's received output, and the executor's accrued
   surplus. Iterate.

This exercises the full real path — order server, WS, settlers, adapter, txmanager — every iteration.

### 8.3 Local contract loop (fast iteration only)
For quick Solidity iteration without a network: foundry/anvil, self-deploy the OIF settlers + a
real/mock adapter, and drive `openForAndFinalise` — the shape of Catalyst's
`InputSettler7683LIFI.samechain.t.sol`. This is the `forge test` unit/integration coverage of the
executor, **not** the integration loop (§8.2 is). The Go side is unit-tested against an `httptest`
order-server mock + a simulated/forked chain backend.

### 8.4 Mainnet deployment
- **We do not deploy the settlers** — LI.FI/OIF canonical deployments at fixed addresses.
- **Per chain we deploy** `LiquidLaneLifiExecutor` (+ register it as a filler on each target LiquidLane
  adapter), register the solver EOA, fund gas, and run the bot — the same steps as §8.2 but against
  `order.li.fi`. LI.FI is live on Ethereum, Base, Optimism, Arbitrum, Polygon, BSC, Katana, MegaETH,
  etc. (`order.li.fi/chains/supported` authoritative); v1 targets the chain(s) hosting the LiquidLane
  RWA adapters we serve.
- No official solver SDK — integrate against the order-server OpenAPI (`/docs`) + WS feed (+ the MCP
  server for scripted testing).

---

## 9. Build phases

Testnet-first: the executor is on Sepolia from P0 so every later phase integrates against the live
`order-dev.li.fi` + real settlers + real adapter (§8.2).

0. **Contract + Sepolia deploy** — vendor OIF interfaces into `../rfq/src/lifi/interfaces/`; write
   `LiquidLaneLifiExecutor` + foundry unit/integration tests (self-deployed settlers + adapter, the
   §8.3 local loop). Then **deploy to Ethereum Sepolia and register the executor as an adapter filler**.
   CGO-free rfq build stays green; `forge fmt`/`forge test`/coverage pass.
1. **Order-server client** — Go client for register / `POST /quotes/submit` / the WS order feed, wired
   to the live `order-dev.li.fi`; register the solver EOA; config parsing + framework wiring
   (`solver.Register`, blank-import). `httptest`-backed unit tests, validated live.
2. **Pricing + decision + tx build** — `default` strategy (getAmountOut/getMaxAssets, margin, asset
   match); `FillCall` encoding + `openForAndFinalise` calldata via generated bindings; txmanager submit.
   Validated end-to-end on Sepolia by self-filling a `lintent.org` order matched to our exclusive quote.
3. **Harden** — staleness/skip edge cases, revert handling, metrics on the shared observability server;
   a repeatable green E2E on Sepolia (`order-dev`).
4. **Mainnet** — deploy the executor per target chain, point config at `order.li.fi`, register the
   solver, and run.

---

## 10. Open items / prerequisites

- **Order-server same-chain settlement contract** — confirm in P1 that the order server routes a
  matched same-chain order with the user's permit2 signature such that *we* self-settle via
  `openForAndFinalise` (the atomic path is proven in the foundry test; the public `filling-orders` doc
  documents the cross-chain `fillOrderOutputs` path — verify the same-chain wrapper is what the order
  server expects, or whether it drives a different settlement call).
- **Exact WS endpoint + quote schema** — the `POST /quotes/submit` body (price-curve/range shape) and
  the `user:vm-order-submit` payload; pull from `order-dev.li.fi/docs` during P1.
- **Adapter filler registration** — our executor must be granted filler rights on each LiquidLane
  adapter (`marketMaker`/`owner`/`isFiller`), by the adapter's vault creator. Onboarding prereq.
- **Sepolia testnet adapter (pin first, blocks P0/§8.2)** — confirm a Sepolia LiquidLane adapter that
  redeems a testnet RWA → underlying for the dev loop. The redstone-oev harness already uses a Sepolia
  LiquidLane adapter (the TLOAN vault / TCOL testbed); confirm it (or a similar one) is reusable and
  that our executor can be registered as its filler — otherwise deploy/point at one.
- **v1 scope** — which RWA↔underlying adapter(s), token(s), and chain(s) to launch on.
- **Reputation thresholds** — the numeric fill-rate/speed cutoffs that gate exclusive orders are
  undocumented; `exclusiveFor` on our own quotes should make this moot for v1.
- **`getMaxAssets` is non-view** (mutates) — read via a call, not a static-call, in the pricing path
  (mirror how the RFQ solver handles it).
