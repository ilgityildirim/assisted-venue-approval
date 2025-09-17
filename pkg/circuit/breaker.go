package circuit

import (
	"context"
	"errors"
	"sync"
	"time"

	"assisted-venue-approval/pkg/logging"
	"assisted-venue-approval/pkg/metrics"
)

// State represents the circuit breaker state
// Closed: normal operation; HalfOpen: testing; Open: fail fast
// Keep enums simple for logging/metrics.
type State int

const (
	Closed State = iota
	Open
	HalfOpen
)

// Config tunes a circuit breaker instance.
type Config struct {
	Name string

	OperationTimeout    time.Duration // per-call timeout
	OpenFor             time.Duration // how long to stay open before probing
	MaxConsecFailures   int           // consecutive failures to open
	WindowSize          int           // sliding window of recent calls
	FailureRate         float64       // 0..1 fraction in window to open
	SlowCallThreshold   time.Duration // duration over which a call is considered slow
	SlowCallRate        float64       // 0..1 fraction in window to open
	HalfOpenMaxInFlight int           // usually 1
}

// ErrOpen indicates the breaker is open and calls are short-circuited.
var ErrOpen = errors.New("circuit open")

// result in the ring buffer
type sample struct {
	success bool
	slow    bool
}

type Breaker struct {
	cfg        Config
	mu         sync.Mutex
	st         State
	lastChange time.Time
	nextProbe  time.Time
	consecFail int

	win  []sample
	idx  int
	used int

	log *logging.Logger
	// metrics
	mState    *metrics.Gauge
	mOpen     *metrics.Counter
	mHalfOpen *metrics.Counter
	mSuccess  *metrics.Counter
	mFailure  *metrics.Counter
	mTimeout  *metrics.Counter
	mSlow     *metrics.Counter
	mLatency  *metrics.Histogram
}

func New(cfg Config, log *logging.Logger) *Breaker {
	if cfg.WindowSize <= 0 {
		cfg.WindowSize = 20
	}
	if cfg.HalfOpenMaxInFlight <= 0 {
		cfg.HalfOpenMaxInFlight = 1
	}
	b := &Breaker{
		cfg:        cfg,
		st:         Closed,
		lastChange: time.Now(),
		win:        make([]sample, cfg.WindowSize),
		log:        log,
		mState:     metrics.Default.Gauge("cb_"+cfg.Name+"_state", "Circuit breaker state (0=closed,1=open,2=half-open)"),
		mOpen:      metrics.Default.Counter("cb_"+cfg.Name+"_opens", "Circuit opened events"),
		mHalfOpen:  metrics.Default.Counter("cb_"+cfg.Name+"_half_open", "Circuit half-open transitions"),
		mSuccess:   metrics.Default.Counter("cb_"+cfg.Name+"_success", "Successful calls through circuit"),
		mFailure:   metrics.Default.Counter("cb_"+cfg.Name+"_failure", "Failed calls through circuit"),
		mTimeout:   metrics.Default.Counter("cb_"+cfg.Name+"_timeout", "Timed out calls"),
		mSlow:      metrics.Default.Counter("cb_"+cfg.Name+"_slow", "Slow calls"),
		mLatency:   metrics.Default.Histogram("cb_"+cfg.Name+"_latency_ms", "Latency of calls (ms)", []float64{10, 25, 50, 100, 200, 500, 1000, 2000, 5000}),
	}
	b.mState.SetFloat64(0)
	return b
}

func (b *Breaker) stateLocked() State { return b.st }

func (b *Breaker) setStateLocked(st State) {
	if b.st == st {
		return
	}
	b.st = st
	b.lastChange = time.Now()
	switch st {
	case Open:
		b.mOpen.Inc(1)
		b.mState.SetFloat64(1)
	case HalfOpen:
		b.mHalfOpen.Inc(1)
		b.mState.SetFloat64(2)
	case Closed:
		b.mState.SetFloat64(0)
	}
	if b.log != nil {
		b.log.WithComponent("circuit").Info("breaker state change", logging.String("name", b.cfg.Name), logging.Int("state", int(st)))
	}
}

// record adds a sample into ring and checks thresholds
func (b *Breaker) record(success bool, slow bool) {
	b.win[b.idx] = sample{success: success, slow: slow}
	if b.used < len(b.win) {
		b.used++
	}
	b.idx = (b.idx + 1) % len(b.win)

	// compute rates
	fail := 0
	slowN := 0
	for i := 0; i < b.used; i++ {
		if !b.win[i].success {
			fail++
		}
		if b.win[i].slow {
			slowN++
		}
	}
	failRate := 0.0
	slowRate := 0.0
	if b.used > 0 {
		failRate = float64(fail) / float64(b.used)
		slowRate = float64(slowN) / float64(b.used)
	}

	if b.stateLocked() == Closed {
		if b.cfg.MaxConsecFailures > 0 && b.consecFail >= b.cfg.MaxConsecFailures {
			b.setStateLocked(Open)
			b.nextProbe = time.Now().Add(b.cfg.OpenFor)
			return
		}
		if b.cfg.FailureRate > 0 && failRate >= b.cfg.FailureRate {
			b.setStateLocked(Open)
			b.nextProbe = time.Now().Add(b.cfg.OpenFor)
			return
		}
		if b.cfg.SlowCallRate > 0 && slowRate >= b.cfg.SlowCallRate {
			b.setStateLocked(Open)
			b.nextProbe = time.Now().Add(b.cfg.OpenFor)
			return
		}
	}
}

// Do runs op under breaker. If open, runs fallback if provided, otherwise returns ErrOpen.
// op should return error only; any outputs can be captured via closure vars.
func (b *Breaker) Do(ctx context.Context, op func(ctx context.Context) error, fallback func(ctx context.Context, cause error) error) error {
	// fast path check
	b.mu.Lock()
	st := b.stateLocked()
	if st == Open {
		if time.Now().Before(b.nextProbe) {
			b.mu.Unlock()
			if fallback != nil {
				return fallback(ctx, ErrOpen)
			}
			return ErrOpen
		}
		// move to half-open for a probe
		b.setStateLocked(HalfOpen)
	}
	b.mu.Unlock()

	// apply timeout
	if b.cfg.OperationTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, b.cfg.OperationTimeout)
		defer cancel()
	}

	start := time.Now()
	err := op(ctx)
	dur := time.Since(start)
	durMs := float64(dur / time.Millisecond)
	b.mLatency.Observe(durMs)
	if dur > b.cfg.SlowCallThreshold && b.cfg.SlowCallThreshold > 0 {
		b.mSlow.Inc(1)
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	// timeout detection
	if errors.Is(err, context.DeadlineExceeded) || (ctx.Err() == context.DeadlineExceeded) {
		b.mTimeout.Inc(1)
	}

	slow := b.cfg.SlowCallThreshold > 0 && dur > b.cfg.SlowCallThreshold

	if err != nil {
		b.consecFail++
		b.mFailure.Inc(1)
		b.record(false, slow)
		if b.stateLocked() == HalfOpen {
			// probe failed -> open
			b.setStateLocked(Open)
			b.nextProbe = time.Now().Add(b.cfg.OpenFor)
		}
		if fallback != nil {
			return fallback(ctx, err)
		}
		return err
	}

	// success
	b.consecFail = 0
	b.mSuccess.Inc(1)
	b.record(true, slow)
	if b.stateLocked() == HalfOpen {
		b.setStateLocked(Closed)
	}
	return nil
}
