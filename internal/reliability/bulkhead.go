package reliability

import (
	"context"
	"sync"
	"time"
)

// Bulkhead bounds how many calls to one dependency may be in flight at
// once, and how long a caller may wait for a slot before failing fast.
// Where the circuit breaker answers "is the dependency healthy?", the
// bulkhead answers "even while healthy, how much of this process may it
// occupy?" — a slow-but-upstream dependency can no longer pin unbounded
// goroutines and starve unrelated work.
//
// The classic failure it prevents: a webhook endpoint that normally answers
// in 50 ms starts answering in 30 s. With unbounded dispatch every timeout
// window holds 30 s × RPS goroutines + sockets; with a bulkhead of N, the
// excess rejects in milliseconds and callers move on (their own retry
// policy applies).
type Bulkhead struct {
	name           string
	slots          chan struct{}
	acquireTimeout time.Duration

	onInFlight func(name string, delta int)
	onRejected func(name string)
}

// NewBulkhead builds a bulkhead admitting at most limit concurrent calls.
// A limit below one is treated as one; a non-positive acquire timeout
// keeps callers waiting for a slot no longer than their own context.
func NewBulkhead(name string, limit int, acquireTimeout time.Duration) *Bulkhead {
	if limit < 1 {
		limit = 1
	}
	return &Bulkhead{
		name:           name,
		slots:          make(chan struct{}, limit),
		acquireTimeout: acquireTimeout,
	}
}

// Do runs fn while holding one slot. The wait for a slot is bounded by the
// smaller of the configured acquire timeout and ctx; on timeout it fails
// fast with ErrBulkheadFull without running fn.
func (b *Bulkhead) Do(ctx context.Context, fn func() error) error {
	timer := time.NewTimer(b.acquireTimeout)
	if b.acquireTimeout <= 0 {
		timer.Stop()
	}
	defer func() {
		if b.acquireTimeout > 0 {
			timer.Stop()
		}
	}()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		if b.onRejected != nil {
			b.onRejected(b.name)
		}
		return ErrBulkheadFull
	case b.slots <- struct{}{}:
	}
	if b.onInFlight != nil {
		b.onInFlight(b.name, 1)
	}
	defer func() {
		<-b.slots
		if b.onInFlight != nil {
			b.onInFlight(b.name, -1)
		}
	}()
	return fn()
}

// ErrBulkheadFull is returned when no slot freed up within the acquire
// timeout. Callers should surface it as backpressure (429/503), not as a
// dependency failure — the dependency may be perfectly healthy; the
// process is simply at capacity for it.
var ErrBulkheadFull = errBulkheadFull{}

type errBulkheadFull struct{}

func (errBulkheadFull) Error() string { return "bulkhead is full" }

// Bulkheads is a named set of identically configured bulkheads. A nil
// registry is valid: its Do runs the call directly, so optional wiring can
// pass nil — the same nil-safe convention as Breakers.
type Bulkheads struct {
	limit          int
	acquireTimeout time.Duration

	mu       sync.Mutex
	items    map[string]*Bulkhead
	onFlight func(name string, delta int)
	onReject func(name string)
}

// NewBulkheads builds a registry; every bulkhead it creates shares the
// limit and acquire timeout.
func NewBulkheads(limit int, acquireTimeout time.Duration) *Bulkheads {
	return &Bulkheads{limit: limit, acquireTimeout: acquireTimeout, items: map[string]*Bulkhead{}}
}

// SetObservers registers the metrics hooks applied to every bulkhead in the
// registry, including ones created later. delta is +1 on acquire, -1 on
// release; rejected fires once per failed slot wait.
func (r *Bulkheads) SetObservers(onInFlight func(name string, delta int), onRejected func(name string)) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onFlight = onInFlight
	r.onReject = onRejected
	for _, b := range r.items {
		b.onInFlight = onInFlight
		b.onRejected = onRejected
	}
}

// Get returns the bulkhead with the given name, creating it on first use.
func (r *Bulkheads) Get(name string) *Bulkhead {
	r.mu.Lock()
	defer r.mu.Unlock()
	if b, ok := r.items[name]; ok {
		return b
	}
	b := NewBulkhead(name, r.limit, r.acquireTimeout)
	b.onInFlight = r.onFlight
	b.onRejected = r.onReject
	r.items[name] = b
	return b
}

// Do runs fn under the named bulkhead. A nil registry runs fn directly.
func (r *Bulkheads) Do(ctx context.Context, name string, fn func() error) error {
	if r == nil {
		return fn()
	}
	return r.Get(name).Do(ctx, fn)
}
