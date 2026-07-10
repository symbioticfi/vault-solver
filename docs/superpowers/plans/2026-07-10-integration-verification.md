# Cross-Cutting Documentation and Verification Implementation Plan

> **Public-port status:** This is the source-branch verification record. Its private `.github/chart/**`
> file list and receipt-attribution wording are non-applicable to the public tree and must not be used to
> recreate removed deployment or callback-receipt code. The current README, config examples, and
> subsystem plans are the authoritative public verification surface.

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Reconcile all operator/maintainer documentation with findings 2–20 and run the complete repository verification story after every subsystem lands.

**Architecture:** Behavioral documentation remains beside the owning subsystem plan, while this final pass performs a contradiction audit across README, charts, examples, and live TODO lists. Verification runs from the finished tree using the exact Go toolchain and deterministic generation target.

**Tech Stack:** Markdown/YAML, Go 1.26.5, golangci-lint 2.11.4, GNU Make, Git.

## Global Constraints

- Finding 1 remains out of scope; do not alter action or image digest policy.
- Do not claim completion from focused tests; run every final command fresh against the finished tree.
- Do not remove unrelated TODOs or rewrite adjacent prose.
- README is operator-facing; `docs/*-PLAN.md` is maintainer-facing.
- Every selected finding 2–20 must map to code/tests or a documented verification result.

---

### Task 1: Reconcile README, Charts, Examples, and Plans

**Files:**
- Modify: `README.md`
- Modify: `docs/3F-PLAN.md`
- Modify: `docs/RFQ-PLAN.md`
- Modify: `docs/OEV-PLAN.md`
- Modify: `docs/strategy-plan.md`
- Modify: `config/3f.example.yaml`
- Modify: `config/rfq.example.yaml`
- Modify: `config/redstone-oev.example.yaml`
- Modify: `.github/chart/mainnet.yaml`
- Modify: `.github/chart/sepolia.yaml`
- Modify: `.github/chart/hoodi.yaml`
- Modify: `.github/chart/vault-solver-3f-sepolia.yaml`

**Interfaces:**
- Consumes: final behavior and configuration from all prior plans.
- Produces: one non-contradictory operator/maintainer narrative and current TODO lists.

- [ ] **Step 1: Run the stale-claim scan and record every hit**

```bash
rg -n 'first \(and currently only\)|BridgeFacilitatorAdapter|\.github/chart/3f-sepolia\.yaml|public discounts|chain\.wsUrl|wsUrl\?|config field present but unused|Swap.*vault slot|applies a discount|stuck-tx bump|float32|maxRate.*float' \
  README.md docs/3F-PLAN.md docs/RFQ-PLAN.md docs/OEV-PLAN.md docs/strategy-plan.md \
  config .github/chart
```

Expected before reconciliation: hits include the known stale 3F, RFQ, chart, WebSocket, transaction,
and rate claims. Deliberately exclude `docs/superpowers/**`: the approved design and implementation
plans preserve historical finding terms as requirements and are not operator/maintainer claims.

- [ ] **Step 2: Correct the README and chart terminology**

Make these exact semantic corrections:

```text
BridgeFacilitatorAdapter -> ThreeFAdapter
.github/chart/3f-sepolia.yaml -> .github/chart/vault-solver-3f-sepolia.yaml
"public discounts" -> "internal-only discounts"
```

Document Go 1.26.5, `make check-generated`, strict RPC preflight, hard fee caps, and the response-size boundary only where operators need them. Keep finding 1 out of the deployment section.

- [ ] **Step 3: Reconcile RFQ architecture statements**

Ensure `docs/RFQ-PLAN.md` states all of the following together:

```text
- quote output has no extra filler discount;
- swap tuples use adapter terminology, not a vault slot;
- the default strategy owns the fill-plan cache;
- external mode requires/scopes configured adapters and never calls discounts;
- internal mode may use internal-only discounts; configured adapters scope quoting but not discount recovery filling;
- executable identity is exact-match selected and decoded signed order fields are authoritative;
- paused read failures are fail-closed;
- ambiguous transaction results reconcile as submitted rather than re-arm.
```

Delete only superseded contradictory sentences.

- [ ] **Step 4: Reconcile 3F and OEV architecture/TODOs**

Ensure `docs/3F-PLAN.md` describes all current solvers, `ThreeFAdapter`, configured adapter reality, exact deci-bps/webhook string, optional salt, offer TTL, generated-client bound, and explicit transaction outcomes. Remove the nonexistent sibling-document reference and generic WebSocket promise.

Ensure `docs/OEV-PLAN.md` describes WSS/loopback policy, read limit, per-component freshness, result dedup, bounded beam frontier, pinned-block exact Morpho fee/rate, header timestamp, and supervised attribution workers. Do not claim production Morpho state is entirely GraphQL.

Move completed selected findings out of live TODO lists; retain unrelated future work.

- [ ] **Step 5: Re-run the stale scan and inspect the diff**

```bash
! rg -n 'first \(and currently only\)|BridgeFacilitatorAdapter|\.github/chart/3f-sepolia\.yaml|public discounts|chain\.wsUrl|wsUrl\?|config field present but unused|Swap.*vault slot|applies a discount' \
  README.md docs/3F-PLAN.md docs/RFQ-PLAN.md docs/OEV-PLAN.md docs/strategy-plan.md \
  config .github/chart
git diff --check
git diff --stat
```

Expected: no listed stale claim remains; diff has no whitespace errors and only touches in-scope documentation/configuration.

- [ ] **Step 6: Commit documentation reconciliation**

```bash
git add README.md docs config .github/chart
git commit -m "docs: reconcile solver hardening behavior"
```

### Task 2: Map Every Finding to Evidence

**Files:**
- Modify: `docs/superpowers/specs/2026-07-09-findings-2-20-hardening-design.md` only if an implemented interface deliberately differs from the approved design and the rationale is already approved.
- Create: `.superpowers/sdd/findings-2-20-evidence.md` as ignored execution evidence; do not commit it.

**Interfaces:**
- Produces: an evidence table used by the final reviewer, not product documentation.

- [ ] **Step 1: Create the evidence table**

Write this table, updating a path or test name only if the final reviewed implementation uses a different concrete name:

```markdown
| Finding | Implementation | Regression test / verification |
|---|---|---|
| 2 | `internal/txmanager` and consumer reconciliation | `go test -race ./internal/txmanager ./internal/solvers/{rfq,bridgefacilitator}` |
| 3 | `bridgefacilitator` offer TTL | `go test ./internal/solvers/bridgefacilitator -run 'Test.*OfferTTL'` |
| 4 | RFQ exact executable selection/decoded order | `go test ./internal/solvers/rfq -run 'Test.*(Executable|SignedOrder)'` |
| 5 | OEV WS URL/read limit | `go test ./internal/solvers/redstoneoev -run 'Test.*WS'` |
| 6 | OEV component freshness | `go test ./internal/solvers/redstoneoev -run 'Test.*Fresh'` |
| 7 | RPC endpoint preflight/redaction | `go test ./internal/chain -run 'Test.*(ChainID|Redact)'` |
| 8 | OEV result dedup | `go test ./internal/solvers/redstoneoev -run 'Test.*Duplicate.*Result'` |
| 9 | RFQ nonzero executor/paused fail-closed | `go test ./internal/solvers/rfq -run 'Test.*(ZeroExecutor|Paused)'` |
| 10 | bounded generated responses | `go test ./internal/httptransport ./internal/solvers/{rfq,bridgefacilitator} -run 'Test.*Oversized'` |
| 11 | fee validation/hard cap | `go test ./internal/config ./internal/txmanager -run 'Test.*(Fee|Gwei|Tip)'` |
| 12 | bounded OEV beam | `go test ./internal/solvers/redstoneoev -run 'Test.*Beam' -bench 'Benchmark.*Bundle'` |
| 13 | exact Morpho state/rate | `go test ./internal/morpho ./internal/solvers/redstoneoev -run 'Test.*(Accrual|MarketRate|MulticallAt)'` |
| 14 | exact 3F rate/salted domain | `go test ./internal/solvers/bridgefacilitator/... -run 'Test.*(DeciBps|Salt)'` |
| 15 | amortized RFQ cache sweep | `go test ./internal/solvers/rfq/strategies/default -run 'Test.*Sweep'` |
| 16 | supervised workers/listener errors | `go test ./cmd/vault-solver ./internal/observability ./internal/solvers/{rfq,redstoneoev}` |
| 17 | checksum and codegen drift | `make check-generated` plus bad-checksum launcher command |
| 18 | signer and protocol boundary characterization | `go test -race ./internal/signer ./internal/solvers/{rfq,redstoneoev,bridgefacilitator}` |
| 19 | Go 1.26.5 pins | `go version`, `go mod verify`, and pin scan |
| 20 | README/plan/chart reconciliation | stale-claim `rg` scan and `git diff --check` |
```

No row may be removed. For documentation/toolchain findings, retain the deterministic command and changed file.

- [ ] **Step 2: Cross-check paths and tests mechanically**

```bash
while IFS='`' read -r _ path _; do
  case "$path" in
    */* ) test -e "$path" || { echo "missing evidence path: $path"; exit 1; } ;;
  esac
done < .superpowers/sdd/findings-2-20-evidence.md
```

Expected: every referenced repository path exists. Manually verify every test command names a test that appears in `go test -list` for its package.

### Task 3: Run the Full Verification Gate

**Files:**
- Verify only; fix failures in the owning task with a failing regression test, then restart this gate from Step 1.

**Interfaces:**
- Produces: fresh completion evidence for the final whole-branch review.

- [ ] **Step 1: Format and lint-autofix**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local golangci-lint run --fix
git diff --check
```

Expected: exit 0 and no unexplained formatting diff.

- [ ] **Step 2: Build all packages**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go build ./...
```

Expected: exit 0.

- [ ] **Step 3: Run race and coverage tests**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race -cover ./...
```

Expected: every package reports `ok` or a no-test-files coverage line; zero failures/races.

- [ ] **Step 4: Run final lint**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local golangci-lint run
```

Expected: exit 0 with zero issues.

- [ ] **Step 5: Verify generated code and module graph**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local make check-generated
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go mod tidy
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go mod verify
git diff --exit-code -- go.mod go.sum api
```

Expected: deterministic generation, tidy module files, and verified modules with no diff.

- [ ] **Step 6: Record OEV beam performance**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -run '^$' -bench 'Benchmark.*Bundle' -benchmem ./internal/solvers/redstoneoev
```

Expected: benchmarks complete for 100, 1,000, and 10,000 candidates and report allocations.

- [ ] **Step 7: Confirm clean tree and finding scope**

```bash
git status --short
git log --oneline d80fb57..HEAD
```

Expected: no uncommitted product files; commits cover findings 2–20 and no finding-1-only work.
