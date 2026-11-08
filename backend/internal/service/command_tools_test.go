package service

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/cmdrun"
)

// newCommandTestService wires a job manager that runs as the test's own user,
// so these tests run anywhere; isolation is tested in cmdrun.
func newCommandTestService(t *testing.T) *ChatService {
	t.Helper()
	sb, err := cmdrun.NewSandbox(cmdrun.SandboxConfig{Isolation: cmdrun.IsolationOff, JobsRoot: filepath.Join(t.TempDir(), "jobs")})
	if err != nil {
		t.Fatalf("NewSandbox: %v", err)
	}
	s := &ChatService{}
	s.SetCommandJobs(cmdrun.NewManager(sb, cmdrun.ManagerConfig{LogDir: t.TempDir()}, nil))
	return s
}

func commandCatalog(s *ChatService, b toolBinding) *toolCatalog {
	catalog := &toolCatalog{localHandlers: map[string]localToolHandler{}}
	registerCommandTools(s, catalog, b, nil)
	return catalog
}

func callTool(t *testing.T, catalog *toolCatalog, ctx context.Context, name string, args map[string]any) string {
	t.Helper()
	h, ok := catalog.localHandlers[name]
	if !ok {
		t.Fatalf("%s not registered", name)
	}
	out, err := h(ctx, args)
	if err != nil {
		t.Fatalf("%s returned a Go error: %v", name, err)
	}
	return out
}

func decodeJobResult(t *testing.T, out string) commandJobResult {
	t.Helper()
	var res commandJobResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("result %q is not JSON: %v", out, err)
	}
	return res
}

func TestRunCommandFinishesWithinWait(t *testing.T) {
	t.Parallel()
	s := newCommandTestService(t)
	catalog := commandCatalog(s, toolBinding{dialogID: uuid.New(), planID: uuid.New(), mode: "execute", actionKey: "s0.step1"})

	res := decodeJobResult(t, callTool(t, catalog, context.Background(), RunCommandToolName, map[string]any{
		"script": "echo one && echo two | tr a-z A-Z",
	}))
	if res.Status != "succeeded" || res.ExitCode == nil || *res.ExitCode != 0 {
		t.Fatalf("result = %+v, want succeeded with exit 0", res)
	}
	if res.Output != "one\nTWO\n" || res.NextOffset != int64(len(res.Output)) {
		t.Fatalf("Output, NextOffset = %q, %d", res.Output, res.NextOffset)
	}
	if run, ok := s.commandJobs.Running(uuid.MustParse(res.JobID)); !ok || run.ActionKey != "s0.step1" || run.Mode != "execute" {
		t.Fatalf("recorded run = %+v, %v; want action key and mode from the binding", run, ok)
	}
}

func TestRunCommandThenWaitJob(t *testing.T) {
	t.Parallel()
	s := newCommandTestService(t)
	catalog := commandCatalog(s, toolBinding{dialogID: uuid.New(), mode: "execute"})
	ctx := context.Background()

	res := decodeJobResult(t, callTool(t, catalog, ctx, RunCommandToolName, map[string]any{
		"script": "echo early; sleep 1; echo late", "wait_seconds": 0,
	}))
	if res.Status != "running" || res.Hint == "" {
		t.Fatalf("first result = %+v, want running with a hint", res)
	}
	waited := decodeJobResult(t, callTool(t, catalog, ctx, WaitJobToolName, map[string]any{
		"job_id": res.JobID, "wait_seconds": float64(30),
	}))
	if waited.Status != "succeeded" {
		t.Fatalf("wait_job = %+v, want succeeded", waited)
	}
	// wait_job continues from what the first result already delivered.
	if waited.Offset != res.NextOffset || res.Output+waited.Output != "early\nlate\n" {
		t.Fatalf("wait_job output = %q from %d, first result ended at %d with %q", waited.Output, waited.Offset, res.NextOffset, res.Output)
	}
}

func TestCommandResultIsCutToHeadAndTail(t *testing.T) {
	t.Parallel()
	s := newCommandTestService(t)
	catalog := commandCatalog(s, toolBinding{dialogID: uuid.New()})

	res := decodeJobResult(t, callTool(t, catalog, context.Background(), RunCommandToolName, map[string]any{
		"script": "head -c 100000 /dev/zero | tr '\\0' a; printf END",
	}))
	if !res.OutputTruncated || len(res.Output) > commandResultMaxBytes+200 || res.TotalBytes != 100003 {
		t.Fatalf("truncated, len, total = %v, %d, %d", res.OutputTruncated, len(res.Output), res.TotalBytes)
	}
	if !strings.HasSuffix(res.Output, "END") || !strings.Contains(res.Output, "read_output with offset 4096") {
		t.Fatalf("output does not keep the tail and point at the omitted part: ...%q", res.Output[len(res.Output)-100:])
	}

	var page readOutputResult
	out := callTool(t, catalog, context.Background(), ReadOutputToolName, map[string]any{"job_id": res.JobID, "offset": 99990})
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatal(err)
	}
	if page.Output != "aaaaaaaaaaEND" || !page.EOF {
		t.Fatalf("read_output page = %+v", page)
	}
	out = callTool(t, catalog, context.Background(), ReadOutputToolName, map[string]any{"job_id": res.JobID, "grep": "END$"})
	if err := json.Unmarshal([]byte(out), &page); err != nil {
		t.Fatal(err)
	}
	if page.Matches == nil || *page.Matches != 1 || !strings.HasPrefix(page.Output, "1: ") {
		t.Fatalf("grep = %+v, want one match on line 1", page)
	}
}

func TestCommandJobsAreScopedToTheirDialog(t *testing.T) {
	t.Parallel()
	s := newCommandTestService(t)
	mine := commandCatalog(s, toolBinding{dialogID: uuid.New()})
	theirs := commandCatalog(s, toolBinding{dialogID: uuid.New()})

	res := decodeJobResult(t, callTool(t, mine, context.Background(), RunCommandToolName, map[string]any{"script": "echo secret"}))
	out := callTool(t, theirs, context.Background(), ReadOutputToolName, map[string]any{"job_id": res.JobID})
	if !strings.HasPrefix(out, "no job ") {
		t.Fatalf("another dialog read the job: %q", out)
	}
}

func TestCommandDiesWithTheRun(t *testing.T) {
	t.Parallel()
	s := newCommandTestService(t)
	dialogID := uuid.New()
	catalog := commandCatalog(s, toolBinding{dialogID: dialogID})
	ctx, cancel := context.WithCancel(context.Background())

	res := decodeJobResult(t, callTool(t, catalog, ctx, RunCommandToolName, map[string]any{"script": "sleep 60", "wait_seconds": 0}))
	job, err := s.commandJobs.Lookup(dialogID.String(), uuid.MustParse(res.JobID))
	if err != nil {
		t.Fatalf("Lookup: %v", err)
	}
	cancel()
	if !job.Wait(context.Background(), 20*time.Second) {
		t.Fatal("job outlived the run that started it")
	}
	if run := job.Snapshot(); run.Status != "cancelled" || run.Reason != cmdrun.ReasonRunEnded {
		t.Fatalf("Status, Reason = %q, %q", run.Status, run.Reason)
	}
}

func TestWaitUntilTool(t *testing.T) {
	t.Parallel()
	s := newCommandTestService(t)
	catalog := commandCatalog(s, toolBinding{dialogID: uuid.New()})

	out := callTool(t, catalog, context.Background(), WaitUntilToolName, map[string]any{
		"script": `test -f "$HOME/ready" && echo state=ready || { touch "$HOME/ready"; echo state=pending; }`,
		"until":  "match", "pattern": "state=ready", "interval_seconds": 2,
	})
	var res waitUntilResult
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("result %q: %v", out, err)
	}
	if res.Status != "met" || res.Attempts != 2 || !strings.Contains(res.LastOutput, "state=ready") {
		t.Fatalf("wait_until = %+v, want met on the second attempt", res)
	}
}

func TestCommandToolArgumentErrors(t *testing.T) {
	t.Parallel()
	s := newCommandTestService(t)
	catalog := commandCatalog(s, toolBinding{dialogID: uuid.New()})
	tests := []struct {
		tool string
		args map[string]any
		want string
	}{
		{RunCommandToolName, map[string]any{"script": "  "}, "script is empty"},
		{RunCommandToolName, map[string]any{"script": "true", "wait_seconds": 901}, "wait_seconds must be between 0 and 900"},
		{RunCommandToolName, map[string]any{"script": "true", "timeout_seconds": 1.5}, "timeout_seconds must be a whole number"},
		{WaitJobToolName, map[string]any{"job_id": "nope"}, `job_id "nope" is not a job id`},
		{WaitJobToolName, map[string]any{"job_id": uuid.NewString()}, "no job "},
		{WaitUntilToolName, map[string]any{"script": "true", "until": "soon"}, `until must be "exit0" or "match"`},
		{WaitUntilToolName, map[string]any{"script": "true", "until": "match"}, "until: match needs a pattern"},
		{ReadOutputToolName, map[string]any{"job_id": uuid.NewString(), "grep": "("}, "no job "},
	}
	for _, tt := range tests {
		if out := callTool(t, catalog, context.Background(), tt.tool, tt.args); !strings.HasPrefix(out, tt.want) {
			t.Errorf("%s(%v) = %q, want it to start with %q", tt.tool, tt.args, out, tt.want)
		}
	}
}

func TestCommandToolsWithoutAManager(t *testing.T) {
	t.Parallel()
	catalog := commandCatalog(&ChatService{}, toolBinding{dialogID: uuid.New()})
	if out := callTool(t, catalog, context.Background(), RunCommandToolName, map[string]any{"script": "true"}); out != errCommandsUnavailable.Error() {
		t.Fatalf("run_command without a manager = %q", out)
	}
}
