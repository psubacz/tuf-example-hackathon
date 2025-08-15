package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"sync"
	"time"
)

// Metrics holds server metrics
type Metrics struct {
	mu           sync.RWMutex
	StartTime    time.Time
	TotalRequests int64
	RequestsByPath map[string]int64
	RequestsByMethod map[string]int64
	ErrorsByCode map[int]int64
	ResponseTimes []time.Duration
	ActiveConnections int64
}

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		StartTime:         time.Now(),
		RequestsByPath:    make(map[string]int64),
		RequestsByMethod:  make(map[string]int64),
		ErrorsByCode:      make(map[int]int64),
		ResponseTimes:     make([]time.Duration, 0, 1000), // Keep last 1000 response times
	}
}

// RecordRequest records a request
func (m *Metrics) RecordRequest(method, path string, statusCode int, duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.TotalRequests++
	m.RequestsByPath[path]++
	m.RequestsByMethod[method]++
	
	if statusCode >= 400 {
		m.ErrorsByCode[statusCode]++
	}
	
	// Keep only the last 1000 response times to prevent memory growth
	if len(m.ResponseTimes) >= 1000 {
		m.ResponseTimes = m.ResponseTimes[1:]
	}
	m.ResponseTimes = append(m.ResponseTimes, duration)
}

// IncrementActiveConnections increments active connection count
func (m *Metrics) IncrementActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveConnections++
}

// DecrementActiveConnections decrements active connection count
func (m *Metrics) DecrementActiveConnections() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveConnections--
}

// GetSnapshot returns a snapshot of current metrics
func (m *Metrics) GetSnapshot() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	var avgResponseTime time.Duration
	if len(m.ResponseTimes) > 0 {
		var total time.Duration
		for _, rt := range m.ResponseTimes {
			total += rt
		}
		avgResponseTime = total / time.Duration(len(m.ResponseTimes))
	}
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	uptime := time.Since(m.StartTime)
	requestsPerSecond := float64(m.TotalRequests) / uptime.Seconds()
	
	return map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339),
		"uptime": uptime.String(),
		"requests": map[string]interface{}{
			"total": m.TotalRequests,
			"per_second": fmt.Sprintf("%.2f", requestsPerSecond),
			"by_path": m.RequestsByPath,
			"by_method": m.RequestsByMethod,
		},
		"errors": map[string]interface{}{
			"by_code": m.ErrorsByCode,
		},
		"performance": map[string]interface{}{
			"avg_response_time": avgResponseTime.String(),
			"active_connections": m.ActiveConnections,
		},
		"system": map[string]interface{}{
			"memory": map[string]interface{}{
				"alloc": fmt.Sprintf("%.2f MB", float64(memStats.Alloc)/1024/1024),
				"total_alloc": fmt.Sprintf("%.2f MB", float64(memStats.TotalAlloc)/1024/1024),
				"sys": fmt.Sprintf("%.2f MB", float64(memStats.Sys)/1024/1024),
				"gc_cycles": memStats.NumGC,
			},
			"goroutines": runtime.NumGoroutine(),
			"cpu_cores": runtime.NumCPU(),
		},
	}
}

// metricsHandler serves metrics in JSON format
func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	if !s.config.Metrics.Enabled {
		http.Error(w, "Metrics disabled", http.StatusNotFound)
		return
	}
	
	metrics := s.metrics.GetSnapshot()
	
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-cache")
	json.NewEncoder(w).Encode(metrics)
}

// PrometheusMetrics returns metrics in Prometheus format
func (m *Metrics) PrometheusMetrics() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	uptime := time.Since(m.StartTime).Seconds()
	
	var avgResponseTime float64
	if len(m.ResponseTimes) > 0 {
		var total time.Duration
		for _, rt := range m.ResponseTimes {
			total += rt
		}
		avgResponseTime = float64(total/time.Duration(len(m.ResponseTimes))) / float64(time.Millisecond)
	}
	
	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	
	prometheus := fmt.Sprintf(`# HELP tuf_server_uptime_seconds Server uptime in seconds
# TYPE tuf_server_uptime_seconds counter
tuf_server_uptime_seconds %.2f

# HELP tuf_server_requests_total Total number of requests
# TYPE tuf_server_requests_total counter
tuf_server_requests_total %d

# HELP tuf_server_requests_per_second Requests per second
# TYPE tuf_server_requests_per_second gauge
tuf_server_requests_per_second %.2f

# HELP tuf_server_avg_response_time_ms Average response time in milliseconds  
# TYPE tuf_server_avg_response_time_ms gauge
tuf_server_avg_response_time_ms %.2f

# HELP tuf_server_active_connections Current active connections
# TYPE tuf_server_active_connections gauge
tuf_server_active_connections %d

# HELP tuf_server_memory_alloc_bytes Current allocated memory in bytes
# TYPE tuf_server_memory_alloc_bytes gauge
tuf_server_memory_alloc_bytes %d

# HELP tuf_server_goroutines Current number of goroutines
# TYPE tuf_server_goroutines gauge
tuf_server_goroutines %d
`, uptime, m.TotalRequests, float64(m.TotalRequests)/uptime, avgResponseTime, m.ActiveConnections, memStats.Alloc, runtime.NumGoroutine())
	
	// Add request counts by path
	for path, count := range m.RequestsByPath {
		prometheus += fmt.Sprintf(`
# HELP tuf_server_requests_by_path_total Requests by path
# TYPE tuf_server_requests_by_path_total counter
tuf_server_requests_by_path_total{path="%s"} %d`, path, count)
	}
	
	// Add request counts by method
	for method, count := range m.RequestsByMethod {
		prometheus += fmt.Sprintf(`
# HELP tuf_server_requests_by_method_total Requests by HTTP method
# TYPE tuf_server_requests_by_method_total counter  
tuf_server_requests_by_method_total{method="%s"} %d`, method, count)
	}
	
	// Add error counts by status code
	for code, count := range m.ErrorsByCode {
		prometheus += fmt.Sprintf(`
# HELP tuf_server_errors_by_code_total Errors by HTTP status code
# TYPE tuf_server_errors_by_code_total counter
tuf_server_errors_by_code_total{code="%d"} %d`, code, count)
	}
	
	return prometheus
}