# syntax=docker/dockerfile:1
# Multi-stage build for minidon.
#
# Stage order:
#   1. node-builder  — installs Node deps and produces web/dist
#   2. go-builder    — compiles Go binary with web/dist embedded via go:embed
#   3. final         — minimal distroless image containing only the binary

# ── Stage 1: Node / frontend build ───────────────────────────────────────────
FROM node:20-alpine AS node-builder

WORKDIR /src/web

# Copy package files first for layer caching
COPY web/package.json web/package-lock.json* ./

RUN npm ci

# Copy the rest of the frontend source and build
COPY web/ ./
RUN npm run build
# Produces /src/web/dist

# ── Stage 2: Go build ─────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS go-builder

WORKDIR /src

# Copy module files first for layer caching
COPY go.mod go.sum* ./
RUN go mod download

# Copy Go source
COPY cmd/ ./cmd/
COPY internal/ ./internal/

# Copy the frontend build output (needed for go:embed)
COPY --from=node-builder /src/web/dist ./web/dist

# Build a fully static binary (CGO disabled)
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" \
    -o /out/minidon ./cmd/minidon

# ── Stage 3: Final minimal image ──────────────────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot AS final

COPY --from=go-builder /out/minidon /minidon

EXPOSE 8080

ENTRYPOINT ["/minidon"]
