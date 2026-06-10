# syntax=docker/dockerfile:1

# ─────────────── build stage ───────────────
# Pin the toolchain via GOTOOLCHAIN (matches the Makefile/CI); the base image only needs to be
# new enough to honour it. CGO is disabled so go-ethereum uses its pure-Go secp256k1 path and the
# result is a fully static binary.
FROM golang:1.26.4-alpine3.23 AS build

ENV GOTOOLCHAIN=go1.26.4 \
    CGO_ENABLED=0 \
    GOFLAGS=-trimpath

WORKDIR /src

# Dependency layer — cached unless go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

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
