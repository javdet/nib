package cmdrun

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

type fakeRecorder struct {
	mu       sync.Mutex
	inserted []domain.CommandRun
	finished []domain.CommandRun
}

func (f *fakeRecorder) InsertCommandRun(_ context.Context, run domain.CommandRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserted = append(f.inserted, run)
	return nil
}

func (f *fakeRecorder) FinishCommandRun(_ context.Context, run domain.CommandRun) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.finished = append(f.finished, run)
	return nil
}

// newTestManager runs jobs as the test's own user, so these tests pass on any
// host; isolation has tests of its own.
func newTestManager(t *testing.T, cfg ManagerConfig, rec Recorder) *Manager {
	t.Helper()
	sb, err := NewSandbox(SandboxConfig{Isolation: IsolationOff, JobsRoot: filepath.Join(t.TempDir(), "jobs")})
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	if cfg.LogDir == "" {
		cfg.LogDir = t.TempDir()
	}
	return NewManager(sb, cfg, rec)
}

func startJob(t *testing.T, m *Manager, ctx context.Context, script string, timeout time.Duration) *Job {
	t.Helper()
	j, err := m.Start(ctx, StartSpec{Owner: "owner", PlanID: uuid.New(), Script: script, Timeout: timeout})
	if err != nil {
		t.Fatalf("Start(%q): %v", script, err)
	}
	return j
}

func waitDone(t *testing.T, j *Job) domain.CommandRun {
	t.Helper()
	if !j.Wait(context.Background(), 30*time.Second) {
		t.Fatalf("job %s did not finish", j.ID())
	}
	return j.Snapshot()
}

func readAll(t *testing.T, j *Job) string {
	t.Helper()
	data, _, err := j.Read(0, 1<<20)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return string(data)
}

func TestJobOutcomes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		script     string
		timeout    time.Duration
		wantStatus domain.CommandRunStatus
		wantExit   int
		wantOutput string
	}{
		{name: "success with a pipe", script: "echo hello | tr a-z A-Z", timeout: 10 * time.Second, wantStatus: domain.CommandRunSucceeded, wantExit: 0, wantOutput: "HELLO\n"},
		{name: "non-zero exit", script: "echo oops >&2; exit 3", timeout: 10 * time.Second, wantStatus: domain.CommandRunFailed, wantExit: 3, wantOutput: "oops\n"},
		{name: "timeout", script: "sleep 30", timeout: 200 * time.Millisecond, wantStatus: domain.CommandRunTimedOut, wantExit: -2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newTestManager(t, ManagerConfig{}, nil)
			run := waitDone(t, startJob(t, m, context.Background(), tt.script, tt.timeout))
			if run.Status != tt.wantStatus {
				t.Fatalf("Status = %q, want %q (reason %q)", run.Status, tt.wantStatus, run.Reason)
			}
			if tt.wantExit == -2 {
				if run.ExitCode != nil {
					t.Fatalf("ExitCode = %d, want none for a killed job", *run.ExitCode)
				}
			} else if run.ExitCode == nil || *run.ExitCode != tt.wantExit {
				t.Fatalf("ExitCode = %v, want %d", run.ExitCode, tt.wantExit)
			}
			if tt.wantOutput != "" {
				j, err := m.Lookup("owner", run.ID)
				if err != nil {
					t.Fatalf("Lookup: %v", err)
				}
				if got := readAll(t, j); got != tt.wantOutput {
					t.Fatalf("output = %q, want %q", got, tt.wantOutput)
				}
			}
		})
	}
}

func TestJobEndsWithOwnerContext(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, ManagerConfig{}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	j := startJob(t, m, ctx, "sleep 30", time.Minute)
	cancel()
	run := waitDone(t, j)
	if run.Status != domain.CommandRunCancelled || run.Reason != ReasonRunEnded {
		t.Fatalf("Status, Reason = %q, %q; want cancelled, %q", run.Status, run.Reason, ReasonRunEnded)
	}
	// The finished job is dropped once its owner is gone.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := m.Lookup("owner", j.ID()); errors.Is(err, ErrJobNotFound) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("job still registered after its owner ended")
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestJobCancelKeepsFirstReason(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, ManagerConfig{}, nil)
	j := startJob(t, m, context.Background(), "sleep 30", time.Minute)
	j.Cancel(ReasonAgent)
	j.Cancel(ReasonOperator)
	run := waitDone(t, j)
	if run.Status != domain.CommandRunCancelled || run.Reason != ReasonAgent {
		t.Fatalf("Status, Reason = %q, %q; want cancelled, %q", run.Status, run.Reason, ReasonAgent)
	}
	if err := m.CancelByID(j.ID(), ReasonOperator); !errors.Is(err, ErrJobNotRunning) {
		t.Fatalf("CancelByID on a finished job = %v, want ErrJobNotRunning", err)
	}
}

func TestLookupIsScopedToOwner(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, ManagerConfig{}, nil)
	j := startJob(t, m, context.Background(), "true", time.Minute)
	if _, err := m.Lookup("someone else", j.ID()); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("Lookup by another owner = %v, want ErrJobNotFound", err)
	}
	if _, err := m.Lookup("owner", j.ID()); err != nil {
		t.Fatalf("Lookup by its owner: %v", err)
	}
}

func TestStartLimits(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, ManagerConfig{MaxPerOwner: 1, MaxTotal: 2}, nil)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	startJob(t, m, ctx, "sleep 30", time.Minute)
	if _, err := m.Start(ctx, StartSpec{Owner: "owner", Script: "true", Timeout: time.Minute}); !errors.Is(err, ErrTooManyJobs) {
		t.Fatalf("second job for one owner = %v, want ErrTooManyJobs", err)
	}
	if _, err := m.Start(ctx, StartSpec{Owner: "b", Script: "sleep 30", Timeout: time.Minute}); err != nil {
		t.Fatalf("job for a second owner: %v", err)
	}
	if _, err := m.Start(ctx, StartSpec{Owner: "c", Script: "true", Timeout: time.Minute}); !errors.Is(err, ErrTooManyJobs) {
		t.Fatalf("job over the global limit = %v, want ErrTooManyJobs", err)
	}
}

func TestOutputCap(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, ManagerConfig{MaxOutputBytes: 100}, nil)
	j := startJob(t, m, context.Background(), "head -c 1000 /dev/zero | tr '\\0' x", time.Minute)
	run := waitDone(t, j)
	if run.OutputBytes != 100 || !run.OutputCapped {
		t.Fatalf("OutputBytes, OutputCapped = %d, %v; want 100, true", run.OutputBytes, run.OutputCapped)
	}
	if got := readAll(t, j); got != strings.Repeat("x", 100) {
		t.Fatalf("output = %q, want 100 x", got)
	}
}

func TestReadOffsets(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, ManagerConfig{}, nil)
	j := startJob(t, m, context.Background(), "printf 0123456789", time.Minute)
	waitDone(t, j)
	data, size, err := j.Read(3, 4)
	if err != nil || string(data) != "3456" || size != 10 {
		t.Fatalf("Read(3, 4) = %q, %d, %v; want \"3456\", 10, nil", data, size, err)
	}
	if data, _, _ := j.Read(20, 4); len(data) != 0 {
		t.Fatalf("Read past the end = %q, want nothing", data)
	}
	got, _, err := m.ReadLog(j.Snapshot().PlanID, j.ID(), 8, 100)
	if err != nil || string(got) != "89" {
		t.Fatalf("ReadLog = %q, %v; want \"89\", nil", got, err)
	}
	if _, _, err := m.ReadLog(uuid.New(), uuid.New(), 0, 10); !errors.Is(err, ErrLogGone) {
		t.Fatalf("ReadLog of a missing run = %v, want ErrLogGone", err)
	}
}

func TestWaitUntil(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name         string
		script       string
		pattern      string
		timeout      time.Duration
		wantStatus   domain.CommandRunStatus
		wantMet      bool
		wantAttempts int
	}{
		{name: "exit 0 on the second attempt", script: `test -f "$HOME/flag" || { touch "$HOME/flag"; exit 1; }`, timeout: 10 * time.Second, wantStatus: domain.CommandRunSucceeded, wantMet: true, wantAttempts: 2},
		{name: "pattern", script: `echo status=ready`, pattern: `status=ready`, timeout: 10 * time.Second, wantStatus: domain.CommandRunSucceeded, wantMet: true, wantAttempts: 1},
		{name: "never", script: `exit 1`, timeout: 300 * time.Millisecond, wantStatus: domain.CommandRunTimedOut},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			m := newTestManager(t, ManagerConfig{}, nil)
			until := &UntilSpec{Interval: 50 * time.Millisecond, AttemptTimeout: 5 * time.Second}
			if tt.pattern != "" {
				until.Pattern = regexp.MustCompile(tt.pattern)
			}
			j, err := m.Start(context.Background(), StartSpec{Owner: "o", Script: tt.script, Timeout: tt.timeout, Until: until})
			if err != nil {
				t.Fatalf("Start: %v", err)
			}
			run := waitDone(t, j)
			if run.Status != tt.wantStatus || run.Kind != domain.CommandRunKindWaitUntil {
				t.Fatalf("Status, Kind = %q, %q; want %q, wait_until", run.Status, run.Kind, tt.wantStatus)
			}
			st := j.Until()
			if st.Met != tt.wantMet {
				t.Fatalf("Met = %v, want %v", st.Met, tt.wantMet)
			}
			if tt.wantAttempts > 0 && st.Attempts != tt.wantAttempts {
				t.Fatalf("Attempts = %d, want %d", st.Attempts, tt.wantAttempts)
			}
		})
	}
}

func TestRecorderSeesStartAndFinish(t *testing.T) {
	t.Parallel()
	rec := &fakeRecorder{}
	m := newTestManager(t, ManagerConfig{}, rec)
	j := startJob(t, m, context.Background(), "true", time.Minute)
	waitDone(t, j)
	rec.mu.Lock()
	defer rec.mu.Unlock()
	if len(rec.inserted) != 1 || rec.inserted[0].Status != domain.CommandRunRunning {
		t.Fatalf("inserted = %+v, want one running row", rec.inserted)
	}
	if len(rec.finished) != 1 || rec.finished[0].Status != domain.CommandRunSucceeded || rec.finished[0].FinishedAt == nil {
		t.Fatalf("finished = %+v, want one succeeded row with a finish time", rec.finished)
	}
}

func TestShutdownCancelsAndRefuses(t *testing.T) {
	t.Parallel()
	m := newTestManager(t, ManagerConfig{}, nil)
	j := startJob(t, m, context.Background(), "sleep 30", time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := m.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if run := j.Snapshot(); run.Reason != ReasonShutdown {
		t.Fatalf("Reason = %q, want %q", run.Reason, ReasonShutdown)
	}
	if _, err := m.Start(context.Background(), StartSpec{Owner: "o", Script: "true", Timeout: time.Minute}); !errors.Is(err, ErrShuttingDown) {
		t.Fatalf("Start after Shutdown = %v, want ErrShuttingDown", err)
	}
}

func TestSweepLogs(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	m := newTestManager(t, ManagerConfig{LogDir: dir}, nil)
	plan := filepath.Join(dir, uuid.NewString())
	if err := os.MkdirAll(plan, 0o700); err != nil {
		t.Fatal(err)
	}
	old, fresh := filepath.Join(plan, "old.log"), filepath.Join(plan, "fresh.log")
	for _, p := range []string{old, fresh} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-48 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	if n := m.SweepLogs(24 * time.Hour); n != 1 {
		t.Fatalf("SweepLogs removed %d, want 1", n)
	}
	if _, err := os.Stat(fresh); err != nil {
		t.Fatalf("fresh log removed: %v", err)
	}
}

func TestChildEnvKeepsOnlyAllowedKeys(t *testing.T) {
	t.Setenv("NIB_TEST_SECRET", "leaked")
	t.Setenv("LANG", "C.UTF-8")
	got := map[string]string{}
	for _, kv := range childEnv("/jobs/x") {
		k, v, _ := strings.Cut(kv, "=")
		got[k] = v
	}
	want := map[string]string{"HOME": "/jobs/x", "TMPDIR": "/jobs/x/tmp", "LANG": "C.UTF-8"}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
	if _, ok := got["PATH"]; !ok {
		t.Error("PATH missing")
	}
	if len(got) != 4 {
		t.Fatalf("env = %v, want only PATH, HOME, TMPDIR and LANG", got)
	}
}

func TestParseIsolationMode(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in      string
		want    IsolationMode
		wantErr bool
	}{
		{in: "", want: IsolationAuto},
		{in: "Auto", want: IsolationAuto},
		{in: "required", want: IsolationRequired},
		{in: "off", want: IsolationOff},
		{in: "on", wantErr: true},
	}
	for _, tt := range tests {
		got, err := ParseIsolationMode(tt.in)
		if (err != nil) != tt.wantErr || got != tt.want {
			t.Errorf("ParseIsolationMode(%q) = %q, %v; want %q, err %v", tt.in, got, err, tt.want, tt.wantErr)
		}
	}
}

func TestUIDPool(t *testing.T) {
	t.Parallel()
	p := newUIDPool(100, 2)
	a, err := p.acquire()
	if err != nil {
		t.Fatal(err)
	}
	b, err := p.acquire()
	if err != nil || a == b {
		t.Fatalf("second uid = %d, %v; want a different uid", b, err)
	}
	if _, err := p.acquire(); !errors.Is(err, ErrNoFreeUID) {
		t.Fatalf("third acquire = %v, want ErrNoFreeUID", err)
	}
	p.release(a)
	if c, err := p.acquire(); err != nil || c != a {
		t.Fatalf("acquire after release = %d, %v; want %d", c, err, a)
	}
	if !p.contains(101) || p.contains(102) || p.contains(99) {
		t.Fatal("contains is wrong at the range edges")
	}
}
