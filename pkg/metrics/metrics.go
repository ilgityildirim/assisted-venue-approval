package metrics

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Simple, dependency-free metrics with Prometheus text exposition.
// Keep implementation minimal: atomic values, mutex-protected registries.

// Counter is a monotonically increasing number.
type Counter struct {
	name string
	help string
	val  int64 // use atomic
}

func (c *Counter) Inc(delta int64) { atomic.AddInt64(&c.val, delta) }
func (c *Counter) Add(delta int64) { c.Inc(delta) }
func (c *Counter) Get() int64      { return atomic.LoadInt64(&c.val) }

// Gauge is an arbitrary number that can go up and down.
type Gauge struct {
	name string
	help string
	val  int64  // store as int64 of float64 bits via math.Float64bits for speed? keep int64 but scale 1e6?
	f64  uint64 // store float64 atomically
}

func (g *Gauge) SetFloat64(v float64) { atomic.StoreUint64(&g.f64, mathFloat64bits(v)) }
func (g *Gauge) AddFloat64(delta float64) {
	for {
		old := atomic.LoadUint64(&g.f64)
		nv := mathFloat64frombits(old) + delta
		if atomic.CompareAndSwapUint64(&g.f64, old, mathFloat64bits(nv)) {
			return
		}
	}
}
func (g *Gauge) GetFloat64() float64 { return mathFloat64frombits(atomic.LoadUint64(&g.f64)) }

// Histogram with fixed buckets (cumulative counts per upper bound) and sum/count.
type Histogram struct {
	name    string
	help    string
	buckets []float64 // sorted ascending
	counts  []uint64  // atomics per bucket
	sum     uint64    // store float64 bits atomically
	count   uint64
}

func (h *Histogram) Observe(v float64) {
	// increment appropriate bucket
	for i, ub := range h.buckets {
		if v <= ub {
			atomic.AddUint64(&h.counts[i], 1)
			atomic.AddUint64(&h.count, 1)
			// add to sum
			for {
				old := atomic.LoadUint64(&h.sum)
				nv := mathFloat64frombits(old) + v
				if atomic.CompareAndSwapUint64(&h.sum, old, mathFloat64bits(nv)) {
					break
				}
			}
			return
		}
	}
	// if v greater than all buckets, count it in +Inf bucket (add an implicit +Inf bucket if not added)
	// Our design ensures last bucket should be +Inf if caller wanted; if not, add to last bucket anyway.
	last := len(h.counts) - 1
	atomic.AddUint64(&h.counts[last], 1)
	atomic.AddUint64(&h.count, 1)
	for {
		old := atomic.LoadUint64(&h.sum)
		nv := mathFloat64frombits(old) + v
		if atomic.CompareAndSwapUint64(&h.sum, old, mathFloat64bits(nv)) {
			break
		}
	}
}

// Registry holds all metrics.
type Registry struct {
	mu         sync.RWMutex
	counters   map[string]*Counter
	gauges     map[string]*Gauge
	histograms map[string]*Histogram
}

func NewRegistry() *Registry {
	return &Registry{
		counters:   make(map[string]*Counter),
		gauges:     make(map[string]*Gauge),
		histograms: make(map[string]*Histogram),
	}
}

var Default = NewRegistry()

func (r *Registry) Counter(name, help string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	if c, ok := r.counters[name]; ok {
		return c
	}
	c := &Counter{name: sanitize(name), help: help}
	r.counters[name] = c
	return c
}

func (r *Registry) Gauge(name, help string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	if g, ok := r.gauges[name]; ok {
		return g
	}
	g := &Gauge{name: sanitize(name), help: help}
	r.gauges[name] = g
	return g
}

func (r *Registry) Histogram(name, help string, buckets []float64) *Histogram {
	r.mu.Lock()
	defer r.mu.Unlock()
	if h, ok := r.histograms[name]; ok {
		return h
	}
	if len(buckets) == 0 || buckets[len(buckets)-1] != +Inf {
		// ensure +Inf bucket
		buckets = append(append([]float64{}, buckets...), +Inf)
	}
	sorted := append([]float64{}, buckets...)
	sort.Float64s(sorted)
	h := &Histogram{name: sanitize(name), help: help, buckets: sorted, counts: make([]uint64, len(sorted))}
	r.histograms[name] = h
	return h
}

// Handler returns an http.Handler that exposes metrics in Prometheus text format.
func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		// snapshot under RLock
		r.mu.RLock()
		// stable ordering for determinism
		cn := keys(r.counters)
		gn := keys(r.gauges)
		hn := keys(r.histograms)
		r.mu.RUnlock()

		for _, name := range cn {
			r.mu.RLock()
			c := r.counters[name]
			r.mu.RUnlock()
			if c == nil {
				continue
			}
			fmt.Fprintf(w, "# HELP %s %s\n", c.name, escapeHelp(c.help))
			fmt.Fprintf(w, "# TYPE %s counter\n", c.name)
			fmt.Fprintf(w, "%s %d\n", c.name, c.Get())
		}
		for _, name := range gn {
			r.mu.RLock()
			g := r.gauges[name]
			r.mu.RUnlock()
			if g == nil {
				continue
			}
			fmt.Fprintf(w, "# HELP %s %s\n", g.name, escapeHelp(g.help))
			fmt.Fprintf(w, "# TYPE %s gauge\n", g.name)
			fmt.Fprintf(w, "%s %g\n", g.name, g.GetFloat64())
		}
		for _, name := range hn {
			r.mu.RLock()
			h := r.histograms[name]
			r.mu.RUnlock()
			if h == nil {
				continue
			}
			fmt.Fprintf(w, "# HELP %s %s\n", h.name, escapeHelp(h.help))
			fmt.Fprintf(w, "# TYPE %s histogram\n", h.name)
			var cum uint64
			for i, ub := range h.buckets {
				c := atomic.LoadUint64(&h.counts[i])
				cum += c
				bname := fmt.Sprintf("%s_bucket{le=\"%g\"}", h.name, ub)
				if isInf(ub) {
					bname = fmt.Sprintf("%s_bucket{le=\"+Inf\"}", h.name)
				}
				fmt.Fprintf(w, "%s %d\n", bname, cum)
			}
			// sum and count
			sum := mathFloat64frombits(atomic.LoadUint64(&h.sum))
			fmt.Fprintf(w, "%s_sum %g\n", h.name, sum)
			fmt.Fprintf(w, "%s_count %d\n", h.name, atomic.LoadUint64(&h.count))
		}
	})
}

// Convenience: global HTTP handler for Default registry.
func Handler() http.Handler { return Default.Handler() }

// Utilities

func mathFloat64bits(v float64) uint64     { return math.Float64bits(v) }
func mathFloat64frombits(b uint64) float64 { return math.Float64frombits(b) }

const (
	Inf = 1e308 // large sentinel for +Inf label
)

func isInf(v float64) bool { return math.IsInf(v, 1) || v > 1e307 }

func sanitize(s string) string {
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	return s
}

func escapeHelp(s string) string {
	return strings.ReplaceAll(s, "\n", " ")
}

func keys[T any](m map[string]T) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// Minimal timer helper for histograms.
type Timer struct {
	h     *Histogram
	start time.Time
}

func (h *Histogram) Start() Timer { return Timer{h: h, start: time.Now()} }
func (t Timer) Observe() {
	if t.h != nil {
		t.h.Observe(time.Since(t.start).Seconds())
	}
}
