#!/usr/bin/env bash
# scripts/oev/oev-balance.sh — full "where is the money" balance sheet + rebalance for the RedStone OEV
# Sepolia testbed, so an e2e run is repeatable: see every pool, then restore the ones a liquidation
# drains. Complements scripts/oev/oev-testrun.sh (which drives the bot) and RedStone's harness (positions/feed).
#
# Why this exists: on the testnet the LiquidLane Account is a STUB — it values seized RWA but never
# redeems it. So each liquidation
#   • drains the vault's freeAssets (TLOAN fronted to repay Morpho — never replenished by redemption),
#   • drains the callback's native ETH (payBid, 0.0005/bid),
#   • drains the Executor deposit (gas liability; below the floor the bot self-stops and won't bid),
#   • grows the callback's TLOAN (retained profit) and the Account's TCOL (seized, unredeemed).
# `rebalance` recycles the retained profit back into the vault (simulating the missing redemption) and
# tops the ETH pools back up — all signed by the owner key — then defers positions to RedStone's reset.
#
# Reads need only an RPC. Writes need the owner key (OEV_SIGNER_PRIVATE_KEY == manifest `owner`).
# Default action is the read-only sheet; every write is an explicit subcommand. Addresses come from the
# committed manifest (scripts/oev/addresses.sepolia.json) — the single source of truth, no hardcoding.
#
# Usage:
#   ETH_RPC_URL_SEPOLIA=https://… scripts/oev/oev-balance.sh [sheet]      # read-only balance sheet (default)
#   …  scripts/oev/oev-balance.sh topup-callback [ETH]   # send ETH to the callback (payBid fuel)
#   …  scripts/oev/oev-balance.sh recycle [TLOAN]        # sweep callback profit → vault freeAssets
#   …  scripts/oev/oev-balance.sh topup-deposit [ETH]    # top up the Executor gas deposit (guarded)
#   …  scripts/oev/oev-balance.sh setup-callback         # authorize + fund a new no-preview callback
#   …  scripts/oev/oev-balance.sh reset                  # re-arm positions (delegates to RedStone harness)
#   …  scripts/oev/oev-balance.sh rebalance              # recycle + topup-callback (+deposit, +RESET=1 reset)
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
MANIFEST="${OEV_MANIFEST:-$(dirname "$0")/addresses.sepolia.json}"
CONFIG="${OEV_CONFIG:-$ROOT/config/redstone-oev.sepolia.example.yaml}"
HARNESS="${OEV_HARNESS:-/tmp/symbiotic/symbiotic}"
RPC="${ETH_RPC_URL_SEPOLIA:-${OEV_LIVE_RPC:-${RPC:-}}}"

# Rebalance targets (override via env). Deposit default is kept above the bot's pre-bid floor.
TARGET_CALLBACK_ETH="${TARGET_CALLBACK_ETH:-0.05}"
TARGET_DEPOSIT_ETH="${TARGET_DEPOSIT_ETH:-0.06}"
KEEP_PROFIT_TLOAN="${KEEP_PROFIT_TLOAN:-0}"   # TLOAN to leave in the callback when recycling

command -v cast >/dev/null || { echo "need foundry 'cast' on PATH (https://getfoundry.sh)" >&2; exit 1; }
command -v jq   >/dev/null || { echo "need 'jq' on PATH" >&2; exit 1; }
[ -f "$MANIFEST" ] || { echo "manifest not found: $MANIFEST" >&2; exit 1; }
[ -n "$RPC" ] || { echo "set ETH_RPC_URL_SEPOLIA (Sepolia RPC URL)" >&2; exit 1; }

m() { jq -r "$1" "$MANIFEST"; }
OWNER=$(m .owner)
EXECUTOR=$(m .external.redstoneExecutor)
MORPHO=$(m .external.morpho)
MARKET=$(m .external.morphoMarket)
ORACLE=$(m .external.morphoOracle)
FEED=$(m .external.collateralFeed)
TLOAN=$(m .external.tloan)
TCOL=$(m .external.tcol)
VAULT=$(m .instance.vault)
ADAPTER=$(m .instance.adapter)
ACCOUNT=$(m .instance.account)
CALLBACK=$(m .instance.callback)
# shellcheck disable=SC2207  # addresses are whitespace-free; word-split into an array (bash 3.2: no mapfile)
POSITIONS=( $(m '.external.testPositions[]') )

# Read a numeric scalar (int or decimal) from the bot config — e.g. `ynum bidEth`. Tolerant: a missing key
# yields empty (not a grep exit-1 that would abort the whole sheet under set -e + pipefail).
ynum() { grep -oE "$1:[[:space:]]*\"?[0-9]+(\.[0-9]+)?" "$CONFIG" | grep -oE '[0-9.]+' | tail -1 || true; }

# --- chain read helpers (tolerant: empty on revert, never abort the sheet) ----------------------
call() { cast call "$@" --rpc-url "$RPC" 2>/dev/null | awk 'NR==1{print $1}' || true; }
bal()  { cast balance "$1" --rpc-url "$RPC" 2>/dev/null | awk '{print $1}' || true; }
# Pipe-processed reads as named functions, so bg() can fan them out like call/bal.
read_feed() { cast call "$FEED" 'latestRoundData()(uint80,int256,uint256,uint256,uint80)' --rpc-url "$RPC" 2>/dev/null | awk 'NR==2{print $1}' || true; }
read_pos()  { cast call "$MORPHO" 'position(bytes32,address)(uint256,uint128,uint128)' "$MARKET" "$1" --rpc-url "$RPC" 2>/dev/null | awk '{print $1}' || true; }
# bg <key> <cmd...> — run a read concurrently; its stdout is captured to $RD/<key> (RD set by sheet).
bg() { local k="$1"; shift; ( "$@" >"$RD/$k" ) & }
# fmt <wei> <divisor> <precision> — the one numeric formatter behind the named units (n/a on empty).
fmt()  { awk -v w="${1:-}" -v d="$2" -v p="$3" 'BEGIN{ if(w=="")print "n/a"; else printf "%.*f", p, w/d }'; }
eth()  { fmt "${1:-}" 1e18 6; }
t6()   { fmt "${1:-}" 1e6  4; }
t18()  { fmt "${1:-}" 1e18 4; }
usd()  { fmt "${1:-}" 1e24 2; }
row()  { printf "  %-22s %s\n" "$1" "$2"; }

sheet() {
  # config-derived thresholds the warnings compare against (only the sheet needs them).
  local MIN_DEPOSIT BID_ETH BID_WEI
  # The bot bids when the Executor deposit ≥ MIN_DEPOSIT (solver.go minDeposit=1e13). Gas is debited from
  # the deposit post-settlement, independent of the auction, and not pre-reserved — so there is no gas floor.
  MIN_DEPOSIT=10000000000000
  BID_ETH=$(ynum bidEth); BID_ETH=${BID_ETH:-0.0005}
  BID_WEI=$(cast to-wei "$BID_ETH" ether)

  local RD i eoaEth depWei nonce locked cbEth cbLoan vFree vTotal rate maxAssets acTcol acAssets price feed
  RD=$(mktemp -d)
  # Fan out the independent reads concurrently — one wave instead of ~15 serial RPC round-trips.
  bg eoaEth    bal  "$OWNER"
  bg depWei    call "$EXECUTOR" 'deposits(address)(uint256)' "$OWNER"
  bg nonce     call "$EXECUTOR" 'nonces(address)(uint256)' "$OWNER"
  bg locked    call "$EXECUTOR" 'locked(address)(bool)' "$OWNER"
  bg cbEth     bal  "$CALLBACK"
  bg cbLoan    call "$TLOAN" 'balanceOf(address)(uint256)' "$CALLBACK"
  bg vFree     call "$VAULT" 'freeAssets()(uint256)'
  bg vTotal    call "$VAULT" 'totalAssets()(uint256)'
  bg rate      call "$ADAPTER" 'getMaxRate(address)(uint256)' "$TCOL"
  bg maxAssets call "$ADAPTER" 'getMaxAssets(address)(uint256)' "$TCOL"
  bg acTcol    call "$TCOL" 'balanceOf(address)(uint256)' "$ACCOUNT"
  bg acAssets  call "$ACCOUNT" 'totalAssets()(uint256)'
  bg price     call "$ORACLE" 'price()(uint256)'
  bg feed      read_feed
  for i in "${!POSITIONS[@]}"; do bg "pos$i" read_pos "${POSITIONS[$i]}"; done
  wait
  eoaEth=$(cat "$RD/eoaEth"); depWei=$(cat "$RD/depWei");     nonce=$(cat "$RD/nonce");   locked=$(cat "$RD/locked")
  cbEth=$(cat "$RD/cbEth");   cbLoan=$(cat "$RD/cbLoan");     vFree=$(cat "$RD/vFree");   vTotal=$(cat "$RD/vTotal")
  rate=$(cat "$RD/rate");     maxAssets=$(cat "$RD/maxAssets"); acTcol=$(cat "$RD/acTcol"); acAssets=$(cat "$RD/acAssets")
  price=$(cat "$RD/price");   feed=$(cat "$RD/feed")

  echo "════════════════════════ OEV money balance sheet (Sepolia) ════════════════════════"
  echo "  market price (oracle):  \$$(usd "$price")   feed: \$$(fmt "${feed:-}" 1e8 2)"
  echo "── SIGNER / EXECUTOR  ($OWNER)"
  row "EOA balance:"      "$(eth "$eoaEth") ETH"
  row "Executor deposit:" "$(eth "$depWei") ETH   (MIN_DEPOSIT $(eth "$MIN_DEPOSIT"))"
  row "Executor nonce:"   "${nonce:-n/a}   locked: ${locked:-n/a}"
  echo "── CALLBACK  ($CALLBACK)"
  row "native (payBid):"  "$(eth "$cbEth") ETH   (~$(awk -v c="${cbEth:-0}" -v b="$BID_WEI" 'BEGIN{printf "%d", (b>0)?c/b:0}') bids at $BID_ETH ETH)"
  row "TLOAN (profit):"   "$(t6 "$cbLoan") TLOAN  ← recyclable into the vault"
  echo "── VAULT  ($VAULT)"
  row "freeAssets:"       "$(t6 "$vFree") TLOAN   (deployable liquidity)"
  row "totalAssets:"      "$(t6 "$vTotal") TLOAN"
  echo "── ADAPTER  ($ADAPTER)"
  row "getMaxRate(TCOL):" "$(t18 "$rate") TLOAN/TCOL  (RWA sell price, net discount)"
  row "getMaxAssets:"     "$(t6 "$maxAssets") TLOAN   (per-swap liquidity cap)"
  echo "── ACCOUNT  ($ACCOUNT)  [stub: values, does NOT redeem]"
  row "TCOL held (seized):" "$(t18 "$acTcol") TCOL   (accumulates unredeemed)"
  row "totalAssets (valued):" "$(t6 "$acAssets") TLOAN"
  echo "── POSITIONS  (market $MARKET)"
  local b pos coll bshares
  for i in "${!POSITIONS[@]}"; do
    b="${POSITIONS[$i]}"
    # position() → (supplyShares, borrowShares, collateral), one field per line (read above); take 2nd, 3rd.
    # shellcheck disable=SC2207  # whitespace-free fields → array
    pos=( $(cat "$RD/pos$i") )
    bshares="${pos[1]:-}"; coll="${pos[2]:-}"
    row "${b:0:10}…" "collateral $(t18 "$coll") TCOL   borrowShares ${bshares:-n/a}"
  done

  # --- warnings: what blocks the next e2e run ---
  echo "──────────────────────────────────────────────────────────────────────────────────"
  # Exact integer comparisons (all values are sub-ETH/uint128 wei, well within bash's 64-bit ints — no
  # awk float rounding). A failed read comes back empty; report THAT distinctly rather than treating it
  # as a passing threshold (a flaky read must never print the green "ready" banner).
  local warned=0
  if [ -z "$depWei" ]; then
    echo "  ⚠ could not read Executor deposit (RPC error) — cannot confirm the bot will bid"; warned=1
  elif [ "$depWei" -lt "$MIN_DEPOSIT" ]; then
    echo "  ⚠ Executor deposit < MIN_DEPOSIT ($(eth "$MIN_DEPOSIT") ETH) — the bot will NOT bid. Fix: topup-deposit"; warned=1
  fi
  if [ -z "$cbEth" ]; then
    echo "  ⚠ could not read callback native balance (RPC error)"; warned=1
  elif [ "$cbEth" -lt "$BID_WEI" ]; then
    echo "  ⚠ callback native < one bid ($BID_ETH ETH) — payBid would revert. Fix: topup-callback"; warned=1
  fi
  if [ -z "$vFree" ]; then
    echo "  ⚠ could not read vault freeAssets (RPC error)"; warned=1
  elif [ "$vFree" -eq 0 ]; then
    echo "  ⚠ vault freeAssets = 0 — no liquidity to front a swap. Fix: recycle (or RedStone mint+deposit)"; warned=1
  fi
  [ "$warned" = 0 ] && echo "  ✓ all pools above their thresholds — ready for an e2e run"
  echo "════════════════════════════════════════════════════════════════════════════════════"
  rm -rf "$RD"
}

# --- writes (owner key required) ----------------------------------------------------------------
# CAVEAT: writes must go through a RELAYING RPC. The public Alchemy Sepolia endpoint accepts txs into a
# private pool without relaying them (they silently never land); point ETH_RPC_URL_SEPOLIA at a public
# relay for writes, e.g. https://ethereum-sepolia-rpc.publicnode.com (docs/OEV-PLAN.md §6.7).
need_key() {
  : "${OEV_SIGNER_PRIVATE_KEY:?set OEV_SIGNER_PRIVATE_KEY (the owner key) for write ops}"
  local from lc_from lc_owner
  from=$(cast wallet address --private-key "$OEV_SIGNER_PRIVATE_KEY")
  lc_from=$(printf '%s' "$from"  | tr 'A-Z' 'a-z')
  lc_owner=$(printf '%s' "$OWNER" | tr 'A-Z' 'a-z')
  if [ "$lc_from" != "$lc_owner" ]; then
    echo "key address $from != manifest owner $OWNER — refusing to send" >&2; exit 1
  fi
  SEND=(cast send --private-key "$OEV_SIGNER_PRIVATE_KEY" --rpc-url "$RPC")
}

# topup_delta <label> <target-wei> <current-wei> — echo the wei delta needed to reach target, or return
# 1 (with a stderr note) when there's nothing to send. FAIL CLOSED on an unreadable current balance: a
# tolerant read returns "" on RPC error, and treating that as 0 would send the FULL target on a transient
# blip. A genuine zero balance reads as "0" (non-empty), so it still tops up correctly.
topup_delta() {
  if [ -z "${3:-}" ]; then echo "$1: could not read current balance (RPC error) — refusing to send" >&2; return 1; fi
  local d=$(( $2 - $3 ))
  if [ "$d" -le 0 ]; then echo "$1 already $(eth "$3") ETH ≥ target $(eth "$2") — nothing to do" >&2; return 1; fi
  echo "$d"
}

topup_callback() {
  need_key
  local target delta
  target=$(cast to-wei "${1:-$TARGET_CALLBACK_ETH}" ether)
  delta=$(topup_delta "callback" "$target" "$(bal "$CALLBACK")") || return 0
  echo "→ sending $(eth "$delta") ETH to callback $CALLBACK"
  "${SEND[@]}" --value "$delta" "$CALLBACK" >/dev/null
  echo "  callback native now $(eth "$(bal "$CALLBACK")") ETH"
}

recycle() {
  need_key
  local cbLoan amt
  cbLoan=$(call "$TLOAN" 'balanceOf(address)(uint256)' "$CALLBACK")
  # Require a plain non-negative integer: an empty (RPC error) or non-decimal (e.g. 0x-prefixed) read must
  # not flow into the 64-bit `$(( ))` below where it could under/overflow into a wrong withdraw amount.
  case "$cbLoan" in ''|*[!0-9]*) echo "recycle: unreadable callback TLOAN balance (\"$cbLoan\") — aborting" >&2; return 0;; esac
  if [ -n "${1:-}" ]; then amt=$(cast to-wei "$1" mwei); else amt=$(( cbLoan - $(cast to-wei "$KEEP_PROFIT_TLOAN" mwei) )); fi
  if [ "$amt" -le 0 ] || [ "$amt" -gt "$cbLoan" ]; then echo "no recyclable profit in callback (have $(t6 "$cbLoan") TLOAN, keep $KEEP_PROFIT_TLOAN)"; return; fi
  echo "→ recycling $(t6 "$amt") TLOAN: callback → owner → vault.deposit (replenishes freeAssets)"
  # withdrawERC20 is the callback's owner-only sweep (source: rfq-integration/src/oev — not a
  # repo-verified ABI); approve + deposit are standard ERC-20 / ERC-4626.
  "${SEND[@]}" "$CALLBACK" 'withdrawERC20(address,address,uint256)' "$TLOAN" "$OWNER" "$amt" >/dev/null
  "${SEND[@]}" "$TLOAN" 'approve(address,uint256)' "$VAULT" "$amt" >/dev/null
  "${SEND[@]}" "$VAULT" 'deposit(uint256,address)' "$amt" "$OWNER" >/dev/null
  echo "  vault freeAssets now $(t6 "$(call "$VAULT" 'freeAssets()(uint256)')") TLOAN"
}

topup_deposit() {
  need_key
  local target delta sig
  target=$(cast to-wei "${1:-$TARGET_DEPOSIT_ETH}" ether)
  delta=$(topup_delta "Executor deposit" "$target" "$(call "$EXECUTOR" 'deposits(address)(uint256)' "$OWNER")") || return 0
  sig="${OEV_DEPOSIT_SIG:-deposit()}" # verified: Executor.deposit() is payable + credits msg.sender (== our signer)
  echo "→ would add $(eth "$delta") ETH to the Executor deposit via: $EXECUTOR \"$sig\" --value $delta"
  # NOT a bare transfer: the Executor's receive() accumulates bidPaid, so a value-only send is lost, not
  # credited — always call deposit(). Still guarded behind an explicit opt-in (it moves funds).
  if [ "${OEV_APPLY_DEPOSIT:-0}" != "1" ]; then
    echo "  (guarded) set OEV_APPLY_DEPOSIT=1 to send"; return
  fi
  "${SEND[@]}" --value "$delta" "$EXECUTOR" "$sig" >/dev/null
  echo "  Executor deposit now $(eth "$(call "$EXECUTOR" 'deposits(address)(uint256)' "$OWNER")") ETH"
}

# Wire the no-preview OEV callback the bot depends on. The callback itself has no mutable allowlists; the
# single required live permission is LiquidLane's filler authorization.
setup_callback() {
  need_key
  echo "→ wiring callback $CALLBACK (owner ops)"
  "${SEND[@]}" "$ADAPTER"  'setFiller(address,bool)'          "$CALLBACK" true >/dev/null; echo "  adapter.isFiller[callback]=true"
  topup_callback
}

reset_positions() {
  [ -d "$HARNESS" ] || { echo "RedStone harness not found at $HARNESS (set OEV_HARNESS) — can't reset positions" >&2; exit 1; }
  echo "→ re-arming positions via RedStone harness ($HARNESS)"
  ( cd "$HARNESS" && npm run reset-positions )
}

rebalance() {
  recycle
  topup_callback
  topup_deposit
  [ "${RESET:-0}" = "1" ] && reset_positions
  echo; sheet
}

case "${1:-sheet}" in
  sheet)          sheet ;;
  topup-callback) topup_callback "${2:-}" ;;
  recycle)        recycle "${2:-}" ;;
  topup-deposit)  topup_deposit "${2:-}" ;;
  setup-callback) setup_callback ;;
  reset)          reset_positions ;;
  rebalance)      rebalance ;;
  *) echo "usage: $0 {sheet|topup-callback [ETH]|recycle [TLOAN]|topup-deposit [ETH]|setup-callback|reset|rebalance}" >&2; exit 2 ;;
esac
