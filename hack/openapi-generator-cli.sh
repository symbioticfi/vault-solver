#!/usr/bin/env bash
set -euo pipefail

: "${OPENAPI_GENERATOR_VERSION:?OPENAPI_GENERATOR_VERSION must be set}"
: "${OPENAPI_GENERATOR_SHA256:?OPENAPI_GENERATOR_SHA256 must be set}"
jar="${TMPDIR:-/tmp}/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar"
url="https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/${OPENAPI_GENERATOR_VERSION}/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar"

verify_jar() {
  local file=$1
  [[ "$OPENAPI_GENERATOR_SHA256" =~ ^[[:xdigit:]]{64}$ ]] || return 1
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
