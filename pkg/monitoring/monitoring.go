package monitoring

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"runtime"
	"runtime/pprof"
	"sort"
	"sync"
	"time"

	"assisted-venue-approval/pkg/config"
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

// StartRuntimeMonitor samples runtime stats periodically and emits alerts when thresholds are exceeded.
// It logs via the provided logger fn (fmt.Printf-compatible). Thresholds come from cfg.
func StartRuntimeMonitor(ctx context.Context, cfg *config.Config, m *Metrics, logger func(string, ...any)) {
	if logger == nil {
		logger = func(format string, a ...any) { fmt.Printf(format+"\n", a...) }
	}
	if cfg == nil || !cfg.AlertsEnabled {
		return
	}
	interval := cfg.AlertSampleEvery
	if interval <= 0 {
		interval = 5 * time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()

	var alertedLatency, alertedGor, alertedMem, alertedGC bool
	var lastNumGC uint32

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			_, _, _, p95 := m.Snapshot()
			gor := runtime.NumGoroutine()
			allocMB := float64(ms.Alloc) / (1024.0 * 1024.0)
			lastPauseMs := 0.0
			if ms.NumGC != 0 {
				lastPauseNs := ms.PauseNs[(ms.NumGC-1)%uint32(len(ms.PauseNs))]
				lastPauseMs = float64(lastPauseNs) / 1e6
			}

			// p95 latency
			if cfg.AlertP95Ms > 0 && p95 > cfg.AlertP95Ms {
				if !alertedLatency {
					logger("ALERT: p95 latency %.1fms exceeded threshold %.1fms", p95, cfg.AlertP95Ms)
					alertedLatency = true
				}
			} else if alertedLatency {
				logger("RECOVERY: p95 latency back to normal: %.1fms", p95)
				alertedLatency = false
			}

			// goroutines
			if cfg.AlertGoroutines > 0 && gor > cfg.AlertGoroutines {
				if !alertedGor {
					logger("ALERT: goroutines %d exceeded threshold %d", gor, cfg.AlertGoroutines)
					alertedGor = true
				}
			} else if alertedGor {
				logger("RECOVERY: goroutines back to normal: %d", gor)
				alertedGor = false
			}

			// memory alloc
			if cfg.AlertMemAllocMB > 0 && allocMB > cfg.AlertMemAllocMB {
				if !alertedMem {
					logger("ALERT: mem alloc %.1fMB exceeded threshold %.1fMB", allocMB, cfg.AlertMemAllocMB)
					alertedMem = true
				}
			} else if alertedMem {
				logger("RECOVERY: mem alloc back to normal: %.1fMB", allocMB)
				alertedMem = false
			}

			// GC pause (only log on change of GC cycle)
			if cfg.AlertGCPauseMs > 0 && ms.NumGC != lastNumGC {
				lastNumGC = ms.NumGC
				if lastPauseMs > cfg.AlertGCPauseMs {
					if !alertedGC {
						logger("ALERT: last GC pause %.1fms exceeded threshold %.1fms (NumGC=%d)", lastPauseMs, cfg.AlertGCPauseMs, ms.NumGC)
						alertedGC = true
					}
				} else if alertedGC {
					logger("RECOVERY: GC pause back to normal: %.1fms (NumGC=%d)", lastPauseMs, ms.NumGC)
					alertedGC = false
				}
			}
		}
	}
}

// CostMetrics contains optional business cost metrics to expose alongside runtime metrics.
// Values are computed by the application and provided via a callback.
// Note: We intentionally keep field names generic here; JSON keys are added in snake_case.
type CostMetrics struct {
	TotalCostUSD float64
	TotalVenues  int64
	CostPerVenue float64
}

// MetricsHandlerWithCosts exposes the same metrics as MetricsHandler and, if provided,
// augments the JSON payload with cost metrics that are more Prometheus-friendly.
// It keeps the original field names for existing metrics and adds:
// - total_cost_usd
// - total_venues_processed
// - cost_per_venue_usd
func MetricsHandlerWithCosts(m *Metrics, costProvider func() (CostMetrics, error)) http.Handler {
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

		// Add cost metrics if provider is present
		if costProvider != nil {
			if cm, err := costProvider(); err == nil {
				resp["total_cost_usd"] = cm.TotalCostUSD
				resp["total_venues_processed"] = cm.TotalVenues
				resp["cost_per_venue_usd"] = cm.CostPerVenue
			} else {
				// Keep fields present for stability, even if provider errors
				resp["total_cost_usd"] = 0.0
				resp["total_venues_processed"] = int64(0)
				resp["cost_per_venue_usd"] = 0.0
			}
		}

		_ = json.NewEncoder(w).Encode(resp)
	})
}
