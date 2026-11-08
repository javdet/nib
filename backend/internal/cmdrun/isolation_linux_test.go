//go:build linux

package cmdrun

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/javdet/nib/internal/domain"
)

// newIsolatedManager needs a backend that can switch uids: root, as in the
// throwaway test container, or the capabilities the image's entrypoint keeps,
// which is how these tests are also run behind the real entrypoint. Anywhere
// else they skip.
func newIsolatedManager(t *testing.T, uidStart int) *Manager {
	t.Helper()
	base := t.TempDir()
	// t.TempDir and its parent are 0700; the pool uids have to traverse them to
	// reach their homes, the way they traverse /tmp in the image.
	for _, dir := range []string{base, filepath.Dir(base)} {
		if err := os.Chmod(dir, 0o711); err != nil {
			t.Fatal(err)
		}
	}
	sb, err := NewSandbox(SandboxConfig{Isolation: IsolationAuto, UIDStart: uidStart, UIDCount: 50, JobsRoot: filepath.Join(base, "jobs")})
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	if !sb.Isolated() {
		t.Skipf("uid isolation unavailable here: %s", sb.Summary())
	}
	return NewManager(sb, ManagerConfig{LogDir: t.TempDir()}, nil)
}

func TestIsolatedJobRunsAsPoolUserWithoutGroupsOrCaps(t *testing.T) {
	t.Parallel()
	m := newIsolatedManager(t, 41000)
	// Stands in for DATA_DIR, which the backend makes 0700 at boot.
	secret := t.TempDir()
	if err := os.Chmod(secret, 0o700); err != nil {
		t.Fatal(err)
	}
	script := `id -u; id -G; grep CapEff /proc/self/status; ls ` + secret + ` >/dev/null 2>&1 && echo READABLE || echo denied; pwd`
	j := startJob(t, m, context.Background(), script, time.Minute)
	run := waitDone(t, j)
	if run.Status != domain.CommandRunSucceeded || !run.Isolated {
		t.Fatalf("Status, Isolated = %q, %v; want succeeded, true (reason %q)", run.Status, run.Isolated, run.Reason)
	}
	lines := strings.Split(strings.TrimSpace(readAll(t, j)), "\n")
	if len(lines) != 5 {
		t.Fatalf("output = %q, want five lines", lines)
	}
	uid, _ := strconv.Atoi(lines[0])
	if uid != run.UID || !m.sb.pool.contains(uid) {
		t.Fatalf("uid = %q, want the job's pool uid %d", lines[0], run.UID)
	}
	if lines[1] != lines[0] {
		t.Fatalf("groups = %q, want only the job's own gid %s", lines[1], lines[0])
	}
	if !strings.HasSuffix(lines[2], "0000000000000000") {
		t.Fatalf("child capabilities = %q, want none", lines[2])
	}
	if lines[3] != "denied" {
		t.Fatalf("a 0700 directory of another user was %s", lines[3])
	}
	if !strings.HasPrefix(lines[4], m.sb.jobsRoot) {
		t.Fatalf("working directory = %q, want the job's home under %s", lines[4], m.sb.jobsRoot)
	}
	if _, err := os.Stat(lines[4]); !os.IsNotExist(err) {
		t.Fatalf("job home still exists after the job: %v", err)
	}
}

func TestIsolatedJobLeavesNothingRunning(t *testing.T) {
	t.Parallel()
	m := newIsolatedManager(t, 41100)
	// setsid takes the sleeper out of the job's process group, which a kill by
	// group would miss and a kill by uid does not.
	j := startJob(t, m, context.Background(), `setsid sleep 300 >/dev/null 2>&1 & sleep 300 >/dev/null 2>&1 & echo started`, time.Minute)
	run := waitDone(t, j)
	if run.Status != domain.CommandRunSucceeded {
		t.Fatalf("Status = %q, want succeeded", run.Status)
	}
	if left := uidProcs(run.UID, run.UID); len(left) != 0 {
		t.Fatalf("processes left running as uid %d: %v", run.UID, left)
	}
}

func TestIsolatedCancelKillsEverything(t *testing.T) {
	t.Parallel()
	m := newIsolatedManager(t, 41200)
	j := startJob(t, m, context.Background(), `setsid sleep 300 >/dev/null 2>&1 & sleep 300`, time.Minute)
	time.Sleep(200 * time.Millisecond)
	j.Cancel(ReasonOperator)
	run := waitDone(t, j)
	if run.Status != domain.CommandRunCancelled {
		t.Fatalf("Status = %q, want cancelled", run.Status)
	}
	if left := uidProcs(run.UID, run.UID); len(left) != 0 {
		t.Fatalf("processes left running as uid %d: %v", run.UID, left)
	}
}

func TestRunOnceIsolated(t *testing.T) {
	t.Parallel()
	m := newIsolatedManager(t, 41300)
	res, err := m.Sandbox().RunOnce(context.Background(), []string{"id", "-u"}, 10*time.Second, 1024)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	uid, _ := strconv.Atoi(strings.TrimSpace(string(res.Output)))
	if res.ExitCode != 0 || !m.sb.pool.contains(uid) {
		t.Fatalf("RunOnce = %q exit %d, want a pool uid and exit 0", res.Output, res.ExitCode)
	}
}

func TestSweepKillsLeftoverPoolProcesses(t *testing.T) {
	t.Parallel()
	m := newIsolatedManager(t, 41400)
	// A process running as a pool uid with no job behind it, as a live reload
	// of the backend would leave.
	p, err := m.sb.start([]string{"sleep", "300"}, "/", childEnv("/nonexistent"), 41449, nil)
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	m.sb.Sweep()
	select {
	case <-p.done:
	case <-time.After(10 * time.Second):
		t.Fatal("leftover pool process survived Sweep")
	}
}
