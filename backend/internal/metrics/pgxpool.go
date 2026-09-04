package metrics

import "github.com/jackc/pgx/v5/pgxpool"

// PgxPoolStats adapts a pgx pool to the stats func RegisterPool takes. This is
// the only file in the package that imports pgx.
func PgxPoolStats(p *pgxpool.Pool) func() PoolStats {
	return func() PoolStats {
		s := p.Stat()
		return PoolStats{
			Total:               s.TotalConns(),
			Acquired:            s.AcquiredConns(),
			Idle:                s.IdleConns(),
			Constructing:        s.ConstructingConns(),
			Max:                 s.MaxConns(),
			Acquires:            s.AcquireCount(),
			EmptyAcquires:       s.EmptyAcquireCount(),
			CanceledAcquires:    s.CanceledAcquireCount(),
			NewConns:            s.NewConnsCount(),
			MaxLifetimeDestroys: s.MaxLifetimeDestroyCount(),
			MaxIdleDestroys:     s.MaxIdleDestroyCount(),
			AcquireWait:         s.AcquireDuration(),
			EmptyAcquireWait:    s.EmptyAcquireWaitTime(),
		}
	}
}
