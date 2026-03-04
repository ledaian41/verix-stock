# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# Install system dependencies
RUN apk add --no-cache ca-certificates git

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile both binaries for production
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /verix-api ./cmd/api

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o /verix-worker ./cmd/worker

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM scratch

# Copy CA certs for outbound HTTPS (essential for scraping and Telegram)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binaries from builder
COPY --from=builder /verix-api /verix-api
COPY --from=builder /verix-worker /verix-worker

# Copy static web files
COPY ./web /web

# Default port
EXPOSE 8080

# API server is the default process
ENTRYPOINT ["/verix-api"]
