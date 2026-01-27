package main

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/patrickjmcd/hdhr2xmltv/internal/hdhr"
	"github.com/patrickjmcd/hdhr2xmltv/internal/xmltv"
)

var (
	version = "dev"
)

func main() {
	tunerAddress := os.Getenv("HDHOMERUN_TUNER_ADDRESS")
	if tunerAddress == "" {
		log.Fatal("HDHOMERUN_TUNER_ADDRESS environment variable is required")
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	cacheTTL := 60 * time.Minute // default 1 hour
	if ttlStr := os.Getenv("CACHE_TTL_MINUTES"); ttlStr != "" {
		if ttl, err := strconv.Atoi(ttlStr); err == nil && ttl > 0 {
			cacheTTL = time.Duration(ttl) * time.Minute
		}
	}

	server := NewServer(tunerAddress, cacheTTL)

	httpServer := &http.Server{
		Addr:         ":" + port,
		Handler:      server,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 120 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	done := make(chan bool, 1)
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-quit
		log.Println("Shutting down server...")

		server.SetShuttingDown(true)

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := httpServer.Shutdown(ctx); err != nil {
			log.Fatalf("Server forced to shutdown: %v", err)
		}
		close(done)
	}()

	log.Printf("hdhr2xmltv server %s starting on port %s", version, port)
	log.Printf("HDHomeRun tuner address: %s", tunerAddress)
	log.Printf("Cache TTL: %v", cacheTTL)

	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	<-done
	log.Println("Server stopped")
}

// Metrics tracks server metrics for Prometheus.
type Metrics struct {
	requestsTotal      sync.Map // map[string]*uint64 - endpoint:status
	requestDurationSum sync.Map // map[string]*uint64 - endpoint (nanoseconds)
	requestDurationCnt sync.Map // map[string]*uint64 - endpoint
	cacheHits          uint64
	cacheMisses        uint64
	guideFetchErrors   uint64
	startTime          time.Time
}

func NewMetrics() *Metrics {
	return &Metrics{
		startTime: time.Now(),
	}
}

func (m *Metrics) IncRequests(endpoint string, status int) {
	key := fmt.Sprintf("%s:%d", endpoint, status)
	val, _ := m.requestsTotal.LoadOrStore(key, new(uint64))
	atomic.AddUint64(val.(*uint64), 1)
}

func (m *Metrics) ObserveDuration(endpoint string, d time.Duration) {
	sumVal, _ := m.requestDurationSum.LoadOrStore(endpoint, new(uint64))
	atomic.AddUint64(sumVal.(*uint64), uint64(d.Nanoseconds()))

	cntVal, _ := m.requestDurationCnt.LoadOrStore(endpoint, new(uint64))
	atomic.AddUint64(cntVal.(*uint64), 1)
}

func (m *Metrics) IncCacheHit()        { atomic.AddUint64(&m.cacheHits, 1) }
func (m *Metrics) IncCacheMiss()       { atomic.AddUint64(&m.cacheMisses, 1) }
func (m *Metrics) IncGuideFetchError() { atomic.AddUint64(&m.guideFetchErrors, 1) }

// GuideCache stores cached guide data.
type GuideCache struct {
	mu           sync.RWMutex
	data         []byte
	fetchedAt    time.Time
	channelCount int
	programCount int
	ttl          time.Duration
}

// IsValid returns true if the cache is still valid.
func (c *GuideCache) IsValid() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.data != nil && time.Since(c.fetchedAt) < c.ttl
}

// Get returns the cached data if valid.
func (c *GuideCache) Get() ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data != nil && time.Since(c.fetchedAt) < c.ttl {
		return c.data, true
	}
	return nil, false
}

// Set stores data in the cache.
func (c *GuideCache) Set(data []byte, channelCount, programCount int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = data
	c.fetchedAt = time.Now()
	c.channelCount = channelCount
	c.programCount = programCount
}

// Clear invalidates the cache.
func (c *GuideCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.data = nil
	c.fetchedAt = time.Time{}
	c.channelCount = 0
	c.programCount = 0
}

// Stats returns cache statistics.
func (c *GuideCache) Stats() (fetchedAt time.Time, age time.Duration, channelCount, programCount int, valid bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.data == nil {
		return time.Time{}, 0, 0, 0, false
	}
	age = time.Since(c.fetchedAt)
	return c.fetchedAt, age, c.channelCount, c.programCount, age < c.ttl
}

// Server handles HTTP requests for XMLTV guide data.
type Server struct {
	tunerAddress string
	mux          *http.ServeMux
	cache        *GuideCache
	metrics      *Metrics
	shuttingDown atomic.Bool
	ready        atomic.Bool
}

// NewServer creates a new HTTP server.
func NewServer(tunerAddress string, cacheTTL time.Duration) *Server {
	s := &Server{
		tunerAddress: tunerAddress,
		mux:          http.NewServeMux(),
		cache:        &GuideCache{ttl: cacheTTL},
		metrics:      NewMetrics(),
	}
	s.ready.Store(true) // Ready immediately, readiness check verifies HDHomeRun connectivity
	s.routes()
	return s
}

func (s *Server) SetShuttingDown(v bool) {
	s.shuttingDown.Store(v)
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) routes() {
	// Application endpoints
	s.mux.HandleFunc("/", s.handleIndex)
	s.mux.HandleFunc("/xmltv", s.handleXMLTV)
	s.mux.HandleFunc("/xmltv.xml", s.handleXMLTV)
	s.mux.HandleFunc("/guide", s.handleXMLTV)
	s.mux.HandleFunc("/guide.xml", s.handleXMLTV)
	s.mux.HandleFunc("/lineup.m3u", s.handleLineupM3U)
	s.mux.HandleFunc("/playlist.m3u", s.handleLineupM3U)
	s.mux.HandleFunc("/device", s.handleDevice)
	s.mux.HandleFunc("/cache", s.handleCacheStatus)
	s.mux.HandleFunc("/cache/clear", s.handleCacheClear)

	// Kubernetes probes
	s.mux.HandleFunc("/health", s.handleHealth)  // Legacy health endpoint
	s.mux.HandleFunc("/healthz", s.handleLivez)  // Liveness probe
	s.mux.HandleFunc("/livez", s.handleLivez)    // Liveness probe (alias)
	s.mux.HandleFunc("/readyz", s.handleReadyz)  // Readiness probe
	s.mux.HandleFunc("/startupz", s.handleLivez) // Startup probe (same as liveness)

	// Prometheus metrics
	s.mux.HandleFunc("/metrics", s.handleMetrics)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	fetchedAt, age, channels, programs, valid := s.cache.Stats()
	cacheStatus := "empty"
	cacheInfo := ""
	if valid {
		cacheStatus = "valid"
		cacheInfo = fmt.Sprintf(" (fetched %s ago, %d channels, %d programs)", age.Round(time.Second), channels, programs)
	} else if !fetchedAt.IsZero() {
		cacheStatus = "expired"
		cacheInfo = fmt.Sprintf(" (expired %s ago)", (age - s.cache.ttl).Round(time.Second))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html>
<html>
<head><title>hdhr2xmltv</title></head>
<body>
<h1>hdhr2xmltv</h1>
<p>HDHomeRun to XMLTV converter</p>
<ul>
<li><a href="/xmltv">/xmltv</a> - Get XMLTV guide data</li>
<li><a href="/lineup.m3u">/lineup.m3u</a> - Get M3U playlist</li>
<li><a href="/device">/device</a> - Get HDHomeRun device info</li>
<li><a href="/cache">/cache</a> - Cache status</li>
<li><a href="/metrics">/metrics</a> - Prometheus metrics</li>
<li><a href="/healthz">/healthz</a> - Liveness probe</li>
<li><a href="/readyz">/readyz</a> - Readiness probe</li>
</ul>
<p>Cache: %s%s</p>
<p>Cache TTL: %v</p>
<p>Version: %s</p>
</body>
</html>`, cacheStatus, cacheInfo, s.cache.ttl, version)
}

// handleLivez is the Kubernetes liveness probe.
// Returns 200 if the process is alive.
func (s *Server) handleLivez(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok"}`)
}

// handleReadyz is the Kubernetes readiness probe.
// Returns 200 if the server can serve traffic (can reach HDHomeRun).
func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Not ready if shutting down
	if s.shuttingDown.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"shutting_down","ready":false}`)
		return
	}

	// Check if we can reach the HDHomeRun (with timeout)
	client := hdhr.NewClient(s.tunerAddress)
	_, err := client.GetDeviceInfo()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprintf(w, `{"status":"hdhr_unreachable","ready":false,"error":"%s"}`, err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","ready":true}`)
}

// handleHealth is the legacy health endpoint (combines liveness + basic info).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	_, _, _, _, cacheValid := s.cache.Stats()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, `{"status":"ok","version":"%s","cache_valid":%t}`, version, cacheValid)
}

// handleMetrics returns Prometheus-compatible metrics.
func (s *Server) handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")

	var buf bytes.Buffer

	// App info
	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_info Application information\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_info gauge\n")
	fmt.Fprintf(&buf, "hdhr2xmltv_info{version=\"%s\"} 1\n", version)

	// Uptime
	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_uptime_seconds Time since server started\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_uptime_seconds gauge\n")
	fmt.Fprintf(&buf, "hdhr2xmltv_uptime_seconds %.0f\n", time.Since(s.metrics.startTime).Seconds())

	// Request counts
	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_http_requests_total Total HTTP requests\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_http_requests_total counter\n")
	s.metrics.requestsTotal.Range(func(key, value interface{}) bool {
		keyStr := key.(string)
		count := atomic.LoadUint64(value.(*uint64))
		// Parse endpoint:status (e.g., "/xmltv:200")
		lastColon := len(keyStr) - 1
		for lastColon >= 0 && keyStr[lastColon] != ':' {
			lastColon--
		}
		if lastColon > 0 {
			endpoint := keyStr[:lastColon]
			status := keyStr[lastColon+1:]
			fmt.Fprintf(&buf, "hdhr2xmltv_http_requests_total{endpoint=\"%s\",status=\"%s\"} %d\n", endpoint, status, count)
		}
		return true
	})

	// Request duration
	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_http_request_duration_seconds HTTP request duration\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_http_request_duration_seconds summary\n")
	s.metrics.requestDurationCnt.Range(func(key, value interface{}) bool {
		endpoint := key.(string)
		count := atomic.LoadUint64(value.(*uint64))
		sumVal, _ := s.metrics.requestDurationSum.Load(endpoint)
		sum := atomic.LoadUint64(sumVal.(*uint64))
		fmt.Fprintf(&buf, "hdhr2xmltv_http_request_duration_seconds_sum{endpoint=\"%s\"} %.6f\n", endpoint, float64(sum)/1e9)
		fmt.Fprintf(&buf, "hdhr2xmltv_http_request_duration_seconds_count{endpoint=\"%s\"} %d\n", endpoint, count)
		return true
	})

	// Cache metrics
	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_cache_hits_total Total cache hits\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_cache_hits_total counter\n")
	fmt.Fprintf(&buf, "hdhr2xmltv_cache_hits_total %d\n", atomic.LoadUint64(&s.metrics.cacheHits))

	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_cache_misses_total Total cache misses\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_cache_misses_total counter\n")
	fmt.Fprintf(&buf, "hdhr2xmltv_cache_misses_total %d\n", atomic.LoadUint64(&s.metrics.cacheMisses))

	_, _, channels, programs, valid := s.cache.Stats()
	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_cache_valid Whether cache is valid\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_cache_valid gauge\n")
	validInt := 0
	if valid {
		validInt = 1
	}
	fmt.Fprintf(&buf, "hdhr2xmltv_cache_valid %d\n", validInt)

	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_cache_channels Number of channels in cache\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_cache_channels gauge\n")
	fmt.Fprintf(&buf, "hdhr2xmltv_cache_channels %d\n", channels)

	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_cache_programs Number of programs in cache\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_cache_programs gauge\n")
	fmt.Fprintf(&buf, "hdhr2xmltv_cache_programs %d\n", programs)

	// Guide fetch errors
	fmt.Fprintf(&buf, "# HELP hdhr2xmltv_guide_fetch_errors_total Total guide fetch errors\n")
	fmt.Fprintf(&buf, "# TYPE hdhr2xmltv_guide_fetch_errors_total counter\n")
	fmt.Fprintf(&buf, "hdhr2xmltv_guide_fetch_errors_total %d\n", atomic.LoadUint64(&s.metrics.guideFetchErrors))

	// Go runtime metrics
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	fmt.Fprintf(&buf, "# HELP go_goroutines Number of goroutines\n")
	fmt.Fprintf(&buf, "# TYPE go_goroutines gauge\n")
	fmt.Fprintf(&buf, "go_goroutines %d\n", runtime.NumGoroutine())

	fmt.Fprintf(&buf, "# HELP go_memstats_alloc_bytes Bytes allocated and in use\n")
	fmt.Fprintf(&buf, "# TYPE go_memstats_alloc_bytes gauge\n")
	fmt.Fprintf(&buf, "go_memstats_alloc_bytes %d\n", memStats.Alloc)

	fmt.Fprintf(&buf, "# HELP go_memstats_sys_bytes Total bytes obtained from system\n")
	fmt.Fprintf(&buf, "# TYPE go_memstats_sys_bytes gauge\n")
	fmt.Fprintf(&buf, "go_memstats_sys_bytes %d\n", memStats.Sys)

	w.Write(buf.Bytes())
}

func (s *Server) handleCacheStatus(w http.ResponseWriter, r *http.Request) {
	fetchedAt, age, channels, programs, valid := s.cache.Stats()

	w.Header().Set("Content-Type", "application/json")
	if fetchedAt.IsZero() {
		fmt.Fprintf(w, `{"valid":false,"empty":true,"ttl_minutes":%d}`, int(s.cache.ttl.Minutes()))
		return
	}

	fmt.Fprintf(w, `{
  "valid": %t,
  "empty": false,
  "fetched_at": "%s",
  "age_seconds": %d,
  "ttl_minutes": %d,
  "expires_in_seconds": %d,
  "channel_count": %d,
  "program_count": %d
}`,
		valid,
		fetchedAt.Format(time.RFC3339),
		int(age.Seconds()),
		int(s.cache.ttl.Minutes()),
		int((s.cache.ttl - age).Seconds()),
		channels,
		programs,
	)
}

func (s *Server) handleCacheClear(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed. Use POST or DELETE.", http.StatusMethodNotAllowed)
		return
	}

	s.cache.Clear()
	log.Println("Cache cleared manually")

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{"status":"ok","message":"cache cleared"}`)
}

func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	client := hdhr.NewClient(s.tunerAddress)

	deviceInfo, err := client.GetDeviceInfo()
	if err != nil {
		log.Printf("Error fetching device info: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get device info: %v", err), http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, `{
  "friendly_name": "%s",
  "model_number": "%s",
  "firmware_name": "%s",
  "firmware_version": "%s",
  "device_id": "%s",
  "tuner_address": "%s"
}`,
		deviceInfo.FriendlyName,
		deviceInfo.ModelNumber,
		deviceInfo.FirmwareName,
		deviceInfo.FirmwareVersion,
		deviceInfo.DeviceID,
		s.tunerAddress,
	)
}

// handleLineupM3U returns an M3U playlist of the channel lineup.
func (s *Server) handleLineupM3U(w http.ResponseWriter, r *http.Request) {
	client := hdhr.NewClient(s.tunerAddress)

	lineup, err := client.GetLineup()
	if err != nil {
		log.Printf("Error fetching lineup: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get lineup: %v", err), http.StatusBadGateway)
		return
	}

	var buf bytes.Buffer
	buf.WriteString("#EXTM3U\r\n")

	for _, ch := range lineup {
		// Determine group title based on Favorite flag
		groupTitle := "Other"
		if ch.Favorite == 1 {
			groupTitle = "Favorites"
		}

		// Write EXTINF line
		// Format: #EXTINF:-1 tvg-id="X" tvg-chno="X" tvg-name="Name" group-title="Group" tvg-type="live",Name
		fmt.Fprintf(&buf, `#EXTINF:-1 tvg-id="%s" tvg-chno="%s" tvg-name="%s" group-title="%s" tvg-type="live",%s`,
			ch.GuideNumber,
			ch.GuideNumber,
			ch.GuideName,
			groupTitle,
			ch.GuideName,
		)
		buf.WriteString("\r\n")

		// Write URL line
		buf.WriteString(ch.URL)
		buf.WriteString("\r\n")
	}

	w.Header().Set("Content-Type", "audio/x-mpegurl; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="lineup.m3u"`)
	w.Write(buf.Bytes())
}

func (s *Server) handleXMLTV(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	defer func() {
		s.metrics.ObserveDuration("/xmltv", time.Since(start))
	}()

	// Check cache first
	if data, ok := s.cache.Get(); ok {
		s.metrics.IncCacheHit()
		s.metrics.IncRequests("/xmltv", 200)
		log.Printf("Serving XMLTV guide from cache")
		w.Header().Set("Content-Type", "application/xml; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="guide.xml"`)
		w.Header().Set("X-Cache", "HIT")
		w.Write(data)
		return
	}

	s.metrics.IncCacheMiss()
	log.Printf("Cache miss - fetching XMLTV guide data from %s", s.tunerAddress)

	client := hdhr.NewClient(s.tunerAddress)

	deviceInfo, err := client.GetDeviceInfo()
	if err != nil {
		s.metrics.IncGuideFetchError()
		s.metrics.IncRequests("/xmltv", 502)
		log.Printf("Error fetching device info: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get device info: %v", err), http.StatusBadGateway)
		return
	}

	if deviceInfo.DeviceAuth == "" {
		s.metrics.IncGuideFetchError()
		s.metrics.IncRequests("/xmltv", 502)
		log.Printf("Device did not provide DeviceAuth")
		http.Error(w, "Device did not provide DeviceAuth", http.StatusBadGateway)
		return
	}

	guide, err := client.GetGuide(deviceInfo)
	if err != nil {
		s.metrics.IncGuideFetchError()
		s.metrics.IncRequests("/xmltv", 502)
		log.Printf("Error fetching guide: %v", err)
		http.Error(w, fmt.Sprintf("Failed to get guide: %v", err), http.StatusBadGateway)
		return
	}

	programCount := 0
	for _, ch := range guide {
		programCount += len(ch.Guide)
	}

	// Write to buffer for caching
	var buf bytes.Buffer
	writer := xmltv.NewWriter(&buf)
	if err := writer.Write(guide); err != nil {
		s.metrics.IncGuideFetchError()
		s.metrics.IncRequests("/xmltv", 500)
		log.Printf("Error writing XMLTV: %v", err)
		http.Error(w, fmt.Sprintf("Failed to generate XMLTV: %v", err), http.StatusInternalServerError)
		return
	}

	// Store in cache
	data := buf.Bytes()
	s.cache.Set(data, len(guide), programCount)

	s.metrics.IncRequests("/xmltv", 200)
	log.Printf("Retrieved %d channels with %d programs in %v (cached)", len(guide), programCount, time.Since(start))

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="guide.xml"`)
	w.Header().Set("X-Cache", "MISS")
	w.Write(data)
}
