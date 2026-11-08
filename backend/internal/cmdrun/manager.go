package cmdrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

var (
	// ErrJobNotFound covers both a job that does not exist and one that belongs
	// to another agent: a Digger must not learn that its sibling's job ids are
	// real.
	ErrJobNotFound = errors.New("no such job")
	// ErrJobNotRunning is an operator cancel of a job that has already ended.
	ErrJobNotRunning = errors.New("the command is not running")
	// ErrTooManyJobs is a start over the per-agent or global limit.
	ErrTooManyJobs = errors.New("too many commands are running")
	// ErrShuttingDown is a start after Shutdown began.
	ErrShuttingDown = errors.New("the server is shutting down")
)

const (
	defaultMaxOutputBytes = 10 << 20
	defaultMaxPerOwner    = 4
	defaultMaxTotal       = 16
	// untilCaptureBytes is how much of one wait_until attempt is matched
	// against the pattern; untilTailBytes is how much the model gets back.
	untilCaptureBytes = 64 << 10
	untilTailBytes    = 8 << 10
	// finishedJobTTL drops a finished job from memory when the run that owns
	// it never ends, which only a context without a Done channel can do.
	finishedJobTTL = 6 * time.Hour

	// Reasons recorded on a run. The tools show them to the model as they are.
	ReasonRunEnded  = "the agent run that started it ended"
	ReasonShutdown  = "the server is shutting down"
	ReasonOperator  = "cancelled by the operator"
	ReasonAgent     = "cancelled with cancel_job"
	reasonTimedOut  = "timed out"
	reasonStartFail = "could not start: "
)

// Recorder keeps the audit trail. Failures are logged, never fatal: an audit
// row that did not land must not stop a command the agent is waiting on.
type Recorder interface {
	InsertCommandRun(ctx context.Context, run domain.CommandRun) error
	FinishCommandRun(ctx context.Context, run domain.CommandRun) error
}

// ManagerConfig tunes the limits. Zero values take the defaults.
type ManagerConfig struct {
	// LogDir holds {plan}/{job}.log. It belongs on the data volume, which is
	// mode 0700 and so out of every job's reach.
	LogDir         string
	MaxOutputBytes int64
	MaxPerOwner    int
	MaxTotal       int
	// Replica names the backend process on every run it records.
	Replica string
}

// Manager owns every running job.
type Manager struct {
	sb  *Sandbox
	cfg ManagerConfig
	rec Recorder

	mu       sync.Mutex
	jobs     map[uuid.UUID]*Job
	perOwner map[string]int
	running  int
	closed   bool
	wg       sync.WaitGroup
}

// NewManager wires the job manager. rec may be nil.
func NewManager(sb *Sandbox, cfg ManagerConfig, rec Recorder) *Manager {
	if cfg.MaxOutputBytes <= 0 {
		cfg.MaxOutputBytes = defaultMaxOutputBytes
	}
	if cfg.MaxPerOwner <= 0 {
		cfg.MaxPerOwner = defaultMaxPerOwner
	}
	if cfg.MaxTotal <= 0 {
		cfg.MaxTotal = defaultMaxTotal
	}
	return &Manager{
		sb:       sb,
		cfg:      cfg,
		rec:      rec,
		jobs:     make(map[uuid.UUID]*Job),
		perOwner: make(map[string]int),
	}
}

// Sandbox is the spawner jobs run in, which api_call shares.
func (m *Manager) Sandbox() *Sandbox { return m.sb }

// UntilSpec makes a job rerun its script until a condition holds.
type UntilSpec struct {
	// Pattern, when set, must match the attempt's output; nil means exit 0.
	Pattern  *regexp.Regexp
	Interval time.Duration
	// AttemptTimeout bounds one attempt, so a hung probe cannot eat the budget.
	AttemptTimeout time.Duration
}

// StartSpec is one job to start.
type StartSpec struct {
	// Owner scopes the job: only a caller passing the same owner can read,
	// wait on or cancel it.
	Owner     string
	PlanID    uuid.UUID
	DialogID  uuid.UUID
	ActionKey string
	Mode      string
	Script    string
	Timeout   time.Duration
	Until     *UntilSpec
}

// Job is one running or finished command.
type Job struct {
	owner   string
	logPath string
	log     *logFile
	until   *UntilSpec
	done    chan struct{}
	cancel  context.CancelFunc

	mu           sync.Mutex
	run          domain.CommandRun
	cancelReason string
	delivered    int64
	attempts     int
	lastExit     int
	lastOutput   []byte
	met          bool
}

// Start launches a job. The job is cancelled when ctx ends: a command never
// outlives the agent run that asked for it, whatever ended that run -- a force
// stop, a timeout, a cancelled investigation or the end of a chat turn.
func (m *Manager) Start(ctx context.Context, spec StartSpec) (*Job, error) {
	if err := m.sb.Refused(); err != nil {
		return nil, err
	}
	if err := m.reserve(spec.Owner); err != nil {
		return nil, err
	}
	reserved := true
	defer func() {
		if reserved {
			m.unreserve(spec.Owner)
		}
	}()

	uid, err := m.sb.acquireUID()
	if err != nil {
		return nil, err
	}
	id := uuid.New()
	home, err := m.sb.makeHome(id.String(), uid)
	if err != nil {
		m.sb.releaseUID(uid)
		return nil, err
	}
	logPath := m.LogPath(spec.PlanID, id)
	lf, err := createLog(logPath, m.cfg.MaxOutputBytes)
	if err != nil {
		m.sb.removeHome(home, uid)
		m.sb.releaseUID(uid)
		return nil, err
	}

	kind := domain.CommandRunKindRun
	if spec.Until != nil {
		kind = domain.CommandRunKindWaitUntil
	}
	jobCtx, cancel := context.WithTimeout(context.Background(), spec.Timeout)
	job := &Job{
		owner:   spec.Owner,
		logPath: logPath,
		log:     lf,
		until:   spec.Until,
		done:    make(chan struct{}),
		cancel:  cancel,
		run: domain.CommandRun{
			ID:        id,
			PlanID:    spec.PlanID,
			DialogID:  spec.DialogID,
			ActionKey: spec.ActionKey,
			Mode:      spec.Mode,
			Kind:      kind,
			Script:    spec.Script,
			Isolated:  uid >= 0,
			UID:       uid,
			Status:    domain.CommandRunRunning,
			StartedAt: time.Now().UTC(),
			Owner:     m.cfg.Replica,
		},
	}

	m.mu.Lock()
	m.jobs[id] = job
	m.mu.Unlock()
	reserved = false

	m.record(func(ctx context.Context, r Recorder) error { return r.InsertCommandRun(ctx, job.Snapshot()) })

	// Not stopped when the job ends: firing after it is what drops the finished
	// job from memory once its owner can no longer ask about it.
	context.AfterFunc(ctx, func() {
		job.Cancel(ReasonRunEnded)
		<-job.done
		m.forget(id)
	})

	m.wg.Add(1)
	go m.runJob(jobCtx, job, home, uid)
	return job, nil
}

func (m *Manager) reserve(owner string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return ErrShuttingDown
	}
	m.pruneLocked()
	if m.perOwner[owner] >= m.cfg.MaxPerOwner {
		return fmt.Errorf("%w: this agent already has %d running; wait for one or cancel it", ErrTooManyJobs, m.cfg.MaxPerOwner)
	}
	if m.running >= m.cfg.MaxTotal {
		return fmt.Errorf("%w: %d are running across every agent; wait for one to finish", ErrTooManyJobs, m.cfg.MaxTotal)
	}
	m.perOwner[owner]++
	m.running++
	return nil
}

func (m *Manager) unreserve(owner string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.running--
	if m.perOwner[owner]--; m.perOwner[owner] <= 0 {
		delete(m.perOwner, owner)
	}
}

func (m *Manager) forget(id uuid.UUID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.jobs, id)
}

func (m *Manager) pruneLocked() {
	cutoff := time.Now().Add(-finishedJobTTL)
	for id, j := range m.jobs {
		snap := j.Snapshot()
		if snap.FinishedAt != nil && snap.FinishedAt.Before(cutoff) {
			delete(m.jobs, id)
		}
	}
}

func (m *Manager) record(write func(context.Context, Recorder) error) {
	if m.rec == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := write(ctx, m.rec); err != nil {
		slog.Warn("command run audit write failed", "error", err)
	}
}

func (m *Manager) runJob(ctx context.Context, job *Job, home string, uid int) {
	defer m.wg.Done()
	defer job.cancel()

	var (
		status domain.CommandRunStatus
		exit   *int
		reason string
	)
	if job.until == nil {
		code, stopped, err := m.sb.runProcess(ctx, m.shellArgv(job.run.Script), home, uid, job.log)
		switch {
		case err != nil:
			status, reason = domain.CommandRunFailed, reasonStartFail+err.Error()
		case stopped:
			status, reason = job.endedBy(ctx.Err())
		default:
			exit = &code
			status = domain.CommandRunSucceeded
			if code != 0 {
				status = domain.CommandRunFailed
			}
		}
	} else {
		status, exit, reason = m.runUntil(ctx, job, home, uid)
		if status == domain.CommandRunTimedOut {
			// The deadline and a cancel both end the loop the same way; only
			// the recorded reason tells them apart.
			status, reason = job.endedBy(ctx.Err())
		}
	}

	m.sb.removeHome(home, uid)
	m.sb.releaseUID(uid)
	job.log.close()
	m.finish(job, status, exit, reason)
}

// runUntil reruns the script until it exits 0 (or its output matches), or ctx
// ends. Each attempt goes into the log under a header, so read_output shows
// the whole history.
func (m *Manager) runUntil(ctx context.Context, job *Job, home string, uid int) (domain.CommandRunStatus, *int, string) {
	argv := m.shellArgv(job.run.Script)
	for attempt := 1; ; attempt++ {
		fmt.Fprintf(job.log, "--- attempt %d at %s ---\n", attempt, time.Now().UTC().Format(time.RFC3339))
		capture := &headBuffer{max: untilCaptureBytes}
		attemptCtx, cancel := context.WithTimeout(ctx, job.until.AttemptTimeout)
		code, _, err := m.sb.runProcess(attemptCtx, argv, home, uid, io.MultiWriter(job.log, capture))
		cancel()
		if err != nil {
			return domain.CommandRunFailed, nil, reasonStartFail + err.Error()
		}
		out := capture.Bytes()
		met := code == 0
		if job.until.Pattern != nil {
			met = job.until.Pattern.Match(out)
		}
		job.mu.Lock()
		job.attempts = attempt
		job.lastExit = code
		job.lastOutput = tailBytes(out, untilTailBytes)
		job.met = met
		job.mu.Unlock()

		if met && ctx.Err() == nil {
			return domain.CommandRunSucceeded, &code, ""
		}
		select {
		case <-ctx.Done():
			return domain.CommandRunTimedOut, &code, reasonTimedOut
		case <-time.After(job.until.Interval):
		}
	}
}

func (m *Manager) shellArgv(script string) []string {
	argv := make([]string, 0, len(m.sb.shell)+1)
	argv = append(argv, m.sb.shell...)
	return append(argv, script)
}

func (m *Manager) finish(job *Job, status domain.CommandRunStatus, exit *int, reason string) {
	stored, produced := job.log.sizes()
	now := time.Now().UTC()
	job.mu.Lock()
	job.run.Status = status
	job.run.ExitCode = exit
	job.run.Reason = reason
	job.run.OutputBytes = stored
	job.run.OutputCapped = produced > stored
	job.run.FinishedAt = &now
	job.run.DurationMS = now.Sub(job.run.StartedAt).Milliseconds()
	job.mu.Unlock()

	m.unreserve(job.owner)
	m.record(func(ctx context.Context, r Recorder) error { return r.FinishCommandRun(ctx, job.Snapshot()) })
	close(job.done)
}

// Lookup returns owner's job id.
func (m *Manager) Lookup(owner string, id uuid.UUID) (*Job, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	j, ok := m.jobs[id]
	if !ok || j.owner != owner {
		return nil, ErrJobNotFound
	}
	return j, nil
}

// CancelByID is the operator's cancel, which is not scoped to an owner.
func (m *Manager) CancelByID(id uuid.UUID, reason string) error {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return ErrJobNotRunning
	}
	select {
	case <-j.done:
		return ErrJobNotRunning
	default:
	}
	j.Cancel(reason)
	return nil
}

// Running returns the live record of a job still in memory.
func (m *Manager) Running(id uuid.UUID) (domain.CommandRun, bool) {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if !ok {
		return domain.CommandRun{}, false
	}
	return j.Snapshot(), true
}

// ReadLog reads a run's output for the HTTP API, live or finished.
func (m *Manager) ReadLog(planID, id uuid.UUID, offset, limit int64) ([]byte, int64, error) {
	m.mu.Lock()
	j, ok := m.jobs[id]
	m.mu.Unlock()
	if ok && j.run.PlanID == planID {
		return j.Read(offset, limit)
	}
	path := m.LogPath(planID, id)
	fi, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, 0, ErrLogGone
		}
		return nil, 0, fmt.Errorf("stat log: %w", err)
	}
	data, err := readRange(path, offset, limit, fi.Size())
	return data, fi.Size(), err
}

// LogPath is where a run's output lives.
func (m *Manager) LogPath(planID, id uuid.UUID) string {
	return filepath.Join(m.cfg.LogDir, planID.String(), id.String()+".log")
}

// SweepLogs removes logs older than maxAge and the plan directories it
// empties. A running job's log is never that old: a job lasts an hour at most.
func (m *Manager) SweepLogs(maxAge time.Duration) int {
	cutoff := time.Now().Add(-maxAge)
	plans, err := os.ReadDir(m.cfg.LogDir)
	if err != nil {
		return 0
	}
	removed := 0
	for _, p := range plans {
		if !p.IsDir() {
			continue
		}
		dir := filepath.Join(m.cfg.LogDir, p.Name())
		logs, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		left := len(logs)
		for _, l := range logs {
			info, err := l.Info()
			if err != nil || !strings.HasSuffix(l.Name(), ".log") || info.ModTime().After(cutoff) {
				continue
			}
			if os.Remove(filepath.Join(dir, l.Name())) == nil {
				removed++
				left--
			}
		}
		if left == 0 {
			_ = os.Remove(dir)
		}
	}
	return removed
}

// RunLogSweeper sweeps at boot and then daily until ctx ends.
func (m *Manager) RunLogSweeper(ctx context.Context, maxAge time.Duration) {
	if maxAge <= 0 {
		return
	}
	sweep := func() {
		if n := m.SweepLogs(maxAge); n > 0 {
			slog.Info("command logs swept", "removed", n)
		}
	}
	sweep()
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			sweep()
		}
	}
}

// Shutdown cancels every running job and waits for them to be killed and
// recorded, or for ctx to end.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.mu.Lock()
	m.closed = true
	jobs := make([]*Job, 0, len(m.jobs))
	for _, j := range m.jobs {
		jobs = append(jobs, j)
	}
	m.mu.Unlock()

	for _, j := range jobs {
		j.Cancel(ReasonShutdown)
	}
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("command jobs still running at shutdown: %w", ctx.Err())
	}
}

// ID is the job's id.
func (j *Job) ID() uuid.UUID { return j.run.ID }

// Done is closed once the job has ended and been recorded.
func (j *Job) Done() <-chan struct{} { return j.done }

// Snapshot is the job's current audit record.
func (j *Job) Snapshot() domain.CommandRun {
	j.mu.Lock()
	defer j.mu.Unlock()
	run := j.run
	if run.Status == domain.CommandRunRunning {
		run.OutputBytes, _ = j.log.sizes()
	}
	return run
}

// Wait blocks until the job ends, d passes or ctx ends, and reports whether
// the job has ended.
func (j *Job) Wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-j.done:
		return true
	case <-t.C:
	case <-ctx.Done():
	}
	select {
	case <-j.done:
		return true
	default:
		return false
	}
}

// Cancel ends the job. The first reason given is the one recorded.
func (j *Job) Cancel(reason string) {
	j.mu.Lock()
	if j.cancelReason == "" {
		j.cancelReason = reason
	}
	j.mu.Unlock()
	j.cancel()
}

func (j *Job) endedBy(ctxErr error) (domain.CommandRunStatus, string) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.cancelReason != "" {
		return domain.CommandRunCancelled, j.cancelReason
	}
	if errors.Is(ctxErr, context.DeadlineExceeded) {
		return domain.CommandRunTimedOut, reasonTimedOut
	}
	return domain.CommandRunCancelled, ""
}

// Read returns up to limit bytes of output at offset, and the bytes kept so
// far.
func (j *Job) Read(offset, limit int64) ([]byte, int64, error) {
	stored, _ := j.log.sizes()
	data, err := readRange(j.logPath, offset, limit, stored)
	return data, stored, err
}

// Size is the bytes of output kept so far.
func (j *Job) Size() int64 {
	stored, _ := j.log.sizes()
	return stored
}

// Delivered is the offset the model has read up to.
func (j *Job) Delivered() int64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	return j.delivered
}

// SetDelivered records how far the model has read.
func (j *Job) SetDelivered(offset int64) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if offset > j.delivered {
		j.delivered = offset
	}
}

// UntilState is how a wait_until job's attempts went.
type UntilState struct {
	Attempts   int
	LastExit   int
	LastOutput []byte
	Met        bool
}

// Until returns the attempts so far.
func (j *Job) Until() UntilState {
	j.mu.Lock()
	defer j.mu.Unlock()
	return UntilState{Attempts: j.attempts, LastExit: j.lastExit, LastOutput: j.lastOutput, Met: j.met}
}

func tailBytes(b []byte, n int) []byte {
	if len(b) <= n {
		return append([]byte(nil), b...)
	}
	return append([]byte(nil), b[len(b)-n:]...)
}
