# Multi-stage build for mini-rbac-go

# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git ca-certificates

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o mini-rbac-go ./cmd/server

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /app/mini-rbac-go .

# Create non-root user
RUN addgroup -g 1000 rbac && \
    adduser -D -u 1000 -G rbac rbac && \
    chown -R rbac:rbac /app

USER rbac

EXPOSE 8080

CMD ["./mini-rbac-go"]
