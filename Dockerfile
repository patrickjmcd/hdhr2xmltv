# Build stage
FROM golang:1.27-alpine AS builder

ARG VERSION=dev

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum* ./

# Download dependencies (if any)
RUN go mod download || true

# Copy source code
COPY . .

# Build the server binary with version info
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-w -s -X main.version=${VERSION}" \
    -o /hdhr2xmltv-server ./cmd/server

# Runtime stage
FROM alpine:3.24

RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

# Copy binary from builder
COPY --from=builder /hdhr2xmltv-server /app/hdhr2xmltv-server

# Create non-root user
RUN adduser -D -g '' appuser
USER appuser

EXPOSE 8080

ENV PORT=8080

ENTRYPOINT ["/app/hdhr2xmltv-server"]
