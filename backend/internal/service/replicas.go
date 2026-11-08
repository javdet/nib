package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/javdet/nib/internal/metrics"
	"github.com/javdet/nib/internal/repository"
)

// Coordination timings. A process is presumed dead after missing several
// heartbeats in a row, not one: closing a live process's runs as orphaned is
// far worse than leaving a dead one's open for an extra minute.
const (
	replicaHeartbeatInterval = 10 * time.Second
	replicaDeadAfter         = 60 * time.Second
	replicaSweepInterval     = 60 * time.Second
	// subagentLeaseTTL is how long a sub-agent's lease outlives the last
	// heartbeat of its process. Longer than the heartbeat interval by a wide
	// margin, so one slow heartbeat never frees a slot that is still in use.
	subagentLeaseTTL = 90 * time.Second
)

// NewReplicaID names a process: the host it runs on, which is the pod name on
// Kubernetes and the container id under Compose, plus a suffix fresh on every
// start, so a restarted process is never mistaken for the one before it.
func NewReplicaID(hostname string) string {
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	hostname = strings.TrimSpace(hostname)
	if hostname == "" {
		hostname = "nib"
	}
	return hostname + "-" + hex.EncodeToString(b)
}

// SetReplica names this process and turns on the lease heartbeat. Everything
// it starts from then on is owned by id.
func (s *ChatService) SetReplica(id string) {
	s.replicaID = id
	s.leaseTTL = subagentLeaseTTL
}

// orphanReconciler closes command runs whose process is gone.
type orphanReconciler interface {
	ReconcileLost(ctx context.Context, liveOwners []string) (int64, error)
}

// ReplicaCoordinator keeps this process registered as alive, and closes the
// runs of processes that are not.
type ReplicaCoordinator struct {
	repo     repository.ReplicaRepository
	chat     *ChatService
	commands orphanReconciler
	self     repository.Replica
}

func NewReplicaCoordinator(
	repo repository.ReplicaRepository,
	chat *ChatService,
	commands orphanReconciler,
	self repository.Replica,
) *ReplicaCoordinator {
	return &ReplicaCoordinator{repo: repo, chat: chat, commands: commands, self: self}
}

// Register records this process as alive. It runs before anything is served,
// so no run is ever owned by a process the registry does not know.
//
// A process registered from the same host is the one this replaced -- a
// container restarted in place keeps its hostname, and two live processes do
// not share one -- so it is retired at once and its runs swept at boot, rather
// than a heartbeat window later.
func (c *ReplicaCoordinator) Register(ctx context.Context) error {
	if c.self.Hostname != "" {
		retired, err := c.repo.RetireReplicasOnHost(ctx, c.self.Hostname, c.self.ID)
		if err != nil {
			return err
		}
		for _, id := range retired {
			slog.Info("retired the previous backend process on this host", "replica_id", id)
		}
	}
	if err := c.repo.RegisterReplica(ctx, c.self); err != nil {
		return err
	}
	slog.Info("registered backend process", "replica_id", c.self.ID)
	return nil
}

// Deregister removes this process from the registry on a clean exit, so its
// peers sweep whatever it left behind at their next pass rather than a
// heartbeat window later.
func (c *ReplicaCoordinator) Deregister(ctx context.Context) {
	if err := c.repo.DeregisterReplica(ctx, c.self.ID); err != nil {
		slog.Warn("deregister backend process", "replica_id", c.self.ID, "error", err)
	}
}

// SweepResult is what one pass closed.
type SweepResult struct {
	Runs        ReconcileResult
	CommandRuns int64
	// Dead lists the processes this pass found dead and removed.
	Dead []string
}

// Sweep removes the processes that stopped heartbeating and closes every run
// owned by a process that is not alive.
func (c *ReplicaCoordinator) Sweep(ctx context.Context) (SweepResult, error) {
	var out SweepResult

	dead, err := c.repo.DeleteStaleReplicas(ctx, replicaDeadAfter)
	if err != nil {
		return out, err
	}
	out.Dead = dead
	for _, id := range dead {
		slog.Warn("backend process stopped heartbeating, closing its runs", "replica_id", id)
	}

	live, err := c.repo.ListLiveReplicas(ctx, replicaDeadAfter)
	if err != nil {
		return out, err
	}
	liveSet := map[string]bool{c.self.ID: true}
	for _, id := range live {
		liveSet[id] = true
	}
	owners := make([]string, 0, len(liveSet))
	for id := range liveSet {
		owners = append(owners, id)
	}

	runs, err := c.chat.SweepOrphanedRuns(func(owner string) bool { return liveSet[owner] })
	if err != nil {
		return out, fmt.Errorf("sweep runs: %w", err)
	}
	out.Runs = runs

	if c.commands != nil {
		n, err := c.commands.ReconcileLost(ctx, owners)
		if err != nil {
			return out, fmt.Errorf("sweep command runs: %w", err)
		}
		out.CommandRuns = n
	}
	return out, nil
}

// Run heartbeats and sweeps until ctx ends.
//
// The two run on loops of their own: a sweep waits on record locks and reads
// every running document, and a heartbeat stuck behind it would let this
// process's own leases expire and its peers take it for dead.
func (c *ReplicaCoordinator) Run(ctx context.Context) {
	go func() {
		heartbeat := time.NewTicker(replicaHeartbeatInterval)
		defer heartbeat.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-heartbeat.C:
				c.beat(ctx)
			}
		}
	}()

	sweep := time.NewTicker(replicaSweepInterval)
	defer sweep.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-sweep.C:
			sweepCtx, cancel := context.WithTimeout(ctx, replicaSweepInterval)
			res, err := c.Sweep(sweepCtx)
			cancel()
			if err != nil {
				slog.Warn("sweep orphaned runs", "error", err)
				continue
			}
			if res.Runs.Total() > 0 || res.CommandRuns > 0 {
				slog.Info("closed runs of backend processes that are gone",
					"actions", res.Runs.Actions, "fanouts", res.Runs.Fanouts,
					"code_fixes", res.Runs.CodeFixes, "stage_runs", res.Runs.StageRuns,
					"incidents", res.Runs.Incidents, "command_runs", res.CommandRuns)
			}
		}
	}
}

func (c *ReplicaCoordinator) beat(ctx context.Context) {
	callCtx, cancel := context.WithTimeout(ctx, replicaHeartbeatInterval)
	defer cancel()

	ok, err := c.repo.HeartbeatReplica(callCtx, c.self.ID)
	if err != nil {
		slog.Warn("backend process heartbeat", "replica_id", c.self.ID, "error", err)
		return
	}
	if !ok {
		// A peer took this process for dead -- a heartbeat stalled past the
		// window -- and may have closed runs it is still doing. Nothing can
		// undo that; registering again at least stops it happening twice.
		slog.Error("this backend process was swept as dead while running; registering again",
			"replica_id", c.self.ID)
		if err := c.repo.RegisterReplica(callCtx, c.self); err != nil {
			slog.Warn("register backend process again", "error", err)
		}
	}
	c.chat.extendOwnLeases(callCtx)
	_, held := c.chat.currentLease()
	metrics.SetLeaseHeld(held)
}
