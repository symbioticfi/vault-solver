# syntax=docker/dockerfile:1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

# ─────────────── build stage ───────────────
# Pin the toolchain via GOTOOLCHAIN (matches the Makefile/CI); the base image only needs to be
# new enough to honour it. CGO is disabled so go-ethereum uses its pure-Go secp256k1 path and the
# result is a fully static binary.
FROM golang:1.27.3-alpine3.24@sha256:d200d2dd76b03a99b52651fd1325aff7dcd35422c35c01bac56571b0507f3deb AS build

ENV GOTOOLCHAIN=go1.27.3 \
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
FROM gcr.io/distroless/static-debian13:nonroot@sha256:18f4cc26c9ffa7f20a359298be7deadac5c607c8e03a02b8119f1a5fc2f82c99 AS final

COPY --from=build /out/vault-solver /usr/local/bin/vault-solver

# Observability server (/metrics, /healthz, /readyz) — default observability.addr is :9090.
EXPOSE 9090

USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/vault-solver"]
# Config path is operator-mounted; the signing key + secrets come from the environment.
CMD ["run", "--config", "/etc/vault-solver/config.yaml"]
