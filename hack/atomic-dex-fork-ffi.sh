#!/bin/bash
set -euo pipefail
# Test-only bridge. The Go binary builds a quote, signatures and calldata from fork facts;
# Forge executes those bytes in its own local EVM. There is no send-RPC in this script.
root="$(cd "$(dirname "$0")/.." && pwd)"
if [[ $# != 2 ]]; then exit 2; fi
if ! ATOMIC_DEX_FFI_INPUT="$1" ATOMIC_DEX_FFI_OUTPUT="$2" \
  "$root/.tools/atomic-dex-fork-tests" -test.run '^TestDexForkBridge$' > "$2.log" 2>&1; then
  cat "$2.log" >&2
  exit 1
fi
cat "$2"
