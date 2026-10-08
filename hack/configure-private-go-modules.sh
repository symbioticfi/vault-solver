#!/usr/bin/env sh
# The helper reads the token from the process environment, never from a Git URL
# or stored credential. Run only in the ephemeral CI runner, not a user checkout.
set -eu
: "${MERKLE_SDK_READ_TOKEN:?Read access to merkle-executor is required}"
git config --global credential.https://github.com.helper '!f() { printf "username=x-access-token\npassword=%s\n" "$MERKLE_SDK_READ_TOKEN"; }; f'
