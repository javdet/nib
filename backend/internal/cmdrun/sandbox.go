// Package cmdrun runs the commands an agent asks for: one shell script per job,
// each in a sandbox of its own, with its output on disk and its lifetime bound
// to the agent run that started it.
//
// The sandbox is a uid. When the backend can switch uids (root, or the
// CAP_SETUID/CAP_SETGID/CAP_KILL the image's entrypoint leaves it), every job
// runs as a uid of its own from a pool, with no supplementary groups. That
// closes what a same-uid child could reach: the backend's /proc entries, the
// data directory (mode 0700) and the Docker socket's group. Without those
// capabilities it falls back to the backend's own uid, which is no worse than
// the execute_command this replaces.
package cmdrun

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
)

// IsolationMode is commands.isolation from config.yaml.
type IsolationMode string

const (
	// IsolationAuto isolates when the backend can and falls back to its own
	// uid, with a warning at boot, when it cannot.
	IsolationAuto IsolationMode = "auto"
	// IsolationRequired refuses every command when isolation is unavailable.
	IsolationRequired IsolationMode = "required"
	// IsolationOff always runs commands as the backend's own uid.
	IsolationOff IsolationMode = "off"
)

// ParseIsolationMode reads the config value, with an empty one meaning auto.
func ParseIsolationMode(s string) (IsolationMode, error) {
	switch IsolationMode(strings.ToLower(strings.TrimSpace(s))) {
	case "", IsolationAuto:
		return IsolationAuto, nil
	case IsolationRequired:
		return IsolationRequired, nil
	case IsolationOff:
		return IsolationOff, nil
	}
	return "", fmt.Errorf("commands.isolation must be auto, required or off, got %q", s)
}

var (
	// ErrIsolationUnavailable is every spawn's answer under
	// commands.isolation: required on a backend that cannot switch uids.
	ErrIsolationUnavailable = errors.New("commands are disabled: commands.isolation is required, but the backend cannot run them as a separate user (it needs root or CAP_SETUID, CAP_SETGID and CAP_KILL)")
	// ErrNoFreeUID means every uid in the pool is held by a running job.
	ErrNoFreeUID = errors.New("every sandbox user is busy; wait for a running command to finish")
)

const (
	defaultUIDStart = 20000
	defaultUIDCount = 1000
	// killGrace is how long a cancelled job has between SIGTERM and SIGKILL.
	killGrace = 10 * time.Second
	// helperTimeout bounds the small mkdir and rm runs around a job.
	helperTimeout = 30 * time.Second
	defaultPath   = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

// SandboxConfig is the commands block of config.yaml plus where job homes go.
type SandboxConfig struct {
	Isolation IsolationMode
	UIDStart  int
	UIDCount  int
	// JobsRoot holds one home directory per job. Empty means
	// $TMPDIR/nib-jobs, which is outside the data volume on purpose: a job's
	// home is scratch space and must not survive the job.
	JobsRoot string
}

// Sandbox spawns processes the way every agent command runs.
type Sandbox struct {
	isolated bool
	// refuse, when set, is returned instead of spawning anything.
	refuse error
	// wrapper is prepended to every argv. It lowers the inheritable and ambient
	// capability sets in the child: the entrypoint hands the backend its
	// capabilities as ambient ones, and those survive both execve and a switch
	// between two non-zero uids, so a job would otherwise be able to setuid
	// straight back to the backend's user.
	wrapper  []string
	shell    []string
	pool     *uidPool
	jobsRoot string
	summary  string
}

// NewSandbox decides once, at boot, how commands will run on this host.
func NewSandbox(cfg SandboxConfig) (*Sandbox, error) {
	mode := cfg.Isolation
	if mode == "" {
		mode = IsolationAuto
	}
	start, count := cfg.UIDStart, cfg.UIDCount
	if start <= 0 {
		start = defaultUIDStart
	}
	if count <= 0 {
		count = defaultUIDCount
	}
	root := strings.TrimSpace(cfg.JobsRoot)
	if root == "" {
		root = filepath.Join(os.TempDir(), "nib-jobs")
	}

	sb := &Sandbox{jobsRoot: root}
	host := probeHost()

	if setpriv, err := exec.LookPath("setpriv"); err == nil {
		if host.ambientCaps {
			sb.wrapper = []string{setpriv, "--inh-caps=-all", "--ambient-caps=-all", "--"}
		}
	} else if host.ambientCaps {
		// Without setpriv a child would inherit the backend's capabilities, so
		// running nothing is the only safe answer.
		sb.refuse = errors.New("commands are disabled: the backend holds ambient capabilities and setpriv is not installed to drop them in the child")
	}

	if bash, err := exec.LookPath("bash"); err == nil {
		sb.shell = []string{bash, "--noprofile", "--norc", "-c"}
	} else if sh, err := exec.LookPath("sh"); err == nil {
		sb.shell = []string{sh, "-c"}
	} else if sb.refuse == nil {
		sb.refuse = errors.New("commands are disabled: neither bash nor sh is on PATH")
	}

	switch {
	case mode == IsolationOff:
		sb.summary = "commands run as the backend's own user (commands.isolation: off)"
	case host.canSwitch:
		sb.isolated = true
		sb.pool = newUIDPool(start, count)
		sb.summary = fmt.Sprintf("commands run as uids %d-%d, one per job", start, start+count-1)
	case mode == IsolationRequired:
		if sb.refuse == nil {
			sb.refuse = ErrIsolationUnavailable
		}
		sb.summary = "commands are refused: isolation is required but unavailable (" + host.reason + ")"
	default:
		sb.summary = "commands run as the backend's own user: isolation is unavailable (" + host.reason + ")"
	}
	if sb.refuse != nil && mode != IsolationRequired {
		sb.summary = sb.refuse.Error()
	}

	// 01733 when isolated: every pool uid may create its own home in it, none
	// may list or remove another's.
	perm := os.FileMode(0o700)
	if sb.isolated {
		perm = 0o1733
	}
	if err := os.MkdirAll(root, perm); err != nil {
		return nil, fmt.Errorf("create jobs root %s: %w", root, err)
	}
	if err := os.Chmod(root, perm); err != nil {
		return nil, fmt.Errorf("chmod jobs root %s: %w", root, err)
	}
	return sb, nil
}

// Isolated reports whether jobs run as a uid of their own.
func (s *Sandbox) Isolated() bool { return s.isolated }

// Summary is one sentence on how commands run, for the boot log.
func (s *Sandbox) Summary() string { return s.summary }

// Refused is the error every spawn returns, or nil when commands can run.
func (s *Sandbox) Refused() error { return s.refuse }

// childEnv is everything a job sees of an environment. The backend runs with
// SECRETS_ENCRYPTION_KEY, LLM_API_KEY, the database DSN and the webhook key,
// and a job's output goes to the LLM provider and the transcript, so an
// inherited environment would let `env` read them all out.
func childEnv(home string) []string {
	path := os.Getenv("PATH")
	if path == "" {
		path = defaultPath
	}
	env := []string{"PATH=" + path, "HOME=" + home, "TMPDIR=" + filepath.Join(home, "tmp")}
	if lang, ok := os.LookupEnv("LANG"); ok {
		env = append(env, "LANG="+lang)
	}
	return env
}

// proc is one spawned process and whatever it left behind.
type proc struct {
	cmd  *exec.Cmd
	uid  int
	done chan struct{}
}

func (p *proc) exitCode() int {
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

// start spawns argv in its own process group. out receives stdout and stderr;
// nil discards both. uid < 0 keeps the backend's own.
func (s *Sandbox) start(argv []string, dir string, env []string, uid int, out *os.File) (*proc, error) {
	full := make([]string, 0, len(s.wrapper)+len(argv))
	full = append(full, s.wrapper...)
	full = append(full, argv...)

	cmd := exec.Command(full[0], full[1:]...)
	cmd.Dir = dir
	cmd.Env = env
	// An *os.File goes to the child as is, so Wait returns when the process
	// exits rather than when every grandchild has closed the pipe.
	if out != nil {
		cmd.Stdout = out
		cmd.Stderr = out
	}
	cmd.SysProcAttr = sysProcAttr(uid)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	p := &proc{cmd: cmd, uid: uid, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

// signal reaches every process the job started. By uid when isolated, which
// also catches anything that left the process group with setsid, and by
// process group otherwise.
func (s *Sandbox) signal(p *proc, sig syscall.Signal) {
	if p.uid >= 0 {
		_, _ = signalUID(p.uid, sig)
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, sig)
}

// stop asks the job to end, then makes it.
func (s *Sandbox) stop(p *proc, grace time.Duration) {
	s.signal(p, syscall.SIGTERM)
	select {
	case <-p.done:
	case <-time.After(grace):
	}
	s.signal(p, syscall.SIGKILL)
	select {
	case <-p.done:
	case <-time.After(5 * time.Second):
	}
}

// sweep kills whatever the job left running after its main process ended, so
// a backgrounded `sleep` cannot outlive it or keep its uid busy.
func (s *Sandbox) sweep(p *proc) {
	if p.uid >= 0 {
		killUID(p.uid, 5*time.Second)
		return
	}
	_ = syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL)
}

// runProcess runs argv to completion or until ctx ends, copying its output into
// out. It returns the exit code (-1 when killed) and whether ctx ended it.
func (s *Sandbox) runProcess(ctx context.Context, argv []string, home string, uid int, out io.Writer) (int, bool, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return -1, false, fmt.Errorf("output pipe: %w", err)
	}
	p, err := s.start(argv, home, childEnv(home), uid, w)
	_ = w.Close()
	if err != nil {
		_ = r.Close()
		return -1, false, fmt.Errorf("start: %w", err)
	}

	copied := make(chan struct{})
	go func() {
		_, _ = io.Copy(out, r)
		close(copied)
	}()

	stopped := false
	select {
	case <-p.done:
	case <-ctx.Done():
		stopped = true
		s.stop(p, killGrace)
	}
	s.sweep(p)
	select {
	case <-copied:
	case <-time.After(2 * time.Second):
	}
	_ = r.Close()
	<-copied
	return p.exitCode(), stopped, nil
}

// runHelper runs a short bookkeeping command (mkdir, rm) as uid.
func (s *Sandbox) runHelper(argv []string, uid int) error {
	ctx, cancel := context.WithTimeout(context.Background(), helperTimeout)
	defer cancel()
	out := &headBuffer{max: 1024}
	code, stopped, err := s.runProcess(ctx, argv, "/", uid, out)
	switch {
	case err != nil:
		return err
	case stopped:
		return fmt.Errorf("%s timed out", argv[0])
	case code != 0:
		return fmt.Errorf("%s exited %d: %s", argv[0], code, strings.TrimSpace(string(out.Bytes())))
	}
	return nil
}

// makeHome creates a job's home and temp directory, owned by the job's uid.
// The backend cannot chown, so an isolated job's home is made by that uid.
func (s *Sandbox) makeHome(name string, uid int) (string, error) {
	home := filepath.Join(s.jobsRoot, name)
	if uid < 0 {
		if err := os.MkdirAll(filepath.Join(home, "tmp"), 0o700); err != nil {
			return "", fmt.Errorf("create job home: %w", err)
		}
		return home, nil
	}
	if err := s.runHelper([]string{"mkdir", "-m", "0700", "--", home, filepath.Join(home, "tmp")}, uid); err != nil {
		return "", fmt.Errorf("create job home: %w", err)
	}
	return home, nil
}

// removeHome deletes a job's home, as the uid that owns everything in it.
func (s *Sandbox) removeHome(home string, uid int) {
	if uid < 0 {
		_ = os.RemoveAll(home)
		return
	}
	_ = s.runHelper([]string{"rm", "-rf", "--", home}, uid)
}

func (s *Sandbox) acquireUID() (int, error) {
	if !s.isolated {
		return -1, nil
	}
	return s.pool.acquire()
}

func (s *Sandbox) releaseUID(uid int) {
	if uid >= 0 {
		s.pool.release(uid)
	}
}

// OnceResult is what RunOnce captured.
type OnceResult struct {
	Output    []byte
	Truncated bool
	ExitCode  int
	TimedOut  bool
}

// RunOnce runs argv to completion in a sandbox of its own and returns its
// output, capped at maxOutput bytes. It is api_call's path: a fixed binary,
// no shell, nothing kept afterwards.
func (s *Sandbox) RunOnce(ctx context.Context, argv []string, timeout time.Duration, maxOutput int) (OnceResult, error) {
	if s.refuse != nil {
		return OnceResult{}, s.refuse
	}
	uid, err := s.acquireUID()
	if err != nil {
		return OnceResult{}, err
	}
	defer s.releaseUID(uid)

	home, err := s.makeHome("once-"+uuid.NewString(), uid)
	if err != nil {
		return OnceResult{}, err
	}
	defer s.removeHome(home, uid)

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	buf := &headBuffer{max: maxOutput}
	code, stopped, err := s.runProcess(runCtx, argv, home, uid, buf)
	if err != nil {
		return OnceResult{}, err
	}
	return OnceResult{
		Output:    buf.Bytes(),
		Truncated: buf.Truncated(),
		ExitCode:  code,
		TimedOut:  stopped && errors.Is(runCtx.Err(), context.DeadlineExceeded),
	}, nil
}

// Sweep clears what a previous backend process left behind: processes still
// running as pool uids (a live reload restarts the backend without restarting
// its container) and job homes. It runs once at boot.
func (s *Sandbox) Sweep() {
	if s.isolated {
		killUIDRange(s.pool.start, s.pool.start+s.pool.count-1)
	}
	entries, err := os.ReadDir(s.jobsRoot)
	if err != nil {
		return
	}
	for _, e := range entries {
		path := filepath.Join(s.jobsRoot, e.Name())
		uid := ownerUID(path)
		if s.isolated && s.pool.contains(uid) {
			s.removeHome(path, uid)
			continue
		}
		_ = os.RemoveAll(path)
	}
}

// headBuffer keeps the first max bytes written to it and drops the rest.
type headBuffer struct {
	mu    sync.Mutex
	max   int
	buf   []byte
	total int64
}

func (b *headBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.total += int64(len(p))
	if room := b.max - len(b.buf); room > 0 {
		if len(p) < room {
			room = len(p)
		}
		b.buf = append(b.buf, p[:room]...)
	}
	return len(p), nil
}

// Truncated reports whether anything was dropped.
func (b *headBuffer) Truncated() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.total > int64(len(b.buf))
}

func (b *headBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.buf...)
}
