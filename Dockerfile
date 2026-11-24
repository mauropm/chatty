# syntax=docker/dockerfile:1
# ============================================================================
# Chatty — multi-stage build producing a tiny, static, non-root image.
# ============================================================================

# ---- build ----------------------------------------------------------------
FROM golang:1.27-alpine AS build
WORKDIR /src

RUN apk add --no-cache ca-certificates tzdata

# Cache module downloads first.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

# Build (front-end assets are embedded via go:embed).
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOFLAGS=-trimpath \
    go build -trimpath -ldflags="-s -w" -o /out/chatty ./cmd/chatty

# ---- runtime --------------------------------------------------------------
FROM scratch
LABEL org.opencontainers.image.title="Chatty" \
      org.opencontainers.image.description="Self-hosted AI chat interface for Mimir/Qwen" \
      org.opencontainers.image.licenses="MIT"

COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
COPY --from=build /out/chatty /chatty

USER 65532:65532
EXPOSE 3333
ENV APP_PORT=3333
ENTRYPOINT ["/chatty"]
HEALTHCHECK --interval=15s --timeout=5s --start-period=25s --retries=5 \
  CMD ["/chatty", "healthcheck"]
