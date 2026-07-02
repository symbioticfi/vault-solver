#!/usr/bin/env bash
# Drive a live test liquidation on the RedStone Sepolia testbed by pairing RedStone's harness
# (reset / update-price / status / mint) with our solver. Read-then-act: it drops the oracle price to
# make the test positions underwater, runs the bot for a bounded window, then re-reads status so you
# can confirm a liquidation landed (collateral seized / debt reduced on a borrower).
#
# The script supplies the orchestration; keys/RPC/config stay in env only, never in the repo.
#
# Prereqs:
#   - RedStone harness checked out with its own .env (deployer + borrower keys):
#       OEV_HARNESS=/path/to/symbiotic        (default: /tmp/symbiotic/symbiotic)
#   - Our bot config tuned for the dev testbed (price + dry-run are env knobs, exported below —
#     OEV_ONCHAIN_PRICE_FOR_TEST=true, OEV_DRY_RUN unset → real bidding):
#       OEV_CONFIG=config/redstone-oev.sepolia.example.yaml
#   - Our bot secrets in env: OEV_SIGNER_PRIVATE_KEY, OEV_REDSTONE_API_KEY, ETH_RPC_URL_SEPOLIA
#   - Sepolia uses OEV_TEST_MONITOR=true because public api.morpho.org does not index this custom Morpho.
#   - One-time on-chain setup done (docs/OEV-PLAN.md §5–§6): the callback is wired
#     (scripts/oev/oev-balance.sh setup-callback), the signer holds an Executor deposit, and the callback +
#     vault are funded (scripts/oev/oev-balance.sh topup-deposit / topup-callback).
#
# Usage:
#   PRICE_USD=1550 RUN_SECONDS=120 scripts/oev/oev-testrun.sh
#   RESET=1 PRICE_USD=1550 scripts/oev/oev-testrun.sh        # reset positions to defaults first
set -euo pipefail

HARNESS="${OEV_HARNESS:-/tmp/symbiotic/symbiotic}"
CONFIG="${OEV_CONFIG:-config/redstone-oev.sepolia.example.yaml}"
MANIFEST="${OEV_MANIFEST:-$(dirname "$0")/addresses.sepolia.json}"
PRICE_USD="${PRICE_USD:-1550}"
RUN_SECONDS="${RUN_SECONDS:-120}"
RESET="${RESET:-0}"

[ -d "$HARNESS" ] || { echo "harness not found: $HARNESS (set OEV_HARNESS)" >&2; exit 1; }
[ -f "$CONFIG" ]  || { echo "config not found: $CONFIG (set OEV_CONFIG)" >&2; exit 1; }
[ -f "$MANIFEST" ] || { echo "manifest not found: $MANIFEST (set OEV_MANIFEST)" >&2; exit 1; }
: "${OEV_SIGNER_PRIVATE_KEY:?set OEV_SIGNER_PRIVATE_KEY (the bot signer / Executor-deposit wallet)}"
: "${OEV_REDSTONE_API_KEY:?set OEV_REDSTONE_API_KEY (RedStone WS key)}"
: "${ETH_RPC_URL_SEPOLIA:?set ETH_RPC_URL_SEPOLIA}"

# The Sepolia harness uses a test-only on-chain Morpho snapshot because public api.morpho.org does not
# index this custom test deployment.
export OEV_ONCHAIN_PRICE_FOR_TEST="${OEV_ONCHAIN_PRICE_FOR_TEST:-true}"
export OEV_TEST_MONITOR="${OEV_TEST_MONITOR:-true}"
export OEV_TEST_MARKETS="${OEV_TEST_MARKETS:-$(node -e 'const fs=require("fs"); const m=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); console.log(m.external.morphoMarket)' "$MANIFEST")}"
export OEV_TEST_POSITIONS="${OEV_TEST_POSITIONS:-$(node -e 'const fs=require("fs"); const m=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); console.log((m.external.testPositions||[]).join(","))' "$MANIFEST")}"

# Soft sanity — OEV_DRY_RUN must be off (the default) to actually send bids, not just log would-bids.
if [ "${OEV_DRY_RUN:-}" = "true" ] || [ "${OEV_DRY_RUN:-}" = "1" ]; then
  echo "⚠  OEV_DRY_RUN is set — the bot will only log would-bids; unset it to send bids" >&2
fi
# The harness scripts read their own .env (RPC + deployer/borrower keys) from $HARNESS.
run_harness() { ( cd "$HARNESS" && "$@" ); }

# Full money balance sheet (our Symbiotic side: deposit/callback/vault/account) — soft, needs `cast`.
money() { command -v cast >/dev/null && "$(dirname "$0")/oev-balance.sh" sheet || true; }
executor_nonce() {
  command -v cast >/dev/null || return 0
  local executor owner
  executor=$(node -e 'const fs=require("fs"); const m=JSON.parse(fs.readFileSync(process.argv[1],"utf8")); console.log(m.external.redstoneExecutor)' "$MANIFEST")
  owner=$(cast wallet address --private-key "$OEV_SIGNER_PRIVATE_KEY")
  cast call "$executor" 'nonces(address)(uint256)' "$owner" --rpc-url "$ETH_RPC_URL_SEPOLIA" 2>/dev/null || true
}

echo "▶ 0/5  harness deps"
[ -d "$HARNESS/node_modules" ] || run_harness npm install

if [ "$RESET" = "1" ]; then
  echo "▶ 1/5  reset positions to defaults (price \$2000, normalised LTV 98/92/85)"
  run_harness npm run reset-positions
fi

echo "▶ 2/5  drop oracle price to \$$PRICE_USD (raises every position's LTV → underwater)"
run_harness env NEW_PRICE_USD="$PRICE_USD" npm run update-price

echo "▶ 3/5  status BEFORE — note each borrower's liquidation price"
run_harness npm run status
money   # money BEFORE — deposit / callback fuel / vault liquidity
NONCE_BEFORE=$(executor_nonce)

echo "▶ 4/5  run the solver (binary) for ${RUN_SECONDS}s"
# Only settles via a RedStone auction (~every 16s on dev) while underwater at the ON-CHAIN price — keep
# the price low for the whole window (don't reset mid-run).
pkill -f "vault-solver run --config" 2>/dev/null || true   # reap an orphan from a previous run
BIN="${OEV_BIN:-bin/vault-solver}"
if [ -z "${OEV_BIN:-}" ]; then
  echo "  building $BIN"
  GOTOOLCHAIN=go1.26.4 go build -o "$BIN" ./cmd/vault-solver
elif [ ! -x "$BIN" ]; then
  echo "custom OEV_BIN is not executable: $BIN" >&2
  exit 1
fi
"$BIN" run --config "$CONFIG" &
BOT_PID=$!
sleep "$RUN_SECONDS"
kill "$BOT_PID" 2>/dev/null || true; sleep 2; kill -9 "$BOT_PID" 2>/dev/null || true
pkill -f "vault-solver run --config" 2>/dev/null || true
wait "$BOT_PID" 2>/dev/null || true

echo "▶ 5/5  status AFTER — reduced collateral/debt on a borrower ⇒ a liquidation landed"
run_harness npm run status
money   # money AFTER — diff vs BEFORE: deposit −gas, callback +profit/−bids, vault −fronted liquidity
NONCE_AFTER=$(executor_nonce)
if [ -n "${NONCE_BEFORE:-}" ] && [ -n "${NONCE_AFTER:-}" ] && [ "$NONCE_AFTER" = "$NONCE_BEFORE" ]; then
  echo "✘ no Executor nonce change detected ($NONCE_BEFORE → $NONCE_AFTER); no settlement landed in ${RUN_SECONDS}s" >&2
  exit 1
fi

echo "✔  done."
echo "   Restore the pools a liquidation drained (recycle profit → vault, refuel callback, re-arm):"
echo "     RESET=1 scripts/oev/oev-balance.sh rebalance          # then run again"
echo "   Or just re-arm positions:  RESET=1 PRICE_USD=$PRICE_USD scripts/oev/oev-testrun.sh"
# Success is confirmed from the harness BEFORE/AFTER status: reduced borrower collateral/debt means a
# liquidation landed. Increase RUN_SECONDS when the dev auction cadence is slow.
