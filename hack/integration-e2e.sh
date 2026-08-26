#!/usr/bin/env bash

set -euo pipefail

SOLVER_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INTEGRATION_DIR="${RFQ_INTEGRATION_DIR:-}"
PROFILE="${1:-}"

if [ -z "$INTEGRATION_DIR" ] || [ ! -x "$INTEGRATION_DIR/scripts/ci/vault-solver-e2e.sh" ]; then
  printf 'RFQ_INTEGRATION_DIR must point to an rfq-integration checkout\n' >&2
  exit 2
fi
case "$PROFILE" in
  3f|rfq|lifi|uniswapx|redstoneoev) ;;
  *)
    printf 'usage: RFQ_INTEGRATION_DIR=/path/to/rfq-integration %s <3f|rfq|lifi|uniswapx|redstoneoev>\n' "$0" >&2
    exit 2
    ;;
esac

export RFQ_SOLVER_SRC="$SOLVER_DIR"
exec "$INTEGRATION_DIR/scripts/ci/vault-solver-e2e.sh" "$PROFILE"
