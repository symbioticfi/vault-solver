#!/usr/bin/env bash
# scripts/oev/oev-fork-refuel.sh — local-only harness for the gas-deposit self-funding ORCHESTRATION
# (maybeRefuelDeposit). Sepolia has no TLOAN/ETH DEX, so the live testbed can't exercise the profit→ETH
# refuel; this forks Sepolia with anvil (treated as live), DEPLOYS a UniV2-style DEX onto the fork, wires
# the deployed callback to it, then runs the forklive Go test that drives the REAL bot refuel path
# (state read → swap profit→ETH via refuelGasDeposit → Executor.deposit). Each run starts a FRESH fork so
# it's deterministic.
#
# Needs: anvil + forge + cast (foundry), the rfq-integration repo (for the DexRouterStub), and
# ETH_RPC_URL_SEPOLIA + OEV_SIGNER_PRIVATE_KEY in env (or .env.local). NOT run in CI.
#
# Usage:  set -a; source .env.local; set +a;  scripts/oev/oev-fork-refuel.sh
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST="${OEV_MANIFEST:-$(dirname "$0")/addresses.sepolia.json}"
RFQ_DIR="${OEV_RFQ_DIR:-$ROOT/../rfq/rfq-integration}"
ANVIL_PORT="${ANVIL_PORT:-8545}"
ANVIL="http://127.0.0.1:${ANVIL_PORT}"
RPC="${ETH_RPC_URL_SEPOLIA:-${OEV_LIVE_RPC:-}}"
export FOUNDRY_DISABLE_NIGHTLY_WARNING=1

command -v anvil >/dev/null || { echo "need foundry 'anvil'" >&2; exit 1; }
command -v jq >/dev/null || { echo "need 'jq' on PATH" >&2; exit 1; }
[ -f "$MANIFEST" ] || { echo "manifest not found: $MANIFEST" >&2; exit 1; }
[ -n "$RPC" ] || { echo "set ETH_RPC_URL_SEPOLIA (a Sepolia RPC to fork)" >&2; exit 1; }
: "${OEV_SIGNER_PRIVATE_KEY:?set OEV_SIGNER_PRIVATE_KEY (owner==signer==keeper on the testbed)}"
[ -f "$RFQ_DIR/test/OevForkLive.t.sol" ] || { echo "rfq-integration not found at $RFQ_DIR (set OEV_RFQ_DIR)" >&2; exit 1; }

m() { jq -r "$1" "$MANIFEST"; }
CB=$(m .instance.callback)
TLOAN=$(m .external.tloan)

cleanup() { [ -n "${ANVIL_PID:-}" ] && kill "$ANVIL_PID" 2>/dev/null || true; }
trap cleanup EXIT

echo "▶ fresh anvil fork of Sepolia on :$ANVIL_PORT"
pkill -f "anvil .*--port ${ANVIL_PORT}" 2>/dev/null || true
anvil --fork-url "$RPC" --port "$ANVIL_PORT" --silent & ANVIL_PID=$!
for _ in $(seq 1 30); do cast chain-id --rpc-url "$ANVIL" >/dev/null 2>&1 && break; sleep 0.3; done
cast chain-id --rpc-url "$ANVIL" >/dev/null || { echo "anvil did not come up" >&2; exit 1; }

echo "▶ deploy + fund the DEX router on the fork"
ROUTER=$(cd "$RFQ_DIR" && forge create test/OevForkLive.t.sol:DexRouterStub \
  --rpc-url "$ANVIL" --private-key "$OEV_SIGNER_PRIVATE_KEY" --broadcast 2>/dev/null \
  | grep -iE "Deployed to" | grep -oE "0x[0-9a-fA-F]{40}" | head -1)
[ -n "$ROUTER" ] || { echo "DEX deploy failed" >&2; exit 1; }
[ "$(cast codesize "$ROUTER" --rpc-url "$ANVIL")" -gt 0 ] || { echo "DEX has no code at $ROUTER" >&2; exit 1; }
cast send "$ROUTER" 'set(uint256)' 400000000000000000000000000 --rpc-url "$ANVIL" --private-key "$OEV_SIGNER_PRIVATE_KEY" >/dev/null
cast rpc anvil_setBalance "$ROUTER" 0xDE0B6B3A7640000 --rpc-url "$ANVIL" >/dev/null # 1 ETH
echo "  router=$ROUTER (rate 4e26, 1 ETH)"

echo "▶ wire the callback for refuel (owner==signer)"
cast send "$CB" 'setApprovedRouter(address,bool)'        "$ROUTER" true --rpc-url "$ANVIL" --private-key "$OEV_SIGNER_PRIVATE_KEY" >/dev/null
cast send "$CB" 'setLoanReserveFloor(address,uint256)'   "$TLOAN" 10000000 --rpc-url "$ANVIL" --private-key "$OEV_SIGNER_PRIVATE_KEY" >/dev/null
cast send "$CB" 'setMaxSwapInPerCall(address,uint256)'   "$TLOAN" 50000000 --rpc-url "$ANVIL" --private-key "$OEV_SIGNER_PRIVATE_KEY" >/dev/null

echo "▶ run the forklive refuel orchestration test"
cd "$ROOT"
OEV_FORK_RPC="$ANVIL" OEV_FORK_ROUTER="$ROUTER" \
  GOTOOLCHAIN=go1.26.4 go test -tags forklive -run TestForkRefuelGasDeposit -v ./internal/solvers/redstoneoev/
