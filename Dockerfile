# Proxera Control Node
# Builds the panel, embeds it into the Go binary, produces a single executable.
# Version is injected from git tags at build time.

ARG VERSION=dev

# --- Stage 1: Build panel ---
FROM node:22-bookworm-slim AS panel-builder
WORKDIR /build/panel
COPY panel/package*.json ./
RUN npm ci
COPY panel/ ./
# SvelteKit $env/static/public vars must be set at build time.
# Empty = relative URLs (same origin), which is correct for AIO.
ARG PUBLIC_API_URL=""
ARG PUBLIC_WS_URL=""
ARG PUBLIC_SITE_NAME="Proxera"
ARG PUBLIC_SITE_URL=""
ENV PUBLIC_API_URL=${PUBLIC_API_URL}
ENV PUBLIC_WS_URL=${PUBLIC_WS_URL}
ENV PUBLIC_SITE_NAME=${PUBLIC_SITE_NAME}
ENV PUBLIC_SITE_URL=${PUBLIC_SITE_URL}
RUN npm run build

# --- Stage 2: Build control binary ---
FROM golang:1.26-bookworm AS go-builder
ARG VERSION
WORKDIR /build
COPY go.work ./
COPY agent/ ./agent/
COPY backend/ ./backend/
COPY --from=panel-builder /build/panel/build ./backend/cmd/control/panel/
WORKDIR /build/backend
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X github.com/proxera/agent/pkg/version.Version=${VERSION}" \
    -o /proxera-control ./cmd/control/main.go

# --- Stage 3: Build agent binaries for distribution ---
FROM golang:1.26-bookworm AS agent-builder
ARG VERSION
WORKDIR /build
COPY go.work ./
COPY agent/ ./agent/
COPY backend/ ./backend/
WORKDIR /build/agent
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags "-X github.com/proxera/agent/pkg/version.Version=${VERSION}" \
    -o /proxera-linux-amd64 ./cmd/proxera/main.go && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build \
    -ldflags "-X github.com/proxera/agent/pkg/version.Version=${VERSION}" \
    -o /proxera-linux-arm64 ./cmd/proxera/main.go

# --- Stage 4: Final image ---
FROM alpine:3.21

RUN apk add --no-cache ca-certificates curl docker-cli && \
    mkdir -p /etc/nginx/ssl /etc/nginx/conf.d /var/log/nginx /var/lib/proxera

WORKDIR /app
COPY --from=go-builder /proxera-control .
COPY --from=agent-builder /proxera-linux-amd64 ./downloads/
COPY --from=agent-builder /proxera-linux-arm64 ./downloads/
COPY backend/install.sh ./install.sh

EXPOSE 5173
ENTRYPOINT ["./proxera-control"]
