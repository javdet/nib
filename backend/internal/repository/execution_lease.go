package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// LeaseRecord is the stored form of the one execution lease.
type LeaseRecord struct {
	// Token identifies one grant, so a release or a stamp can only ever touch
	// its own.
	Token uuid.UUID
	// Owner is the backend process that took it.
	Owner string
	// Holder is the lease's own description, opaque to storage.
	Holder []byte
	// ExpiresAt is when the lease stops counting as held. The zero time never
	// expires.
	ExpiresAt time.Time
}

// Expired reports whether the lease no longer counts as held at now.
func (r LeaseRecord) Expired(now time.Time) bool {
	return !r.ExpiresAt.IsZero() && now.After(r.ExpiresAt)
}

// ExecutionLeaseRepository stores the one execution lease. Every write is a
// compare-and-set on the token, so two processes racing for the slot, or a
// late release racing a new grant, can never both win.
type ExecutionLeaseRepository interface {
	// TryAcquireLease stores rec when the slot is empty and reports whether it
	// did; otherwise it returns the holder. It never replaces a holder, expired
	// or not: expiring one is DeleteLease, by the caller that handled it.
	TryAcquireLease(ctx context.Context, rec LeaseRecord) (bool, LeaseRecord, error)
	// GetLease returns ErrNotFound when the slot is empty.
	GetLease(ctx context.Context) (LeaseRecord, error)
	// UpdateLeaseHolder replaces the holder of the grant token, reporting false
	// when that grant is no longer the one stored.
	UpdateLeaseHolder(ctx context.Context, token uuid.UUID, holder []byte) (bool, error)
	// ExtendLease moves the expiry of the grant token.
	ExtendLease(ctx context.Context, token uuid.UUID, expiresAt time.Time) (bool, error)
	// DeleteLease drops the grant token, reporting false when it was not the
	// one stored.
	DeleteLease(ctx context.Context, token uuid.UUID) (bool, error)
}
