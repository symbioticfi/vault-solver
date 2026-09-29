#!/usr/bin/env bash
# Registers a LiquidLaneLifiExecutor as the LI.FI solver account for an API key (EIP-1271).
#
# Required env:
#   LIFI_API_KEY   LI.FI solver API key the executor is registered under
#   EXECUTOR       LiquidLaneLifiExecutor proxy address
#   RPC_URL        RPC for the executor's chain
#   CALLER_PRIVATE_KEY, or CAST_WALLET_ARGS (e.g. "--account kpk-caller", "--ledger"): a current executor caller
# Optional env:
#   LIFI_BASE_URL  default https://order.li.fi (use https://order-dev.li.fi for dev)
set -euo pipefail

: "${LIFI_API_KEY:?}" "${EXECUTOR:?}" "${RPC_URL:?}"
LIFI_BASE_URL="${LIFI_BASE_URL:-https://order.li.fi}"
if [ -z "${CAST_WALLET_ARGS:-}" ]; then
  : "${CALLER_PRIVATE_KEY:?set CALLER_PRIVATE_KEY or CAST_WALLET_ARGS}"
  CAST_WALLET_ARGS="--private-key ${CALLER_PRIVATE_KEY}"
fi

lifi() {
  curl -fsS -H "x-api-key: ${LIFI_API_KEY}" -H 'content-type: application/json' "$@"
}

# Prints true/false; used via assignment so a failed request aborts under set -e.
registered() {
  lifi "${LIFI_BASE_URL}/solver-api/solver/identities" |
    jq --arg a "${EXECUTOR}" 'any(.data[]; (.address | ascii_downcase) == ($a | ascii_downcase))'
}

state=$(registered)
if [ "${state}" = true ]; then
  echo "${EXECUTOR} is already registered for this API key"
  exit 0
fi

chain_id=$(cast chain-id --rpc-url "${RPC_URL}")
message=$(lifi "${LIFI_BASE_URL}/api/v1/solver/register/message" | jq -er .data.message)
message_hash=$(cast hash-message "${message}")
digest=$(cast call "${EXECUTOR}" 'lifiRegistrationDigest(bytes32)(bytes32)' "${message_hash}" --rpc-url "${RPC_URL}")
# shellcheck disable=SC2086 # CAST_WALLET_ARGS holds multiple flags
signature=$(cast wallet sign --no-hash ${CAST_WALLET_ARGS} "${digest}")

# LI.FI verifies exactly this call; fail before submitting if the signer is not a current caller.
magic=$(cast call "${EXECUTOR}" 'isValidSignature(bytes32,bytes)(bytes4)' "${message_hash}" "${signature}" --rpc-url "${RPC_URL}" || true)
if [ "${magic}" != "0x1626ba7e" ]; then
  echo "executor rejected the signature; is the signer in executor callers()?" >&2
  exit 1
fi

jq -n --arg m "${message}" --arg s "${signature}" --arg a "${EXECUTOR}" --arg c "eip155:${chain_id}" \
  '{message: $m, signature: $s, account: $a, chain: $c}' |
  lifi -X POST "${LIFI_BASE_URL}/api/v1/solver/register" -d @- >/dev/null

state=$(registered)
if [ "${state}" != true ]; then
  echo "registration submitted but ${EXECUTOR} is not listed in solver identities" >&2
  exit 1
fi
echo "registered ${EXECUTOR} on eip155:${chain_id}"
