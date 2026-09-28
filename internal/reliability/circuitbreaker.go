package reliability

import (
	"context"
	"errors"
	"sync"
	"time"
)

// CircuitState names the three states of the breaker state machine: closed
// lets everything through, open rejects everything fast, half-open admits a
// single probe to decide between recovery and another open period.
type CircuitState int

// The three states of the breaker state machine.
const (
	StateClosed CircuitState = iota
	StateOpen
	StateHalfOpen
)

func (s CircuitState) String() string {
	switch s {
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

// Value maps the state onto a Prometheus gauge sample.
func (s CircuitState) Value() float64 {
	return float64(s)
}

// ErrCircuitOpen is returned instead of running the protected call while the
// breaker is open. Callers should treat it as retry-later, not as a failure
// of the dependency itself.
var ErrCircuitOpen = errors.New("circuit breaker is open")

// Breaker trips open after `threshold` consecutive failures, stays open for
// `openTimeout`, then admits exactly one probe call (half-open). A successful
// probe closes the breaker again; a failed one reopens it for another period.
// All methods are safe for concurrent use.
type Breaker struct {
	name        string
	threshold   int
	openTimeout time.Duration

	mu       sync.Mutex
	state    CircuitState
	failures int
	openedAt time.Time
	probing  bool

	onChange func(name string, from, to CircuitState)
}

// NewBreaker builds a breaker with the given trip threshold and open
// cool-down. Thresholds below one are treated as one.
func NewBreaker(name string, threshold int, openTimeout time.Duration) *Breaker {
	if threshold < 1 {
		threshold = 1
	}
	if openTimeout <= 0 {
		openTimeout = time.Second
	}
	return &Breaker{name: name, threshold: threshold, openTimeout: openTimeout}
}

// OnStateChange registers a single transition observer; the last registration
// wins. Observers run while the breaker is locked, so they must not call back
// into the breaker.
func (b *Breaker) OnStateChange(fn func(name string, from, to CircuitState)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onChange = fn
}

// Allow reports whether a call may proceed now, or ErrCircuitOpen. When the
// open cool-down has elapsed it admits one probe and flips to half-open.
func (b *Breaker) Allow() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	switch b.state {
	case StateClosed:
		return nil
	case StateOpen:
		if time.Since(b.openedAt) < b.openTimeout || b.probing {
			return ErrCircuitOpen
		}
		b.probing = true
		b.transitionLocked(StateHalfOpen)
		return nil
	default: // half-open: a probe is already in flight
		return ErrCircuitOpen
	}
}

// Do runs fn under the breaker. While open it fails fast with ErrCircuitOpen
// without calling fn. Context cancellation is returned without recording an
// outcome so shutdowns never trip the breaker.
func (b *Breaker) Do(ctx context.Context, fn func() error) error {
	if err := b.Allow(); err != nil {
		return err
	}
	err := fn()
	if err == nil {
		b.RecordSuccess()
		return nil
	}
	if ctx.Err() != nil {
		return err
	}
	b.RecordFailure()
	return err
}

// RecordSuccess registers a completed call and closes the breaker.
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	b.failures = 0
	if b.state != StateClosed {
		b.transitionLocked(StateClosed)
	}
}

// RecordFailure registers a failed call; enough consecutive failures trip the
// breaker open, and a failed half-open probe reopens it immediately.
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.probing = false
	if b.state == StateHalfOpen {
		b.openedAt = time.Now()
		b.transitionLocked(StateOpen)
		return
	}
	b.failures++
	if b.state == StateClosed && b.failures >= b.threshold {
		b.openedAt = time.Now()
		b.transitionLocked(StateOpen)
	}
}

// State reports the current state.
func (b *Breaker) State() CircuitState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

func (b *Breaker) transitionLocked(to CircuitState) {
	from := b.state
	if from == to {
		return
	}
	b.state = to
	if b.onChange != nil {
		b.onChange(b.name, from, to)
	}
}

// Breakers is a named set of identically configured breakers. A nil registry
// is valid: its Do runs the call directly, so optional wiring can pass nil.
type Breakers struct {
	threshold   int
	openTimeout time.Duration
	onChange    func(name string, from, to CircuitState)

	mu    sync.Mutex
	items map[string]*Breaker
}

// NewBreakers builds a registry; every breaker it creates shares the
// threshold and cool-down.
func NewBreakers(threshold int, openTimeout time.Duration) *Breakers {
	return &Breakers{threshold: threshold, openTimeout: openTimeout, items: map[string]*Breaker{}}
}

// SetOnChange registers a transition observer applied to every breaker in
// the registry, including ones created later — the metrics wiring calls it
// once at startup.
func (r *Breakers) SetOnChange(fn func(name string, from, to CircuitState)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onChange = fn
	for _, b := range r.items {
		b.OnStateChange(fn)
	}
}

// Get returns the breaker with the given name, creating it on first use.
func (r *Breakers) Get(name string) *Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.items == nil {
		r.items = map[string]*Breaker{}
	}
	if b, ok := r.items[name]; ok {
		return b
	}
	b := NewBreaker(name, r.threshold, r.openTimeout)
	if r.onChange != nil {
		b.OnStateChange(r.onChange)
	}
	r.items[name] = b
	return b
}

// Items snapshots the registry, for wiring metrics at startup.
func (r *Breakers) Items() map[string]*Breaker {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]*Breaker, len(r.items))
	for name, b := range r.items {
		out[name] = b
	}
	return out
}

// Do runs fn under the named breaker. A nil registry runs fn directly.
func (r *Breakers) Do(ctx context.Context, name string, fn func() error) error {
	if r == nil {
		return fn()
	}
	return r.Get(name).Do(ctx, fn)
}
