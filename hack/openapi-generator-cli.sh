#!/usr/bin/env bash
# Simplified version of https://openapi-generator.tech/docs/installation#bash-launcher-script that does not require maven:
# on demand downloads specified version of openapi-generator-cli.java and runs it
set -o pipefail

if [ -z "$OPENAPI_GENERATOR_VERSION" ]
then
  echo "openapi-generator version must be specified in OPENAPI_GENERATOR_VERSION environment variable"
  exit 1
fi

if [ ! -f "/tmp/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar" ]
then
  curl -f "https://repo1.maven.org/maven2/org/openapitools/openapi-generator-cli/${OPENAPI_GENERATOR_VERSION}/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar" \
    -o "/tmp/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar"
fi

# Convenience for mac users: by default homebrew is not symlinked so we need to check known java location in homebrew
# shellcheck disable=SC2086 # JAVA_OPTS may contain multiple flags
PATH="/opt/homebrew/opt/openjdk/bin:$PATH" java -ea ${JAVA_OPTS} -Xms512M -Xmx1024M -server -jar "/tmp/openapi-generator-cli-${OPENAPI_GENERATOR_VERSION}.jar" "$@"