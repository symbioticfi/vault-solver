# Generic Runtime and Tooling Hardening Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Upgrade to Go 1.26.5, make code generation reproducible, validate every RPC endpoint, bound generated-client responses, add pinned-block multicalls, reject unsafe fees, and remove the unused generic WebSocket setting.

**Architecture:** Generic safety mechanisms stay under `internal/{chain,config,httptransport,txmanager}` and integrations compose them without protocol knowledge. CI regenerates only from committed interface artifacts. Runtime changes fail closed at startup or at the HTTP/RPC boundary.

**Tech Stack:** Go 1.26.5, go-ethereum, standard-library HTTP, GNU Make, Bash, Java OpenAPI Generator 7.12.0, GitHub Actions.

## Global Constraints

- Finding 1 is out of scope: do not add workflow or container-image digest pins beyond existing pins.
- Keep `go 1.26`; set every exact toolchain pin to `go1.26.5`.
- Never hand-edit generated Go under `api/`; regenerate from committed inputs.
- Use `github.com/go-errors/errors`, not `fmt.Errorf`, in production code.
- Keep protocol-specific behavior out of generic packages.
- Follow strict TDD for Go behavior and record RED/GREEN commands in the task report.
- Update examples/docs in the same task as user-visible behavior.

---

### Task 1: Pin Go 1.26.5 Everywhere

**Files:**
- Modify: `go.mod`
- Modify: `deploy/Dockerfile`
- Modify: `scripts/oev/oev-testrun.sh`
- Modify: `scripts/oev/oev-fork-refuel.sh`
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: official toolchain `go1.26.5`.
- Produces: one exact version used by `go.mod`, Docker, scripts, and contributor commands.

- [ ] **Step 1: Record the current pin inventory**

Run:

```bash
rg -n '1\.26\.4|go1\.26\.4' go.mod deploy scripts CLAUDE.md
```

Expected: matches in the five files above; this is the pre-change failure inventory.

- [ ] **Step 2: Replace exact pins without changing the language directive**

Apply these replacements:

```text
go.mod:                 toolchain go1.26.5
deploy/Dockerfile:      FROM golang:1.26.5-alpine3.23 AS build
deploy/Dockerfile:      ENV GOTOOLCHAIN=go1.26.5
scripts/oev/*.sh:       GOTOOLCHAIN=go1.26.5
CLAUDE.md commands:     GOTOOLCHAIN=go1.26.5
```

Leave `go 1.26` unchanged.

- [ ] **Step 3: Verify toolchain and module graph**

Run:

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go version
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go mod tidy
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go mod verify
git diff --exit-code -- go.sum
! rg -n '1\.26\.4|go1\.26\.4' go.mod deploy scripts CLAUDE.md
```

Expected: `go version go1.26.5`, verification succeeds, and `go.sum` is unchanged.

- [ ] **Step 4: Commit**

```bash
git add go.mod deploy/Dockerfile scripts/oev/oev-testrun.sh scripts/oev/oev-fork-refuel.sh CLAUDE.md
git commit -m "build(deps): update Go toolchain to 1.26.5"
```

### Task 2: Verify OpenAPI Generator and Generated-Code Drift

**Files:**
- Modify: `hack/openapi-generator-cli.sh`
- Modify: `Makefile`
- Modify: `.github/workflows/ci.yml`
- Modify: `README.md`
- Modify: `CLAUDE.md`

**Interfaces:**
- Consumes: generator `7.12.0`, SHA-256 `33e7dfa7a1f04d58405ee12ae19e2c6fc2a91497cf2e56fa68f1875a95cbf220`.
- Produces: `make check-generated`, which regenerates from vendored inputs and rejects tracked/untracked drift.

- [ ] **Step 1: Verify RED: bad checksum is currently ignored**

Run:

```bash
OPENAPI_GENERATOR_VERSION=7.12.0 OPENAPI_GENERATOR_SHA256=deadbeef \
  bash ./hack/openapi-generator-cli.sh version
```

Expected before implementation: the generator runs instead of rejecting the checksum.

- [ ] **Step 2: Harden the launcher**

Implement this complete launcher behavior while retaining Homebrew Java compatibility:

```bash
#!/usr/bin/env bash
set -euo pipefail

: "${OPENAPI_GENERATOR_VERSION:?OPENAPI_GENERATOR_VERSION must be set}"
: "${OPENAPI_GENERATOR_SHA256:?OPENAPI_GENERATOR_SHA256 must be set}"
jar="${TMPDIR:-/tmp}/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar"
url="https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/${OPENAPI_GENERATOR_VERSION}/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar"

verify_jar() {
  local file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    printf '%s  %s\n' "$OPENAPI_GENERATOR_SHA256" "$file" | sha256sum -c - >/dev/null
  else
    printf '%s  %s\n' "$OPENAPI_GENERATOR_SHA256" "$file" | shasum -a 256 -c - >/dev/null
  fi
}

if [[ -f "$jar" ]] && ! verify_jar "$jar"; then rm -f "$jar"; fi
if [[ ! -f "$jar" ]]; then
  tmp=$(mktemp "${jar}.XXXXXX")
  trap 'rm -f "$tmp"' EXIT
  curl -fL "$url" -o "$tmp"
  verify_jar "$tmp"
  mv "$tmp" "$jar"
  trap - EXIT
fi
PATH="/opt/homebrew/opt/openjdk/bin:$PATH" \
  java -ea ${JAVA_OPTS:-} -Xms512M -Xmx1024M -server -jar "$jar" "$@"
```

- [ ] **Step 3: Verify checksum RED becomes GREEN**

Run:

```bash
if OPENAPI_GENERATOR_VERSION=7.12.0 OPENAPI_GENERATOR_SHA256=deadbeef \
  bash ./hack/openapi-generator-cli.sh version; then exit 1; fi
OPENAPI_GENERATOR_VERSION=7.12.0 \
OPENAPI_GENERATOR_SHA256=33e7dfa7a1f04d58405ee12ae19e2c6fc2a91497cf2e56fa68f1875a95cbf220 \
  bash ./hack/openapi-generator-cli.sh version
```

Expected: bad checksum exits non-zero; correct checksum prints `7.12.0`.

- [ ] **Step 4: Add checksum plumbing and `check-generated`**

Add the checksum beside the version, pass it through `gen_openapi_client`, then add:

```make
OPENAPI_GENERATOR_SHA256 ?= 33e7dfa7a1f04d58405ee12ae19e2c6fc2a91497cf2e56fa68f1875a95cbf220

.PHONY: check-generated
check-generated: generate ## Regenerate committed code and fail on drift
	@git diff --exit-code -- api/bindings api/threef api/rfqbackend api/morphographql api/graphql/morpho/operations.json
	@untracked="$$(git ls-files --others --exclude-standard -- api/bindings api/threef api/rfqbackend api/morphographql api/graphql/morpho/operations.json)"; \
		test -z "$$untracked" || { echo "untracked generated files:"; echo "$$untracked"; exit 1; }
```

- [ ] **Step 5: Add CI and documentation**

Add a `generated` job using the workflow's existing pinned checkout/setup-go steps, followed by:

```yaml
      - name: Verify Java
        run: java -version
      - name: Install code-generation tools
        run: make tools
      - name: Verify generated code is current
        run: make check-generated
```

Do not add an action or alter existing action pins. Document `make check-generated`, the JAR checksum, and that CI never refreshes live artifacts in `README.md` and `CLAUDE.md`.

- [ ] **Step 6: Verify clean and dirty generation**

Run:

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local make tools
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local make check-generated
```

Then use `apply_patch` to add a temporary optional string property named `codexDriftProbe` to one
schema in `openapi/3f-bf.openapi.json`. Verify `make check-generated` exits non-zero because the
changed vendored contract regenerates a different tracked client. Remove only that temporary schema
property with `apply_patch`, run `make check-generated` again, and expect exit 0 with no generated
diff. Do not use a generated-file-only mutation: regeneration would erase it before the drift check
and produce a false GREEN.

- [ ] **Step 7: Commit**

```bash
git add hack/openapi-generator-cli.sh Makefile .github/workflows/ci.yml README.md CLAUDE.md
git commit -m "ci(codegen): verify deterministic generated output"
```

### Task 3: Add a Reusable HTTP Response-Body Limit

**Files:**
- Create: `internal/httptransport/response_limit.go`
- Create: `internal/httptransport/response_limit_test.go`

**Interfaces:**
- Produces: `func LimitResponses(base http.RoundTripper, limit int64) http.RoundTripper`.
- Produces: sentinel `ErrResponseTooLarge` and typed `ResponseTooLargeError` for both declared and
  streamed overflow; streamed overflow also unwraps to `*http.MaxBytesError`.

- [ ] **Step 1: Write failing declared-length and streaming tests**

Use this test adapter and assertions:

```go
type roundTripperFunc func(*http.Request) (*http.Response, error)
func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestLimitResponsesRejectsChunkedBody(t *testing.T) {
	base := roundTripperFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, ContentLength: -1,
			Body: io.NopCloser(strings.NewReader("12345"))}, nil
	})
	req := httptest.NewRequest(http.MethodGet, "http://example.test", nil)
	resp, err := LimitResponses(base, 4).RoundTrip(req)
	if err != nil { t.Fatal(err) }
	_, err = io.ReadAll(resp.Body)
	if !errors.Is(err, ErrResponseTooLarge) {
		t.Fatalf("read error = %v, want ErrResponseTooLarge", err)
	}
	var maxErr *http.MaxBytesError
	if !errors.As(err, &maxErr) {
		t.Fatalf("read error = %T %v, want *http.MaxBytesError", err, err)
	}
}
```

Add `TestLimitResponsesRejectsDeclaredLength` with `ContentLength: 5`, limit 4, and a test
`ReadCloser` that records `Close`; assert `errors.Is(err, ErrResponseTooLarge)`,
`errors.As(err, *ResponseTooLargeError)`, and `closed == true`.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/httptransport -run TestLimitResponses -v
```

Expected: build failure because `LimitResponses` does not exist.

- [ ] **Step 3: Implement the minimal transport**

```go
package httptransport

import (
	"io"
	"net/http"
	"strconv"
	"github.com/go-errors/errors"
)

type responseLimitTransport struct { base http.RoundTripper; limit int64 }

var ErrResponseTooLarge = errors.New("http response body too large")

type ResponseTooLargeError struct {
	Limit int64
	Cause error
}

func (e *ResponseTooLargeError) Error() string {
	return "http response body exceeds " + strconv.FormatInt(e.Limit, 10) + " bytes"
}
func (e *ResponseTooLargeError) Unwrap() error { return e.Cause }
func (e *ResponseTooLargeError) Is(target error) bool { return target == ErrResponseTooLarge }

type responseLimitReadCloser struct {
	io.ReadCloser
	limit int64
}

func (r *responseLimitReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		return n, &ResponseTooLargeError{Limit: r.limit, Cause: err}
	}
	return n, err
}

func LimitResponses(base http.RoundTripper, limit int64) http.RoundTripper {
	if base == nil { base = http.DefaultTransport }
	if limit <= 0 { panic("httptransport: response limit must be positive") }
	return &responseLimitTransport{base: base, limit: limit}
}

func (t *responseLimitTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	resp, err := t.base.RoundTrip(req)
	if err != nil { return nil, err }
	if resp.ContentLength > t.limit {
		_ = resp.Body.Close()
		return nil, &ResponseTooLargeError{Limit: t.limit}
	}
	resp.Body = &responseLimitReadCloser{
		ReadCloser: http.MaxBytesReader(nil, resp.Body, t.limit),
		limit: t.limit,
	}
	return resp, nil
}
```

- [ ] **Step 4: Run GREEN and commit**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/httptransport -v
git add internal/httptransport
git commit -m "feat(http): bound generated client responses"
```

Expected: both tests pass with pristine output.

### Task 4: Apply Response Limits to 3F and RFQ Clients

**Files:**
- Modify: `internal/solvers/bridgefacilitator/apiclient.go`
- Modify: `internal/solvers/bridgefacilitator/apiclient_test.go`
- Modify: `internal/solvers/rfq/backend.go`
- Modify: `internal/solvers/rfq/backend_test.go`

**Interfaces:**
- Consumes: `httptransport.LimitResponses` from Task 3.
- Produces: both generated clients reject bodies over `8 << 20` bytes while preserving timeouts and RFQ path rewriting.

- [ ] **Step 1: Add real-client oversized-response tests**

For each package, use an `httptest.Server` that writes a valid response prefix followed by more than
`maxGeneratedResponseBytes`. Call `listAuctions` and `listOpenOrders`, respectively, and assert the
shared typed boundary survives the generated-client layer:

```go
if !errors.Is(err, httptransport.ErrResponseTooLarge) {
	t.Fatalf("error = %v, want ErrResponseTooLarge", err)
}
```

In one case flush headers without a `Content-Length` so the streaming limit is exercised.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/solvers/bridgefacilitator ./internal/solvers/rfq -run 'Test.*OversizedResponse' -v
```

Expected: the bounded-error assertion fails because the clients currently read without a cap.

- [ ] **Step 3: Compose the limiter around current transports**

Add in both packages:

```go
const maxGeneratedResponseBytes = 8 << 20
```

Wire 3F:

```go
cfg.HTTPClient = &http.Client{
	Timeout: timeout,
	Transport: httptransport.LimitResponses(http.DefaultTransport, maxGeneratedResponseBytes),
}
```

Wire RFQ:

```go
cfg.HTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: httptransport.LimitResponses(
		internalDiscountTransport{base: http.DefaultTransport}, maxGeneratedResponseBytes),
}
```

Do not modify generated files.

- [ ] **Step 4: Run GREEN and commit**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/solvers/bridgefacilitator ./internal/solvers/rfq -run 'Test(APIClient|Backend|.*OversizedResponse)' -v
git add internal/solvers/bridgefacilitator/apiclient.go internal/solvers/bridgefacilitator/apiclient_test.go internal/solvers/rfq/backend.go internal/solvers/rfq/backend_test.go
git commit -m "fix(api): reject oversized generated responses"
```

Expected: oversized and existing client transport tests pass.

### Task 5: Preflight Every RPC Endpoint and Redact Labels

**Files:**
- Modify: `internal/chain/chain.go`
- Modify: `internal/chain/fallback.go`
- Modify: `internal/chain/fallback_test.go`
- Modify: `cmd/vault-solver/run.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `config/3f.example.yaml`
- Modify: `docs/3F-PLAN.md`
- Modify: `docs/RFQ-PLAN.md`

**Interfaces:**
- Produces: `func Dial(ctx context.Context, rpcURLs []string, writeRPCURL, multicallAddr string, expectedChainID uint64, log logr.Logger) (*Client, error)`.
- Produces: startup rejects any unreachable/wrong-chain primary, fallback, or distinct write endpoint.
- Removes: generic YAML `chain.wsUrl`.

- [ ] **Step 1: Add failing preflight and redaction tests**

Use the existing JSON-RPC test server helpers to cover:

```go
_, err := Dial(ctx,
	[]string{primary.URL + "/primary?key=one", wrong.URL + "/secret/path?apiKey=two"},
	write.URL+"/relay/private?token=three", multicall, 1, log)
if err == nil { t.Fatal("expected wrong-chain endpoint rejection") }
if strings.Contains(logs.String(), "secret/path") || strings.Contains(logs.String(), "apiKey") ||
	strings.Contains(err.Error(), "token=three") {
	t.Fatalf("endpoint secret leaked: err=%v logs=%s", err, logs.String())
}
```

Add cases for healthy primary + wrong fallback, wrong write RPC, and all endpoints matching. Add
unreachable and malformed URLs containing userinfo, a secret path, query, and fragment; assert none of
those substrings appears in either the returned error or captured logs. Exercise a runtime fallback
where every endpoint fails and apply the same assertion. Add a config fixture containing `wsUrl` and
require strict-decode failure.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/chain ./internal/config -run 'TestDial.*Chain|Test.*Endpoint.*Redact|Test.*WSURL' -v
```

Expected: old `Dial` signature/behavior fails the new tests and `wsUrl` still decodes.

- [ ] **Step 3: Add origin-only endpoint labels**

```go
func endpointLabel(u *url.URL) string {
	if u == nil || u.Scheme == "" || u.Host == "" { return "invalid endpoint" }
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}
```

Use only this label plus an ordinal in logs/errors. URL parse errors must name the endpoint index
without quoting or wrapping the raw URL. Update `parseHTTPEndpoints`, `dialClient`, and
`fallbackTransport.RoundTrip` as part of this boundary:

- never log `ep.Redacted()` because it retains path/query/fragment;
- never log `lastErr.Error()` from an HTTP transport;
- never return a `%w` chain whose underlying `url.Error` can render the raw URL;
- report only safe classes such as `invalid endpoint`, `unsupported scheme`, `transport failure`,
  `HTTP 503`, or `chain-id request failed`; and
- preserve the safe endpoint ordinal/origin in the outer error so operators can identify the
  configured endpoint without exposing credentials or routing tokens.

This is an intentional security-boundary exception to normal cause wrapping: retain the cause only
inside a private classification decision, never in a returned/logged error string.

- [ ] **Step 4: Implement strict chain-ID preflight**

Change `Dial` to accept `expectedChainID`. De-duplicate full URLs internally, but preflight every distinct read URL and a distinct write URL with:

```go
func validateEndpointChainID(ctx context.Context, raw string, expected *big.Int, log logr.Logger) error {
	ec, err := dialClient(ctx, []string{raw}, log)
	if err != nil { return errors.New("dial failed") }
	defer ec.Close()
	got, err := ec.ChainID(ctx)
	if err != nil { return errors.New("chain-id request failed") }
	if got.Cmp(expected) != 0 {
		return errors.Errorf("chain id mismatch: got %s, want %s", got, expected)
	}
	return nil
}
```

Wrap only these sanitized failures with the safe endpoint label. Ensure the final client-construction
dial follows the same sanitization even though preflight already passed. Cache `expected` in
`Client.chainID` and remove the redundant post-dial chain-ID comparison from `run.go`.

- [ ] **Step 5: Remove generic `chain.wsUrl`**

Delete:

```go
WSURL string `yaml:"wsUrl,omitempty"`
```

Remove it from the 3F example and 3F/RFQ plan text. Keep OEV's solver-local `ws.url`.

- [ ] **Step 6: Run GREEN and commit**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/chain ./internal/config ./cmd/vault-solver -v
git add internal/chain/chain.go internal/chain/fallback.go internal/chain/fallback_test.go cmd/vault-solver/run.go internal/config/config.go internal/config/config_test.go config/3f.example.yaml docs/3F-PLAN.md docs/RFQ-PLAN.md
git commit -m "fix(chain): validate every configured RPC endpoint"
```

Expected: endpoint, fallback, redaction, strict-config, and command wiring tests pass.

### Task 6: Add Pinned-Block Multicall

**Files:**
- Modify: `internal/chain/chain.go`
- Modify: `internal/chain/fallback_test.go`

**Interfaces:**
- Produces: `func (c *Client) MulticallAt(ctx context.Context, calls []Call, blockNumber *big.Int) ([]CallResult, error)`.
- Preserves: `Multicall(ctx, calls)` as latest-block shorthand.

- [ ] **Step 1: Add a failing real-RPC block-tag test**

Capture the second `eth_call` parameter in the existing JSON-RPC server and assert:

```go
_, err := client.MulticallAt(ctx, []Call{{Target: target, Data: []byte{1}}}, big.NewInt(123))
if err != nil { t.Fatal(err) }
if gotBlockTag != "0x7b" { t.Fatalf("block tag = %q, want 0x7b", gotBlockTag) }
```

Call `Multicall` separately and assert `latest`.

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/chain -run TestMulticallAtUsesBlockTag -v
```

Expected: build failure because `MulticallAt` does not exist.

- [ ] **Step 3: Move existing logic behind the new method**

```go
func (c *Client) Multicall(ctx context.Context, calls []Call) ([]CallResult, error) {
	return c.MulticallAt(ctx, calls, nil)
}

func (c *Client) MulticallAt(ctx context.Context, calls []Call, blockNumber *big.Int) ([]CallResult, error) {
	in := make([]multicall3.Multicall3Call3, len(calls))
	for i, call := range calls {
		in[i] = multicall3.Multicall3Call3{
			Target: call.Target, AllowFailure: call.AllowFailure, CallData: call.Data,
		}
	}
	data := multicallB.PackAggregate3(in)
	ret, err := c.CallContract(ctx, ethereum.CallMsg{To: &c.multicall, Data: data}, blockNumber)
	if err != nil {
		return nil, errors.Errorf("chain: multicall aggregate3: %w", err)
	}
	out, err := multicallB.UnpackAggregate3(ret)
	if err != nil {
		return nil, errors.Errorf("chain: multicall unpack aggregate3: %w", err)
	}
	res := make([]CallResult, len(out))
	for i, o := range out {
		res[i] = CallResult{Success: o.Success, ReturnData: o.ReturnData}
	}
	return res, nil
}
```

- [ ] **Step 4: Run GREEN and commit**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/chain -v
git add internal/chain/chain.go internal/chain/fallback_test.go
git commit -m "feat(chain): support pinned-block multicalls"
```

Expected: pinned/latest tag tests and all existing chain tests pass.

### Task 7: Reject Unsafe EIP-1559 Fee Configuration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/txmanager/txmanager.go`
- Modify: `internal/txmanager/txmanager_test.go`
- Modify: `config/3f.example.yaml`
- Modify: `config/rfq.example.yaml`

**Interfaces:**
- Produces: finite non-negative fee validation and an explicit error when a suggested tip exceeds an explicit max fee.
- Removes: silent `maxFee = tip` cap raising.

- [ ] **Step 1: Add failing validation and runtime tests**

Add YAML table rows for `.nan`, `.inf`, `-.inf`, `-1`, and:

```yaml
txManager: {maxFeeGwei: 1, tipGwei: 2}
```

Each must fail with the relevant field name. Add a txmanager test whose backend suggests 3 gwei while max fee is 2 gwei:

```go
_, _, err := m.fees(context.Background())
if err == nil || !strings.Contains(err.Error(), "suggested tip") {
	t.Fatalf("fees error = %v, want suggested-tip-over-cap", err)
}
```

- [ ] **Step 2: Run RED**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test ./internal/config ./internal/txmanager -run 'Test.*(Fee|Gwei|Tip)' -v
```

Expected: unsafe values validate or max fee is silently raised, so tests fail.

- [ ] **Step 3: Add config validation**

```go
func validateGwei(name string, value float64) error {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return errors.Errorf("txManager.%s must be finite and >= 0", name)
	}
	return nil
}
```

Validate both fields, then enforce:

```go
if c.TxManager.MaxFeeGwei > 0 && c.TxManager.TipGwei > 0 &&
	c.TxManager.MaxFeeGwei < c.TxManager.TipGwei {
	return errors.New("txManager.maxFeeGwei must be >= txManager.tipGwei")
}
```

- [ ] **Step 4: Enforce the runtime cap**

Replace the silent raise with:

```go
if maxFee.Cmp(tip) < 0 {
	return nil, nil, errors.Errorf("suggested tip %s exceeds configured max fee %s", tip, maxFee)
}
```

- [ ] **Step 5: Document, run GREEN, and commit**

Document finite/non-negative values and the hard `max >= tip` invariant in both examples.

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/config ./internal/txmanager -v
git add internal/config/config.go internal/config/config_test.go internal/txmanager/txmanager.go internal/txmanager/txmanager_test.go config/3f.example.yaml config/rfq.example.yaml
git commit -m "fix(txmanager): enforce configured fee caps"
```

Expected: all config and fee-selection tests pass.

### Task 8: Verify the Generic Foundation

**Files:**
- Verify only; do not add unrelated cleanup.

**Interfaces:**
- Produces: a stable dependency base for transaction, RFQ, OEV, and 3F work.

- [ ] **Step 1: Run format, focused tests, build, and lint**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local golangci-lint run --fix
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go test -race ./internal/chain ./internal/config ./internal/httptransport ./internal/txmanager ./internal/solvers/bridgefacilitator ./internal/solvers/rfq
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local go build ./...
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local golangci-lint run
```

Expected: all commands exit 0 with no lint findings.

- [ ] **Step 2: Verify generation and scope**

```bash
PATH=/tmp/codex-go1.26.5/go/bin:$PATH GOTOOLCHAIN=local make check-generated
git status --short
git log --oneline --max-count=7
```

Expected: generated output and worktree are clean; commits map only to findings 7, 10, 11, 17, and 19 plus `MulticallAt` support required by finding 13.
