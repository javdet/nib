package service

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/repository/postgres"
)

// These tests run two ChatServices against one database, the way two replicas
// of the backend would, and check that what used to be process-local holds
// across them. They need a real server: NIB_SERVICE_TEST_DSN names one
// (pgvector/pgvector:pg18), and they skip without it. Its public schema is
// reset, so it must not be shared with the repository package's tests.

func servicePGPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("NIB_SERVICE_TEST_DSN"))
	if dsn == "" {
		t.Skip("NIB_SERVICE_TEST_DSN is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	t.Cleanup(pool.Close)
	if _, err := pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatalf("reset schema: %v", err)
	}
	files, err := filepath.Glob(filepath.Join("..", "..", "migrations", "*.up.sql"))
	if err != nil || len(files) == 0 {
		t.Fatalf("find migrations: %v", err)
	}
	sort.Strings(files)
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		if _, err := pool.Exec(ctx, string(body)); err != nil {
			t.Fatalf("apply %s: %v", filepath.Base(f), err)
		}
	}
	return pool
}

// replica builds one "process": a ChatService wired to the shared database the
// way main.go wires it, listening on the bus until the test ends.
func replica(t *testing.T, pool *pgxpool.Pool, id string) *ChatService {
	t.Helper()
	dir := t.TempDir()
	svc := &ChatService{
		actionExec:      ActionExecConfig{}.withDefaults(),
		activity:        NewActivityBroker(),
		actionPlansDir:  filepath.Join(dir, "action_plans"),
		planStateDir:    filepath.Join(dir, "plan_state"),
		planFanoutDir:   filepath.Join(dir, "plan_fanout"),
		subagentsDir:    filepath.Join(dir, "subagents"),
		codeFixesDir:    filepath.Join(dir, "code_fixes"),
		incidentRunsDir: filepath.Join(dir, "incident_runs"),
	}
	svc.SetReplica(id)
	svc.SetDistributedLocker(postgres.NewAdvisoryLocker(pool))
	svc.SetRunStateStore(postgres.NewRunStateRepo(pool))
	svc.SetExecutionLeaseStore(postgres.NewExecutionLeaseRepo(pool))

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	bus := postgres.NewNotifyBus(pool, pool, BusChannelActivity, BusChannelControl)
	go bus.Run(ctx, svc.HandleBusMessage)
	svc.SetMessageBus(bus)
	deadline := time.Now().Add(5 * time.Second)
	for !bus.Listening() {
		if time.Now().After(deadline) {
			t.Fatal("bus never started listening")
		}
		time.Sleep(10 * time.Millisecond)
	}
	return svc
}

func pgDialog(t *testing.T, pool *pgxpool.Pool) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := pool.QueryRow(context.Background(),
		`INSERT INTO chat_dialogs (mode, title) VALUES ('main', 'test') RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("insert dialog: %v", err)
	}
	return id
}

func TestReplicasShareOneExecutionLease(t *testing.T) {
	pool := servicePGPool(t)
	a, b := replica(t, pool, "a"), replica(t, pool, "b")
	planID := uuid.New()

	runCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	l := subagentLease(planID, "s0.step0", "1.1")
	l.cancel = cancel
	held, ok := a.acquireExecutionLease(l)
	if !ok {
		t.Fatal("a: acquire refused on an empty slot")
	}

	refused, ok := b.acquireExecutionLease(subagentLease(uuid.New(), "s0.step1", "1.2"))
	if ok {
		t.Fatal("b took the lease a holds")
	}
	if refused.Owner != "a" || refused.Number != "1.1" {
		t.Fatalf("b's refusal names %+v, want a's 1.1", refused)
	}
	if got, ok := b.ExecutionInProgress(); !ok || got.token != held.token {
		t.Fatalf("b sees %+v, %v; want a's lease", got, ok)
	}

	// b force-stops it: the lease goes at once, and a's sub-agent is cancelled
	// through the bus, since only a holds its cancel func.
	taken, ok := b.takeExecutionLease()
	if !ok {
		t.Fatal("b could not take the lease")
	}
	if err := b.stopLeaseHolder(context.Background(), taken); err != nil {
		t.Fatalf("stopLeaseHolder: %v", err)
	}
	select {
	case <-runCtx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("a's sub-agent was never cancelled")
	}
	if _, ok := a.ExecutionInProgress(); ok {
		t.Fatal("the lease is still held after the force stop")
	}
	// a's own deferred release of the old grant must not touch a new one.
	next, ok := b.acquireExecutionLease(subagentLease(uuid.New(), "s0.step1", "1.2"))
	if !ok {
		t.Fatal("b: acquire refused after the stop")
	}
	a.releaseExecutionLease(held.token)
	if got, ok := a.ExecutionInProgress(); !ok || got.token != next.token {
		t.Fatal("a stale release dropped the new grant")
	}
}

// A sub-agent lease whose process stopped heartbeating frees itself.
func TestDeadReplicasLeaseExpires(t *testing.T) {
	pool := servicePGPool(t)
	a, b := replica(t, pool, "a"), replica(t, pool, "b")
	a.leaseTTL = 50 * time.Millisecond

	if _, ok := a.acquireExecutionLease(subagentLease(uuid.New(), "s0.step0", "1.1")); !ok {
		t.Fatal("a: acquire refused")
	}
	time.Sleep(150 * time.Millisecond)
	if _, ok := b.acquireExecutionLease(subagentLease(uuid.New(), "s0.step1", "1.2")); !ok {
		t.Fatal("b still refused by a lease nobody extended")
	}
}

func TestActivityCrossesReplicasOnce(t *testing.T) {
	pool := servicePGPool(t)
	a, b := replica(t, pool, "a"), replica(t, pool, "b")
	dialogID := uuid.New()

	onA, unsubA := a.SubscribeActivity(dialogID)
	defer unsubA()
	onB, unsubB := b.SubscribeActivity(dialogID)
	defer unsubB()

	a.activity.Publish(dialogID, domain.AgentActivity{Kind: domain.ActivityTurnEnd})

	for name, ch := range map[string]<-chan domain.AgentActivity{"a": onA, "b": onB} {
		select {
		case ev := <-ch:
			if ev.Kind != domain.ActivityTurnEnd {
				t.Fatalf("%s got %q, want turn_end", name, ev.Kind)
			}
		case <-time.After(5 * time.Second):
			t.Fatalf("%s never got the event", name)
		}
		select {
		case ev := <-ch:
			t.Fatalf("%s got the event twice (%q)", name, ev.Kind)
		case <-time.After(100 * time.Millisecond):
		}
	}
}

func TestNamedLockExcludesAcrossReplicas(t *testing.T) {
	pool := servicePGPool(t)
	a, b := replica(t, pool, "a"), replica(t, pool, "b")
	planID := uuid.New()

	a.planMutex(planID).Lock()
	if b.planMutex(planID).TryLock() {
		t.Fatal("b took a plan lock a holds")
	}
	a.planMutex(planID).Unlock()

	deadline := time.Now().Add(5 * time.Second)
	for !b.planMutex(planID).TryLock() {
		if time.Now().After(deadline) {
			t.Fatal("b never took the lock after a released it")
		}
		time.Sleep(10 * time.Millisecond)
	}
	b.planMutex(planID).Unlock()
}

func TestSweepSparesLiveReplicas(t *testing.T) {
	pool := servicePGPool(t)
	a, b := replica(t, pool, "a"), replica(t, pool, "b")
	planID := pgDialog(t, pool)

	if _, err := a.startActionExecRun(planID, "s0.step0"); err != nil {
		t.Fatalf("startActionExecRun: %v", err)
	}

	// b sweeping while a is alive leaves a's run alone.
	res, err := b.SweepOrphanedRuns(func(owner string) bool { return owner == "a" || owner == "b" })
	if err != nil || res.Actions != 0 {
		t.Fatalf("sweep with a alive closed %d runs (%v), want none", res.Actions, err)
	}
	runs, _ := b.ReadActionPlanExecRuns(planID)
	if !runs["s0.step0"].Active() {
		t.Fatalf("a's run = %+v, want still running", runs["s0.step0"])
	}

	// Once a is gone it is closed.
	res, err = b.SweepOrphanedRuns(func(owner string) bool { return owner == "b" })
	if err != nil || res.Actions != 1 {
		t.Fatalf("sweep with a dead closed %d runs (%v), want 1", res.Actions, err)
	}
	runs, _ = b.ReadActionPlanExecRuns(planID)
	if got := runs["s0.step0"]; got.Status != ActionExecFailed || got.Error != restartedReason {
		t.Fatalf("a's run = %+v, want failed as orphaned", got)
	}
}

func TestImportRunStateFilesMovesThemOnce(t *testing.T) {
	pool := servicePGPool(t)
	planID := pgDialog(t, pool)
	gone := uuid.New()

	dir := t.TempDir()
	svc := &ChatService{
		actionPlansDir: filepath.Join(dir, "action_plans"),
		planStateDir:   filepath.Join(dir, "plan_state"),
	}
	files := fileRunState{s: svc}
	put := func(kind repository.RunStateKind, id uuid.UUID, v any) {
		b, _ := json.Marshal(v)
		if err := files.PutRunState(context.Background(), kind, id, b); err != nil {
			t.Fatalf("seed %s: %v", kind, err)
		}
	}
	put(repository.RunStatePlanState, planID, PlanState{Status: ActionPlanStatusInProgress})
	put(repository.RunStateActionNotes, planID, map[string]string{"s0.step0": "vpc-123"})
	// A file of a dialog that no longer exists has nowhere to go.
	put(repository.RunStatePlanState, gone, PlanState{Status: ActionPlanStatusDone})

	store := postgres.NewRunStateRepo(pool)
	n, err := svc.ImportRunStateFiles(context.Background(), store)
	if err != nil || n != 2 {
		t.Fatalf("ImportRunStateFiles = %d, %v; want 2", n, err)
	}
	for _, p := range []string{
		filepath.Join(dir, "plan_state", planID.String()+".json"),
		filepath.Join(dir, "plan_state", gone.String()+".json"),
		filepath.Join(dir, "action_plans", planID.String()+".notes.json"),
	} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("%s still in place after the import", p)
		}
		if _, err := os.Stat(p + migratedSuffix); err != nil {
			t.Errorf("%s was not kept as %s: %v", p, migratedSuffix, err)
		}
	}

	svc.SetRunStateStore(store)
	state, err := svc.ReadPlanState(planID)
	if err != nil || state.Status != ActionPlanStatusInProgress {
		t.Fatalf("ReadPlanState from the store = %+v, %v", state, err)
	}
	notes, err := svc.readActionPlanNotes(planID)
	if err != nil || notes["s0.step0"] != "vpc-123" {
		t.Fatalf("readActionPlanNotes from the store = %v, %v", notes, err)
	}

	// A second boot finds nothing left to move.
	if n, err := svc.ImportRunStateFiles(context.Background(), store); err != nil || n != 0 {
		t.Fatalf("second ImportRunStateFiles = %d, %v; want 0", n, err)
	}
}
