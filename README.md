# hdhr2xmltv

Convert HDHomeRun TV guide data to XMLTV format.

Fetches program guide data from your HDHomeRun tuner and outputs it in XMLTV format, compatible with Plex, Jellyfin, Emby, and other media servers.

## Features

- Fetches guide data via Silicon Dust's API using your HDHomeRun's device authentication
- Outputs standard XMLTV format
- Available as CLI tool or HTTP server
- Docker support with multi-arch images (amd64, arm64)
- Built-in caching to minimize API requests
- Kubernetes-ready with liveness/readiness probes
- Prometheus metrics endpoint
- No external dependencies (pure Go)

## Installation

### Docker (Recommended)

```bash
docker pull ghcr.io/patrickjmcd/hdhr2xmltv:latest
```

### From Source

```bash
go install github.com/patrickjmcd/hdhr2xmltv@latest
go install github.com/patrickjmcd/hdhr2xmltv/cmd/server@latest
```

### Build Locally

```bash
git clone https://github.com/patrickjmcd/hdhr2xmltv.git
cd hdhr2xmltv
go build -o hdhr2xmltv .
go build -o hdhr2xmltv-server ./cmd/server
```

## Usage

### CLI

```bash
# Basic usage
./hdhr2xmltv -ip 192.168.1.100

# Save to file
./hdhr2xmltv -ip 192.168.1.100 -output guide.xml

# Verbose output
./hdhr2xmltv -ip 192.168.1.100 -verbose
```

**CLI Options:**

| Flag | Description | Default |
|------|-------------|---------|
| `-ip` | HDHomeRun device IP address | `192.168.1.100` |
| `-output` | Output file path | stdout |
| `-verbose` | Enable verbose logging | `false` |
| `-version` | Show version | |

### HTTP Server

```bash
# Run server
HDHOMERUN_TUNER_ADDRESS=192.168.1.100 ./hdhr2xmltv-server

# With custom port and cache TTL
HDHOMERUN_TUNER_ADDRESS=192.168.1.100 PORT=3000 CACHE_TTL_MINUTES=120 ./hdhr2xmltv-server
```

**Environment Variables:**

| Variable | Description | Default |
|----------|-------------|---------|
| `HDHOMERUN_TUNER_ADDRESS` | HDHomeRun device IP address | *required* |
| `PORT` | HTTP server port | `8080` |
| `CACHE_TTL_MINUTES` | Cache duration in minutes | `60` |

### Docker

```bash
# Run with Docker
docker run -p 8080:8080 \
  -e HDHOMERUN_TUNER_ADDRESS=192.168.1.100 \
  ghcr.io/patrickjmcd/hdhr2xmltv:latest

# With custom cache TTL (4 hours)
docker run -p 8080:8080 \
  -e HDHOMERUN_TUNER_ADDRESS=192.168.1.100 \
  -e CACHE_TTL_MINUTES=240 \
  ghcr.io/patrickjmcd/hdhr2xmltv:latest
```

### Docker Compose

```yaml
services:
  hdhr2xmltv:
    image: ghcr.io/patrickjmcd/hdhr2xmltv:latest
    ports:
      - "8080:8080"
    environment:
      - HDHOMERUN_TUNER_ADDRESS=192.168.1.100
      - CACHE_TTL_MINUTES=60
    restart: unless-stopped
```

## API Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /` | Index page with links |
| `GET /xmltv` | XMLTV guide data |
| `GET /guide.xml` | XMLTV guide data (alias) |
| `GET /device` | HDHomeRun device info (JSON) |
| `GET /cache` | Cache status (JSON) |
| `POST /cache/clear` | Clear the cache |
| `GET /health` | Legacy health check (JSON) |
| `GET /healthz` | Kubernetes liveness probe |
| `GET /readyz` | Kubernetes readiness probe |
| `GET /metrics` | Prometheus metrics |

### Response Headers

The `/xmltv` endpoint includes cache status headers:

- `X-Cache: HIT` - Served from cache
- `X-Cache: MISS` - Fetched from API

## Kubernetes Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: hdhr2xmltv
spec:
  replicas: 1
  selector:
    matchLabels:
      app: hdhr2xmltv
  template:
    metadata:
      labels:
        app: hdhr2xmltv
    spec:
      containers:
        - name: hdhr2xmltv
          image: ghcr.io/patrickjmcd/hdhr2xmltv:latest
          ports:
            - containerPort: 8080
          env:
            - name: HDHOMERUN_TUNER_ADDRESS
              value: "192.168.1.100"
            - name: CACHE_TTL_MINUTES
              value: "60"
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 5
            periodSeconds: 10
          resources:
            requests:
              memory: "32Mi"
              cpu: "10m"
            limits:
              memory: "128Mi"
              cpu: "100m"
---
apiVersion: v1
kind: Service
metadata:
  name: hdhr2xmltv
spec:
  selector:
    app: hdhr2xmltv
  ports:
    - port: 8080
      targetPort: 8080
```

### Prometheus Metrics

The `/metrics` endpoint exposes Prometheus-compatible metrics:

| Metric | Type | Description |
|--------|------|-------------|
| `hdhr2xmltv_info` | gauge | Application version info |
| `hdhr2xmltv_uptime_seconds` | gauge | Time since server started |
| `hdhr2xmltv_http_requests_total` | counter | Total HTTP requests by endpoint and status |
| `hdhr2xmltv_http_request_duration_seconds` | summary | Request duration by endpoint |
| `hdhr2xmltv_cache_hits_total` | counter | Total cache hits |
| `hdhr2xmltv_cache_misses_total` | counter | Total cache misses |
| `hdhr2xmltv_cache_valid` | gauge | Whether cache is valid (1/0) |
| `hdhr2xmltv_cache_channels` | gauge | Number of channels in cache |
| `hdhr2xmltv_cache_programs` | gauge | Number of programs in cache |
| `hdhr2xmltv_guide_fetch_errors_total` | counter | Total guide fetch errors |
| `go_goroutines` | gauge | Number of goroutines |
| `go_memstats_alloc_bytes` | gauge | Bytes allocated and in use |
| `go_memstats_sys_bytes` | gauge | Total bytes from system |

## Integrating with Media Servers

### Plex

1. Go to Settings > Live TV & DVR
2. Add the XMLTV guide URL: `http://your-server:8080/xmltv`

### Jellyfin

1. Go to Dashboard > Live TV > Guide Providers
2. Add XMLTV provider with URL: `http://your-server:8080/xmltv`

### Emby

1. Go to Settings > Live TV > Guide Data
2. Add XMLTV source: `http://your-server:8080/xmltv`

## How It Works

1. Connects to your HDHomeRun device to retrieve device authentication
2. Uses the device auth to fetch guide data from Silicon Dust's API
3. Converts the guide data to XMLTV format
4. Caches the result to minimize API requests

The guide data comes from Silicon Dust (the company behind HDHomeRun), not from the device itself. This is the same data source used by the official HDHomeRun app.

## License

MIT
