# ── Stage 1: Build ────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder

# Install system dependencies
RUN apk add --no-cache ca-certificates git

WORKDIR /build

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Compile both API and Worker binaries
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o verix-api ./cmd/api

RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="-s -w" -o verix-worker ./cmd/worker

# ── Stage 2: Runtime ──────────────────────────────────────────────────────────
FROM scratch

# Copy CA certs for outbound HTTPS (important for scraping & telegram)
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binaries
COPY --from=builder /build/verix-api /verix-api
COPY --from=builder /build/verix-worker /verix-worker

# Copy web static files for the dashboard
COPY ./web /web

# Default port for API
EXPOSE 8080

# The default entrypoint is the API
# For worker, override the command in docker-compose or docker run
ENTRYPOINT ["/verix-api"]
