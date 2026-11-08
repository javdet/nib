package repository

import (
	"context"
	"time"
)

// Replica is one running backend process.
type Replica struct {
	ID       string
	Hostname string
	Version  string
}

// ReplicaRepository is the registry of live backend processes.
type ReplicaRepository interface {
	// RegisterReplica records r as alive now, replacing any row of the same id.
	RegisterReplica(ctx context.Context, r Replica) error
	// HeartbeatReplica marks id alive now. It reports false when the row is
	// gone -- another process judged this one dead and swept it -- so the
	// caller can register again.
	HeartbeatReplica(ctx context.Context, id string) (bool, error)
	DeregisterReplica(ctx context.Context, id string) error
	// RetireReplicasOnHost removes every process registered from hostname
	// except keep, and returns their ids.
	RetireReplicasOnHost(ctx context.Context, hostname, keep string) ([]string, error)
	// ListLiveReplicas returns every process that heartbeat within maxAge.
	ListLiveReplicas(ctx context.Context, maxAge time.Duration) ([]string, error)
	// DeleteStaleReplicas removes every process whose last heartbeat is older
	// than maxAge and returns their ids.
	DeleteStaleReplicas(ctx context.Context, maxAge time.Duration) ([]string, error)
}
