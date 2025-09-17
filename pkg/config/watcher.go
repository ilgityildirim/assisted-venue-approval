package config

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"assisted-venue-approval/pkg/metrics"
)

// Change describes a configuration update event.
// Only a subset of fields may have changed; see Fields for the list of keys.
type Change struct {
	Old    *Config
	New    *Config
	Fields []string
	Err    error
}

// Subscriber channel buffer size; small to apply back-pressure if receivers are slow.
const subBuf = 4

// Watcher periodically reloads configuration from environment and optional file.
// If CONFIG_FILE is set and points to a .env-like file, it will reload environment
// variables from that file when its mtime changes.
//
// Keep it simple: polling interval-based. TODO: consider fsnotify if needed.
type Watcher struct {
	mu        sync.RWMutex
	cur       *Config
	closed    bool
	intv      time.Duration
	subs      []chan Change
	cancel    context.CancelFunc
	filePath  string
	lastMTime time.Time

	// metrics
	mReloads  *metrics.Counter
	mFailures *metrics.Counter
}

func NewWatcher(interval time.Duration) *Watcher {
	fp := strings.TrimSpace(os.Getenv("CONFIG_FILE"))
	w := &Watcher{
		intv:      interval,
		filePath:  fp,
		mReloads:  metrics.Default.Counter("config_reload_total", "Total number of config reload attempts"),
		mFailures: metrics.Default.Counter("config_reload_failures_total", "Total number of failed config reloads"),
	}
	w.cur = Load()
	return w
}

// Subscribe returns a channel to receive Change notifications.
// Caller should drain the channel until it is closed.
func (w *Watcher) Subscribe() <-chan Change {
	w.mu.Lock()
	defer w.mu.Unlock()
	ch := make(chan Change, subBuf)
	w.subs = append(w.subs, ch)
	return ch
}

// Close stops the watcher and closes subscriber channels.
func (w *Watcher) Close() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	// cancel loop if running
	if w.cancel != nil {
		w.cancel()
	}
	// close subs
	for _, s := range w.subs {
		close(s)
	}
	w.subs = nil
	w.mu.Unlock()
}

// Start begins polling in a goroutine. It is safe to call once.
func (w *Watcher) Start() {
	w.mu.Lock()
	if w.cancel != nil {
		w.mu.Unlock()
		return // already started
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.mu.Unlock()

	go w.loop(ctx)
}

func (w *Watcher) loop(ctx context.Context) {
	t := time.NewTicker(w.intv)
	defer t.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			w.checkOnce()
		}
	}
}

func (w *Watcher) checkOnce() {
	// Optional: reload .env file if changed
	if w.filePath != "" {
		if fi, err := os.Stat(w.filePath); err == nil {
			mt := fi.ModTime()
			if mt.After(w.lastMTime) {
				_ = w.loadDotEnv(w.filePath)
				w.lastMTime = mt
			}
		}
	}

	newCfg := Load()
	if err := newCfg.Validate(); err != nil {
		w.mFailures.Inc(1)
		w.notify(Change{Old: w.cur, New: newCfg, Err: fmt.Errorf("invalid config: %w", err)})
		return
	}

	// compute diffs of selected keys
	fields := diffKeys(w.cur, newCfg)
	if len(fields) == 0 {
		return
	}

	w.mReloads.Inc(1)
	w.mu.Lock()
	w.cur = newCfg
	w.mu.Unlock()
	w.notify(Change{Old: w.cur, New: newCfg, Fields: fields})
}

func (w *Watcher) notify(chg Change) {
	w.mu.RLock()
	subs := append([]chan Change(nil), w.subs...)
	w.mu.RUnlock()
	for _, s := range subs {
		select {
		case s <- chg:
		default:
			// drop if slow; keep system moving
		}
	}
}

func diffKeys(a, b *Config) []string {
	if a == nil || b == nil {
		return []string{"all"}
	}
	var f []string
	appendIf := func(cond bool, name string) {
		if cond {
			f = append(f, name)
		}
	}
	appendIf(a.ApprovalThreshold != b.ApprovalThreshold, "ApprovalThreshold")
	appendIf(a.WorkerCount != b.WorkerCount, "WorkerCount")
	appendIf(a.LogLevel != b.LogLevel, "LogLevel")
	appendIf(a.LogFormat != b.LogFormat, "LogFormat")
	appendIf(a.EnableFileLogging != b.EnableFileLogging, "EnableFileLogging")
	appendIf(a.MetricsEnabled != b.MetricsEnabled || a.MetricsPath != b.MetricsPath, "Metrics")
	appendIf(a.ProfilingEnabled != b.ProfilingEnabled || a.ProfilingPort != b.ProfilingPort, "Profiling")
	// Add others as needed
	return f
}

// Very small .env parser (KEY=VALUE per line, # comments). Not robust, but fine.
func (w *Watcher) loadDotEnv(path string) error {
	f, err := os.Open(filepath.Clean(path))
	if err != nil {
		return err
	}
	defer f.Close()
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kv := strings.SplitN(line, "=", 2)
		if len(kv) != 2 {
			continue
		}
		k := strings.TrimSpace(kv[0])
		v := strings.TrimSpace(kv[1])
		v = strings.Trim(v, "\"'")
		_ = os.Setenv(k, v)
	}
	return nil
}
