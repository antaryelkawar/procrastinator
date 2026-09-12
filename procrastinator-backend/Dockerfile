# syntax=docker/dockerfile:1

# Build stage: cross-compile a pure-Go static binary for the target platform.
# TARGETOS/TARGETARCH are auto-provided by `docker buildx build --platform ...`.
FROM golang:1.27-alpine AS build

# Pure-Go static build: no CGO, so no emulation is needed in this stage.
ENV CGO_ENABLED=0
# Fail fast on go.mod/go.sum drift instead of silently resolving modules.
ENV GOFLAGS=-mod=readonly

# Standard buildx build args for multi-platform cross-compilation.
ARG TARGETOS
ARG TARGETARCH

WORKDIR /src

# Cache-friendly ordering: fetch modules before copying source, so the module
# download layer stays cached across source-only changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -o /out/procrastinator ./api/cmd/procrastinator

# Runtime stage: distroless static ships CA certs (needed for pgx Postgres TLS)
# and no shell (minimal attack surface). :nonroot runs as uid/gid 65532.
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
# WORKDIR /app makes the default `-migrations` flag value `migrations` resolve
# to /app/migrations with zero config.

COPY --from=build /out/procrastinator /procrastinator
COPY --from=build /src/migrations/ /app/migrations/

USER nonroot

CMD ["/procrastinator"]
