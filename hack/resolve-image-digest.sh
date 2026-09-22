#!/usr/bin/env bash
set -euo pipefail

image=${1:?usage: resolve-image-digest.sh IMAGE}
inspect_error=$(mktemp)
trap 'rm -f "$inspect_error"' EXIT

if digest=$(docker buildx imagetools inspect "$image" --format '{{.Manifest.Digest}}' 2>"$inspect_error"); then
  if [[ ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
    printf 'Invalid registry digest for %s: %s\n' "$image" "$digest" >&2
    exit 1
  fi
  printf 'digest=%s\n' "$digest"
elif [[ "$(<"$inspect_error")" == "ERROR: $image: not found" ]]; then
  # Only a missing tag permits a build; authentication and registry failures must stop the job.
  printf 'digest=\n'
else
  cat "$inspect_error" >&2
  exit 1
fi
