# syntax=docker/dockerfile:1

# ─────────────── build stage ───────────────
# Pin the toolchain via GOTOOLCHAIN (matches the Makefile/CI); the base image only needs to be
# new enough to honour it. CGO is disabled so go-ethereum uses its pure-Go secp256k1 path and the
# result is a fully static binary.
FROM golang:1.27.1-alpine3.24 AS build

ENV GOTOOLCHAIN=go1.27.1 \
    CGO_ENABLED=0 \
    GOFLAGS=-trimpath

WORKDIR /src

# Dependency layer — cached unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN apk add --no-cache git ca-certificates
ENV GOPRIVATE=github.com/symbioticfi/merkle-executor
# BuildKit keeps the read token out of build arguments and image layers.
RUN --mount=type=secret,id=merkle_sdk_read_token,required=true \
    GIT_CONFIG_COUNT=1 \
    GIT_CONFIG_KEY_0=credential.https://github.com.helper \
    GIT_CONFIG_VALUE_0='!f() { read sdk_token < /run/secrets/merkle_sdk_read_token; printf "username=x-access-token\npassword=%s\n" "$sdk_token"; }; f' \
    go mod download

# Source + build. Version metadata is injected via build args (see docker-compose / make docker).
COPY . .
ARG VERSION=dev
ARG COMMIT=none
ARG DATE=unknown
RUN go build \
      -ldflags "-s -w \
        -X github.com/symbioticfi/vault-solver/internal/version.Version=${VERSION} \
        -X github.com/symbioticfi/vault-solver/internal/version.Commit=${COMMIT} \
        -X github.com/symbioticfi/vault-solver/internal/version.Date=${DATE}" \
      -o /out/vault-solver ./cmd/vault-solver

# ─────────────── runtime stage ───────────────
# Distroless static + nonroot: no shell, no package manager, runs as uid 65532.
FROM gcr.io/distroless/static-debian13:nonroot AS final

COPY --from=build /out/vault-solver /usr/local/bin/vault-solver

# Observability server (/metrics, /healthz, /readyz) — default observability.addr is :9090.
EXPOSE 9090

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/vault-solver"]
# Config path is operator-mounted; the signing key + secrets come from the environment.
CMD ["run", "--config", "/etc/vault-solver/config.yaml"]
