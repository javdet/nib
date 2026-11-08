package service

import (
	"context"
	"log/slog"
	"sync"
)

// DistributedLocker takes locks that hold across every backend process sharing
// the database. Postgres advisory locks implement it; without one the service
// keeps its locks process-local, which is exactly right for a single replica.
type DistributedLocker interface {
	// Acquire blocks until the lock named key is held or ctx ends.
	Acquire(ctx context.Context, key string) (release func(), err error)
	// TryAcquire takes the lock only if nobody holds it.
	TryAcquire(ctx context.Context, key string) (release func(), ok bool, err error)
}

// SetDistributedLocker makes every named lock hold across processes.
func (s *ChatService) SetDistributedLocker(l DistributedLocker) {
	s.distLocker = l
}

// namedLock is the lock behind every read-modify-write of shared state: an
// in-process slot first, so goroutines of one process queue in memory rather
// than each pinning a database connection, then the distributed lock when one
// is configured.
//
// A distributed lock that cannot be taken because the database is unreachable
// is logged and skipped rather than retried forever: the read and write the
// lock guards go to the same database and fail on their own, and a lock that
// blocks indefinitely would wedge the agent loop instead of failing it.
type namedLock struct {
	key     string
	slot    chan struct{}
	dist    DistributedLocker
	release func()
}

func newNamedLock(key string, dist DistributedLocker) *namedLock {
	return &namedLock{key: key, slot: make(chan struct{}, 1), dist: dist}
}

// Lock satisfies sync.Locker, so a call site written against a sync.Mutex
// keeps working unchanged.
func (l *namedLock) Lock() {
	_ = l.LockContext(context.Background())
}

// LockContext waits for the lock until ctx ends.
func (l *namedLock) LockContext(ctx context.Context) error {
	select {
	case l.slot <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	if l.dist == nil {
		return nil
	}
	release, err := l.dist.Acquire(ctx, l.key)
	if err != nil {
		if ctx.Err() != nil {
			<-l.slot
			return ctx.Err()
		}
		slog.Warn("distributed lock unavailable, holding the in-process lock only",
			"key", l.key, "error", err)
		return nil
	}
	l.release = release
	return nil
}

// TryLock takes the lock only if it is free both here and in every other
// process.
func (l *namedLock) TryLock() bool {
	select {
	case l.slot <- struct{}{}:
	default:
		return false
	}
	if l.dist == nil {
		return true
	}
	release, ok, err := l.dist.TryAcquire(context.Background(), l.key)
	if err != nil {
		slog.Warn("distributed lock unavailable, holding the in-process lock only",
			"key", l.key, "error", err)
		return true
	}
	if !ok {
		<-l.slot
		return false
	}
	l.release = release
	return true
}

// Unlock releases what Lock took. The release func is read while the slot is
// still held, so the next holder cannot overwrite it first.
func (l *namedLock) Unlock() {
	release := l.release
	l.release = nil
	if release != nil {
		release()
	}
	select {
	case <-l.slot:
	default:
	}
}

// lockFor returns the lock named key, the same one for every caller in this
// process.
func (s *ChatService) lockFor(key string) *namedLock {
	if v, ok := s.namedLocks.Load(key); ok {
		return v.(*namedLock)
	}
	v, _ := s.namedLocks.LoadOrStore(key, newNamedLock(key, s.distLocker))
	return v.(*namedLock)
}

var _ sync.Locker = (*namedLock)(nil)
