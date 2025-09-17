package monitoring

import (
	"encoding/json"
	"net/http"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	pp "net/http/pprof"
)

// Metrics provides lightweight in-memory metrics for request durations and runtime stats.
type Metrics struct {
	mu        sync.Mutex
	durations []float64 // milliseconds, circular buffer of last N
	idx       int
	count     int64 // total requests observed
	n         int   // capacity
}

func NewMetrics(capacity int) *Metrics {
	if capacity <= 0 {
		capacity = 256
	}
	return &Metrics{durations: make([]float64, capacity), n: capacity}
}

// Observe adds a duration sample (in milliseconds).
func (m *Metrics) Observe(ms float64) {
	m.mu.Lock()
	m.durations[m.idx] = ms
	m.idx = (m.idx + 1) % m.n
	m.count++
	m.mu.Unlock()
}

// Snapshot returns basic stats including quantiles for recent samples.
func (m *Metrics) Snapshot() (count int64, avg, p50, p95 float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// copy valid samples
	var samples []float64
	if m.count < int64(m.n) {
		samples = append(samples, m.durations[:m.idx]...)
	} else {
		samples = append(samples, m.durations...)
	}
	if len(samples) == 0 {
		return m.count, 0, 0, 0
	}
	// compute avg and quantiles
	var sum float64
	for _, v := range samples {
		sum += v
	}
	avg = sum / float64(len(samples))
	cp := make([]float64, len(samples))
	copy(cp, samples)
	sort.Float64s(cp)
	p50 = cp[(len(cp)*50)/100]
	p95 = cp[(len(cp)*95)/100]
	return m.count, avg, p50, p95
}

// ResponseWriter wrapper to capture status codes
type statusWriter struct {
	w          http.ResponseWriter
	statusCode int
}

func (sw *statusWriter) Header() http.Header         { return sw.w.Header() }
func (sw *statusWriter) Write(b []byte) (int, error) { return sw.w.Write(b) }
func (sw *statusWriter) WriteHeader(statusCode int) {
	sw.statusCode = statusCode
	sw.w.WriteHeader(statusCode)
}

// Middleware returns a standard http middleware that measures request duration
// and records it into Metrics. Keep it simple; we don't track labels.
func Middleware(m *Metrics) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			sw := &statusWriter{w: w, statusCode: http.StatusOK}
			next.ServeHTTP(sw, r)
			dur := time.Since(start).Seconds() * 1000.0
			m.Observe(dur)
		})
	}
}

// MetricsHandler exposes runtime and request metrics in JSON for quick consumption.
func MetricsHandler(m *Metrics) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		count, avg, p50, p95 := m.Snapshot()
		resp := map[string]interface{}{
			"time":             time.Now().Format(time.RFC3339),
			"requests_total":   count,
			"duration_ms_avg":  avg,
			"duration_ms_p50":  p50,
			"duration_ms_p95":  p95,
			"goroutines":       runtime.NumGoroutine(),
			"mem_alloc_bytes":  ms.Alloc,
			"heap_inuse_bytes": ms.HeapInuse,
			"gc_num":           ms.NumGC,
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// RegisterPprof registers all standard pprof handlers on the provided mux under /debug/pprof/.
func RegisterPprof(mux *http.ServeMux) {
	mux.HandleFunc("/debug/pprof/", pp.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pp.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pp.Profile) // CPU profile
	mux.HandleFunc("/debug/pprof/symbol", pp.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pp.Trace)
	// Maltypes are served automatically by Index, but register explicitly too
	mux.Handle("/debug/pprof/goroutine", pp.Handler("goroutine"))
	mux.Handle("/debug/pprof/heap", pp.Handler("heap"))
	mux.Handle("/debug/pprof/block", pp.Handler("block"))
	mux.Handle("/debug/pprof/mutex", pp.Handler("mutex"))
}

// EnableProfiling toggles runtime profiling rates for block/mutex when enabled.
func EnableProfiling(enabled bool) {
	if enabled {
		// 1 means capture every blocking event; adjust if too heavy
		runtime.SetBlockProfileRate(1)
		// Sample mutex contention roughly 1/5 of events
		runtime.SetMutexProfileFraction(5)
		// Leave CPU profile off by default; it's on-demand via /profile
		_ = pprof.Lookup("block")
		_ = pprof.Lookup("mutex")
	} else {
		runtime.SetBlockProfileRate(0)
		runtime.SetMutexProfileFraction(0)
	}
}
