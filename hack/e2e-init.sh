#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
HARNESS_REPOSITORY="https://github.com/symbioticfi/rfq-integration.git"
HARNESS_REVISION="91c0d18f83c3a0690ea188b9bf51212b8df89509"
HARNESS_DIR="${VAULT_SOLVER_E2E_DIR:-$ROOT_DIR/.e2e}"

case "$HARNESS_DIR" in
  /*) ;;
  *) HARNESS_DIR="$ROOT_DIR/$HARNESS_DIR" ;;
esac

fail() {
  printf '[vault-solver-e2e] %s\n' "$*" >&2
  exit 1
}

command -v git >/dev/null 2>&1 || fail "git is required"
[ "$HARNESS_DIR" != "$ROOT_DIR" ] || fail "refusing to manage the repository root"
[ ! -L "$HARNESS_DIR" ] || fail "harness directory must not be a symlink: $HARNESS_DIR"

if [ ! -e "$HARNESS_DIR" ]; then
  mkdir -p "$HARNESS_DIR"
  git -C "$HARNESS_DIR" init --quiet
  git -C "$HARNESS_DIR" remote add origin "$HARNESS_REPOSITORY"
elif ! git -C "$HARNESS_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; then
  fail "harness directory exists but is not a Git checkout: $HARNESS_DIR"
fi

origin="$(git -C "$HARNESS_DIR" config --get remote.origin.url 2>/dev/null || true)"
[ "$origin" = "$HARNESS_REPOSITORY" ] \
  || fail "unexpected harness origin: ${origin:-missing}"

if ! git -C "$HARNESS_DIR" cat-file -e "$HARNESS_REVISION^{commit}" 2>/dev/null; then
  printf '[vault-solver-e2e] fetching harness %s\n' "$HARNESS_REVISION"
  git -C "$HARNESS_DIR" fetch --no-tags --depth 1 origin "$HARNESS_REVISION"
fi

current="$(git -C "$HARNESS_DIR" rev-parse --verify HEAD 2>/dev/null || true)"
if [ -n "$current" ] && [ "$current" != "$HARNESS_REVISION" ]; then
  git -C "$HARNESS_DIR" submodule deinit --force --all >/dev/null 2>&1 || true
  git -C "$HARNESS_DIR" clean -ffdx >/dev/null
fi

git -C "$HARNESS_DIR" -c advice.detachedHead=false \
  checkout --detach --force "$HARNESS_REVISION" >/dev/null
git -C "$HARNESS_DIR" reset --hard "$HARNESS_REVISION" >/dev/null

actual="$(git -C "$HARNESS_DIR" rev-parse HEAD)"
[ "$actual" = "$HARNESS_REVISION" ] || fail "failed to select pinned harness revision"
printf '[vault-solver-e2e] harness ready at %s (%s)\n' "$HARNESS_DIR" "$actual"
