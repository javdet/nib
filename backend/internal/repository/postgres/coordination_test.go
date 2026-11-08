package postgres

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

func TestRunStateRepoRoundTrip(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRunStateRepo(pool)
	a, b := testDialog(t, pool), testDialog(t, pool)

	if _, err := repo.GetRunState(ctx, repository.RunStateFanout, a); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetRunState on nothing = %v, want ErrNotFound", err)
	}

	if err := repo.PutRunState(ctx, repository.RunStateFanout, a, []byte(`{"status":"running"}`)); err != nil {
		t.Fatalf("PutRunState: %v", err)
	}
	if err := repo.PutRunState(ctx, repository.RunStateFanout, a, []byte(`{"status":"done"}`)); err != nil {
		t.Fatalf("PutRunState overwrite: %v", err)
	}
	got, err := repo.GetRunState(ctx, repository.RunStateFanout, a)
	if err != nil {
		t.Fatalf("GetRunState: %v", err)
	}
	if string(got) != `{"status": "done"}` {
		t.Fatalf("GetRunState = %s, want the overwritten document", got)
	}

	// Kinds are separate documents of one dialog.
	if _, err := repo.GetRunState(ctx, repository.RunStatePlanState, a); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("another kind of the same dialog = %v, want ErrNotFound", err)
	}

	inserted, err := repo.InsertRunState(ctx, repository.RunStateFanout, a, []byte(`{"status":"stale"}`))
	if err != nil || inserted {
		t.Fatalf("InsertRunState over an existing document = %v, %v; want false, nil", inserted, err)
	}
	inserted, err = repo.InsertRunState(ctx, repository.RunStateFanout, b, []byte(`{"status":"new"}`))
	if err != nil || !inserted {
		t.Fatalf("InsertRunState into an empty slot = %v, %v; want true, nil", inserted, err)
	}

	many, err := repo.GetRunStates(ctx, repository.RunStateFanout, []uuid.UUID{a, b, uuid.New()})
	if err != nil {
		t.Fatalf("GetRunStates: %v", err)
	}
	if len(many) != 2 {
		t.Fatalf("GetRunStates returned %d documents, want 2", len(many))
	}

	list, err := repo.ListRunStates(ctx, repository.RunStateFanout)
	if err != nil || len(list) != 2 {
		t.Fatalf("ListRunStates = %d entries, %v; want 2", len(list), err)
	}

	// Only documents with a running value anywhere, nested in a map included.
	if err := repo.PutRunState(ctx, repository.RunStateActionExec, a,
		[]byte(`{"s0.step0":{"status":"done"},"s0.step1":{"status":"running"}}`)); err != nil {
		t.Fatalf("PutRunState: %v", err)
	}
	if err := repo.PutRunState(ctx, repository.RunStateActionExec, b,
		[]byte(`{"s0.step0":{"status":"failed","error":"was running"}}`)); err != nil {
		t.Fatalf("PutRunState: %v", err)
	}
	running, err := repo.ListRunningRunStates(ctx, repository.RunStateActionExec)
	if err != nil || len(running) != 1 || running[0].DialogID != a {
		t.Fatalf("ListRunningRunStates = %+v, %v; want only %s", running, err, a)
	}

	if err := repo.DeleteRunState(ctx, repository.RunStateFanout, a); err != nil {
		t.Fatalf("DeleteRunState: %v", err)
	}
	if err := repo.DeleteRunState(ctx, repository.RunStateFanout, a); err != nil {
		t.Fatalf("DeleteRunState of nothing: %v", err)
	}
	if _, err := repo.GetRunState(ctx, repository.RunStateFanout, a); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetRunState after delete = %v, want ErrNotFound", err)
	}
}

// A model's output can carry a NUL, which encoding/json writes as \u0000 and a
// jsonb column refuses outright.
func TestRunStateRepoStripsNUL(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRunStateRepo(pool)
	id := testDialog(t, pool)

	if err := repo.PutRunState(ctx, repository.RunStateActionNotes, id, []byte(`{"s0.step0":"a\u0000b"}`)); err != nil {
		t.Fatalf("PutRunState with a NUL: %v", err)
	}
	got, err := repo.GetRunState(ctx, repository.RunStateActionNotes, id)
	if err != nil {
		t.Fatalf("GetRunState: %v", err)
	}
	if !strings.Contains(string(got), `"ab"`) {
		t.Fatalf("GetRunState = %s, want the NUL dropped", got)
	}
}

func TestRunStateRepoFollowsItsDialog(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewRunStateRepo(pool)

	err := repo.PutRunState(ctx, repository.RunStatePlanState, uuid.New(), []byte(`{}`))
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("PutRunState for a missing dialog = %v, want ErrNotFound", err)
	}

	id := testDialog(t, pool)
	if err := repo.PutRunState(ctx, repository.RunStatePlanState, id, []byte(`{}`)); err != nil {
		t.Fatalf("PutRunState: %v", err)
	}
	if _, err := pool.Exec(ctx, `DELETE FROM chat_dialogs WHERE id = $1`, id); err != nil {
		t.Fatalf("delete dialog: %v", err)
	}
	if _, err := repo.GetRunState(ctx, repository.RunStatePlanState, id); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetRunState after the dialog went = %v, want the document gone with it", err)
	}
}

func TestExecutionLeaseRepoIsOneSlot(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewExecutionLeaseRepo(pool)

	first := repository.LeaseRecord{Token: uuid.New(), Owner: "a", Holder: []byte(`{"key":"s0.step0"}`)}
	ok, _, err := repo.TryAcquireLease(ctx, first)
	if err != nil || !ok {
		t.Fatalf("TryAcquireLease on an empty slot = %v, %v; want granted", ok, err)
	}

	second := repository.LeaseRecord{Token: uuid.New(), Owner: "b", Holder: []byte(`{}`),
		ExpiresAt: time.Now().Add(time.Minute)}
	ok, held, err := repo.TryAcquireLease(ctx, second)
	if err != nil || ok {
		t.Fatalf("TryAcquireLease on a held slot = %v, %v; want refused", ok, err)
	}
	if held.Token != first.Token || held.Owner != "a" || !held.ExpiresAt.IsZero() {
		t.Fatalf("holder = %+v, want the first grant, never expiring", held)
	}

	if ok, err := repo.UpdateLeaseHolder(ctx, second.Token, []byte(`{"x":1}`)); err != nil || ok {
		t.Fatalf("UpdateLeaseHolder with a stale token = %v, %v; want false", ok, err)
	}
	if ok, err := repo.UpdateLeaseHolder(ctx, first.Token, []byte(`{"key":"s0.step0","dialogId":"d"}`)); err != nil || !ok {
		t.Fatalf("UpdateLeaseHolder = %v, %v; want true", ok, err)
	}
	until := time.Now().Add(time.Hour).Truncate(time.Microsecond)
	if ok, err := repo.ExtendLease(ctx, first.Token, until); err != nil || !ok {
		t.Fatalf("ExtendLease = %v, %v; want true", ok, err)
	}
	got, err := repo.GetLease(ctx)
	if err != nil {
		t.Fatalf("GetLease: %v", err)
	}
	if !got.ExpiresAt.Equal(until) || !strings.Contains(string(got.Holder), "dialogId") {
		t.Fatalf("GetLease = %+v, want the stamped holder expiring at %v", got, until)
	}

	if ok, err := repo.DeleteLease(ctx, second.Token); err != nil || ok {
		t.Fatalf("DeleteLease with a stale token = %v, %v; want false", ok, err)
	}
	if ok, err := repo.DeleteLease(ctx, first.Token); err != nil || !ok {
		t.Fatalf("DeleteLease = %v, %v; want true", ok, err)
	}
	if _, err := repo.GetLease(ctx); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetLease after release = %v, want ErrNotFound", err)
	}
	if ok, _, err := repo.TryAcquireLease(ctx, second); err != nil || !ok {
		t.Fatalf("TryAcquireLease after release = %v, %v; want granted", ok, err)
	}
}

func TestReplicaRepoLiveness(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewReplicaRepo(pool)

	for _, id := range []string{"a", "b"} {
		if err := repo.RegisterReplica(ctx, repository.Replica{ID: id, Hostname: "h", Version: "v"}); err != nil {
			t.Fatalf("RegisterReplica %s: %v", id, err)
		}
	}
	// b stopped heartbeating a while ago.
	if _, err := pool.Exec(ctx, `UPDATE nib_replicas SET heartbeat_at = now() - interval '5 minutes' WHERE id = 'b'`); err != nil {
		t.Fatalf("age b: %v", err)
	}

	live, err := repo.ListLiveReplicas(ctx, time.Minute)
	if err != nil || len(live) != 1 || live[0] != "a" {
		t.Fatalf("ListLiveReplicas = %v, %v; want [a]", live, err)
	}
	dead, err := repo.DeleteStaleReplicas(ctx, time.Minute)
	if err != nil || len(dead) != 1 || dead[0] != "b" {
		t.Fatalf("DeleteStaleReplicas = %v, %v; want [b]", dead, err)
	}
	if ok, err := repo.HeartbeatReplica(ctx, "b"); err != nil || ok {
		t.Fatalf("HeartbeatReplica of a swept process = %v, %v; want false", ok, err)
	}
	if ok, err := repo.HeartbeatReplica(ctx, "a"); err != nil || !ok {
		t.Fatalf("HeartbeatReplica = %v, %v; want true", ok, err)
	}
	if err := repo.RegisterReplica(ctx, repository.Replica{ID: "a2", Hostname: "h"}); err != nil {
		t.Fatalf("RegisterReplica a2: %v", err)
	}
	retired, err := repo.RetireReplicasOnHost(ctx, "h", "a2")
	if err != nil || len(retired) != 1 || retired[0] != "a" {
		t.Fatalf("RetireReplicasOnHost = %v, %v; want [a]", retired, err)
	}
	if err := repo.RegisterReplica(ctx, repository.Replica{ID: "a", Hostname: "h"}); err != nil {
		t.Fatalf("RegisterReplica a again: %v", err)
	}
	if err := repo.DeregisterReplica(ctx, "a2"); err != nil {
		t.Fatalf("DeregisterReplica a2: %v", err)
	}
	if err := repo.DeregisterReplica(ctx, "a"); err != nil {
		t.Fatalf("DeregisterReplica: %v", err)
	}
	if live, _ := repo.ListLiveReplicas(ctx, time.Minute); len(live) != 0 {
		t.Fatalf("ListLiveReplicas after deregister = %v, want none", live)
	}
}

func TestAdvisoryLockerExcludesAcrossSessions(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	locker := NewAdvisoryLocker(pool)

	release, err := locker.Acquire(ctx, "plan:x")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	if _, ok, err := locker.TryAcquire(ctx, "plan:x"); err != nil || ok {
		t.Fatalf("TryAcquire of a held lock = %v, %v; want false", ok, err)
	}
	other, ok, err := locker.TryAcquire(ctx, "plan:y")
	if err != nil || !ok {
		t.Fatalf("TryAcquire of another key = %v, %v; want true", ok, err)
	}
	other()

	acquired := make(chan func())
	go func() {
		r, err := locker.Acquire(ctx, "plan:x")
		if err != nil {
			t.Errorf("second Acquire: %v", err)
			close(acquired)
			return
		}
		acquired <- r
	}()
	select {
	case <-acquired:
		t.Fatal("a second session took a held lock")
	case <-time.After(200 * time.Millisecond):
	}

	release()
	select {
	case r := <-acquired:
		if r != nil {
			r()
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the waiter never took the lock after it was released")
	}

	// A waiter that gives up leaves no lock behind. Its session is dropped
	// rather than reused, and the server may grant the lock to it in the
	// moment before it notices -- so "behind" means once the session is gone,
	// not instantly.
	release, err = locker.Acquire(ctx, "plan:x")
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	if _, err := locker.Acquire(waitCtx, "plan:x"); err == nil {
		t.Fatal("Acquire with an expiring context took a held lock")
	}
	release()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, ok, err := locker.TryAcquire(ctx, "plan:x")
		if err != nil {
			t.Fatalf("TryAcquire: %v", err)
		}
		if ok {
			r()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the lock stayed held after the waiter that gave up was gone")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestNotifyBusDelivers(t *testing.T) {
	pool := testPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	bus := NewNotifyBus(pool, pool, "nib_test_a", "nib_test_b")
	got := make(chan string, 4)
	go bus.Run(ctx, func(channel string, payload []byte) {
		got <- channel + ":" + string(payload)
	})

	deadline := time.Now().Add(5 * time.Second)
	for !bus.Listening() {
		if time.Now().After(deadline) {
			t.Fatal("the bus never started listening")
		}
		time.Sleep(10 * time.Millisecond)
	}

	if err := bus.Publish(ctx, "nib_test_b", []byte(`{"x":1}`)); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case msg := <-got:
		if msg != `nib_test_b:{"x":1}` {
			t.Fatalf("delivered %q, want the published message", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("nothing delivered")
	}

	cancel()
	deadline = time.Now().Add(5 * time.Second)
	for bus.Listening() {
		if time.Now().After(deadline) {
			t.Fatal("the bus still reports listening after its context ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestMarkRunningCommandRunsLostSparesLiveOwners(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewCommandRunRepo(pool)

	runs := map[string]uuid.UUID{}
	for _, owner := range []string{"live", "dead", ""} {
		id := uuid.New()
		runs[owner] = id
		if err := repo.InsertCommandRun(ctx, domain.CommandRun{
			ID: id, Kind: domain.CommandRunKindRun, Script: "true", UID: -1,
			Status: domain.CommandRunRunning, StartedAt: time.Now(), Owner: owner,
		}); err != nil {
			t.Fatalf("InsertCommandRun %q: %v", owner, err)
		}
	}

	n, err := repo.MarkRunningCommandRunsLost(ctx, "gone", []string{"live"})
	if err != nil || n != 2 {
		t.Fatalf("MarkRunningCommandRunsLost = %d, %v; want 2", n, err)
	}
	for owner, id := range runs {
		run, err := repo.GetCommandRun(ctx, id)
		if err != nil {
			t.Fatalf("GetCommandRun %q: %v", owner, err)
		}
		want := domain.CommandRunLost
		if owner == "live" {
			want = domain.CommandRunRunning
		}
		if run.Status != want || run.Owner != owner {
			t.Errorf("run of %q = %s owned by %q, want %s", owner, run.Status, run.Owner, want)
		}
	}
}
