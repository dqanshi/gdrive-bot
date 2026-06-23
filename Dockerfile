# ── Build stage ─────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS builder
RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gdrive-bot ./cmd/bot

# ── Runtime stage ────────────────────────────────────────────────────────────
FROM alpine:3.20
RUN apk add --no-cache ca-certificates rclone tzdata

WORKDIR /app

COPY --from=builder /gdrive-bot /app/gdrive-bot

RUN mkdir -p config downloads logs

# The env file is mounted at runtime via docker-compose or injected as env vars.
VOLUME ["/app/config", "/app/downloads", "/app/logs"]

ENTRYPOINT ["/app/gdrive-bot"]
