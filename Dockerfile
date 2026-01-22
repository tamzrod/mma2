# Dockerfile
# MMA2.0 – Modbus Memory Appliance (v2)

# ================================
# Build stage
# ================================
FROM golang:1.25-alpine AS builder

WORKDIR /build

# Install CA certs (for go modules)
RUN apk add --no-cache ca-certificates

# Copy module files first (cache-friendly)
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -o mma2 ./cmd/mma2

# ================================
# Runtime stage
# ================================
FROM alpine:3.19

# Security: non-root user
RUN addgroup -S mma && adduser -S mma -G mma

WORKDIR /app

# Copy binary
COPY --from=builder /build/mma2 /app/mma2

# Copy docs & example config (optional but useful)
COPY docs /app/docs

USER mma

# MMA2 listens dynamically based on config
EXPOSE 502

ENTRYPOINT ["/app/mma2"]
