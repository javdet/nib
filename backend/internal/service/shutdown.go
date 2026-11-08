package service

import (
	"context"
	"errors"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ErrShuttingDown refuses new work once the process has been told to stop. It
// is a 503: nothing is wrong with the request, and the same request succeeds
// against the process that replaces this one.
var ErrShuttingDown = errors.New("nib is shutting down and accepts no new work; try again once it has restarted")

// restartInterruptedReason is what a run cut off by the end of a drain records,
// so the operator can tell it from a run somebody stopped, and knows Continue
// picks it up.
const restartInterruptedReason = "interrupted by a nib restart before it finished; Continue resumes it"

// drainPollInterval is how often a drain looks for work still running. The
// execution lease has no signal of its own to wait on, and a second is nothing
// next to a drain measured in minutes.
var drainPollInterval = time.Second

// drainLogInterval is how often a drain says what it is still waiting for.
const drainLogInterval = 30 * time.Second

// workTracker counts the background work a restart would cut off -- chat
// turns, fan-outs, investigations, the report and knowledge-base agents -- so a
// shutdown can wait for it instead of killing it mid-write.
//
// The execution lease is not tracked here: it already is the record of the one
// action, code action or code fix running, and a drain reads it directly.
type workTracker struct {
	mu sync.Mutex
	// draining refuses new work; stopped additionally cancels anything adopted
	// after the in-flight work was cancelled.
	draining bool
	stopped  bool
	next     uint64
	active   map[uint64]trackedWork
}

type trackedWork struct {
	kind   string
	cancel context.CancelFunc
}

// refuse returns ErrShuttingDown once a drain has begun.
func (t *workTracker) refuse() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.draining {
		return ErrShuttingDown
	}
	return nil
}

// begin registers new work, or refuses it once a drain has begun. Checking and
// registering under one lock is what keeps a drain from finding the process
// idle between the two.
func (t *workTracker) begin(kind string, cancel context.CancelFunc) (func(), error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.draining {
		return nil, ErrShuttingDown
	}
	return t.addLocked(kind, cancel), nil
}

// adopt registers work that continues something already in flight -- the
// knowledge-base update a finished report chains, an operator's answer to a
// question a running fan-out is waiting on -- which a drain lets through. Work
// adopted after the drain gave up is cancelled at once, so it ends through its
// own cancellation path rather than being killed with the process.
func (t *workTracker) adopt(kind string, cancel context.CancelFunc) func() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped && cancel != nil {
		cancel()
	}
	return t.addLocked(kind, cancel)
}

func (t *workTracker) addLocked(kind string, cancel context.CancelFunc) func() {
	if t.active == nil {
		t.active = make(map[uint64]trackedWork)
	}
	t.next++
	id := t.next
	t.active[id] = trackedWork{kind: kind, cancel: cancel}
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			delete(t.active, id)
			t.mu.Unlock()
		})
	}
}

func (t *workTracker) startDrain() {
	t.mu.Lock()
	t.draining = true
	t.mu.Unlock()
}

func (t *workTracker) isDraining() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.draining
}

func (t *workTracker) isStopped() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.stopped
}

// stop marks the drain over and cancels everything still registered.
func (t *workTracker) stop() {
	t.mu.Lock()
	t.draining = true
	t.stopped = true
	cancels := make([]context.CancelFunc, 0, len(t.active))
	for _, w := range t.active {
		if w.cancel != nil {
			cancels = append(cancels, w.cancel)
		}
	}
	t.mu.Unlock()
	for _, c := range cancels {
		c()
	}
}

// running names what is still registered, "kind×n", sorted for stable logs.
func (t *workTracker) running() []string {
	t.mu.Lock()
	counts := make(map[string]int, len(t.active))
	for _, w := range t.active {
		counts[w.kind]++
	}
	t.mu.Unlock()
	return countList(counts)
}

func countList(counts map[string]int) []string {
	out := make([]string, 0, len(counts))
	for kind, n := range counts {
		if n == 1 {
			out = append(out, kind)
			continue
		}
		out = append(out, kind+"×"+strconv.Itoa(n))
	}
	sort.Strings(out)
	return out
}

// ShuttingDown reports whether a drain has begun.
func (s *ChatService) ShuttingDown() bool {
	return s.work.isDraining()
}

// Drain stops new work from starting and waits until the work already running
// has finished, or ctx ends. It reports whether everything finished.
//
// Work in flight is let through to its end: a running action may still take
// steps, a fan-out may still start its next wave, and an operator may still
// answer a question one of them is waiting on. What is refused is anything new
// -- a chat message, an Execute, a fan-out, an investigation, the next item of
// a stage run.
func (s *ChatService) Drain(ctx context.Context) bool {
	s.work.startDrain()

	tick := time.NewTicker(drainPollInterval)
	defer tick.Stop()
	lastLog := time.Now()
	for {
		busy := s.inFlight()
		if len(busy) == 0 {
			return true
		}
		if time.Since(lastLog) >= drainLogInterval {
			lastLog = time.Now()
			slog.Info("shutdown: waiting for work in flight", "running", strings.Join(busy, ", "))
		}
		select {
		case <-ctx.Done():
			slog.Warn("shutdown: drain timed out", "running", strings.Join(busy, ", "))
			return false
		case <-tick.C:
		}
	}
}

// StopInFlight cancels whatever a drain left running and waits, until ctx
// ends, for it to record how it ended. A sub-agent action records itself as
// interrupted by the restart, which Continue picks up after the next boot. A
// container is left alone: it outlives this process either way, and the next
// boot's reconcile closes its row.
func (s *ChatService) StopInFlight(ctx context.Context) bool {
	s.work.stop()

	// Every sub-agent lease this process granted, and only those: one held by
	// another process is that process's to stop.
	s.leaseCancels.Range(func(_, v any) bool {
		v.(context.CancelFunc)()
		return true
	})

	tick := time.NewTicker(drainPollInterval / 4)
	defer tick.Stop()
	for {
		busy := s.inFlight(ExecutionKindContainer)
		if len(busy) == 0 {
			return true
		}
		select {
		case <-ctx.Done():
			slog.Warn("shutdown: work still running after cancellation", "running", strings.Join(busy, ", "))
			return false
		case <-tick.C:
		}
	}
}

// inFlight lists the work a shutdown is waiting for. Leases of the kinds in
// ignore do not count.
func (s *ChatService) inFlight(ignore ...ExecutionKind) []string {
	busy := s.work.running()

	s.execLeaseMu.Lock()
	held, ok := s.currentLease()
	s.execLeaseMu.Unlock()

	// A lease another process took is that process's to wait for.
	var lease string
	if ok && held.Owner == s.replicaID {
		lease = leaseDescription(held)
		for _, k := range ignore {
			if held.Kind == k {
				lease = ""
			}
		}
	}

	if lease != "" {
		busy = append(busy, lease)
	}
	return busy
}

func leaseDescription(l ExecutionLease) string {
	switch {
	case l.Fix:
		return "code fix"
	case l.Kind == ExecutionKindContainer:
		return "code action " + l.Number
	default:
		return "action " + l.Number
	}
}

// beginTurn registers a chat turn the operator started, or refuses it once a
// drain has begun. The returned context is cancelled when a drain gives up, so
// the turn ends through its own error path instead of with the process.
func (s *ChatService) beginTurn(ctx context.Context) (context.Context, func(), error) {
	ctx, cancel := context.WithCancel(ctx)
	done, err := s.work.begin("chat turn", cancel)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	return ctx, func() { done(); cancel() }, nil
}

// adoptTurn registers a turn that answers a question work in flight is waiting
// on, which a drain lets through: refusing it would only strand that work.
func (s *ChatService) adoptTurn(ctx context.Context) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := s.work.adopt("chat turn", cancel)
	return ctx, func() { done(); cancel() }
}
