package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/cmdrun"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/llm"
)

const (
	RunCommandToolName = "run_command"
	WaitJobToolName    = "wait_job"
	ReadOutputToolName = "read_output"
	CancelJobToolName  = "cancel_job"
	WaitUntilToolName  = "wait_until"

	// commandResultMaxBytes is the most output one tool result hands the
	// model. A job keeps up to 10 MiB on disk; a single kubectl get -o json
	// returned whole would end the turn on the context window.
	commandResultMaxBytes  = 32 << 10
	commandResultHeadBytes = 4 << 10
	commandResultTailBytes = commandResultMaxBytes - commandResultHeadBytes

	defaultCommandWaitSeconds    = 30
	defaultWaitJobSeconds        = 300
	maxCommandWaitSeconds        = 900
	defaultCommandTimeoutSeconds = 600
	maxCommandTimeoutSeconds     = 3600
	defaultUntilIntervalSeconds  = 10
	minUntilIntervalSeconds      = 2
	maxUntilIntervalSeconds      = 300
	defaultUntilTimeoutSeconds   = 300
	maxUntilTimeoutSeconds       = 900
	maxUntilAttemptTimeout       = 2 * time.Minute
	cancelJobWait                = 15 * time.Second
	maxGrepMatches               = 500
	maxGrepLineBytes             = 2 << 10
)

// CommandToolNames are the tools that replaced execute_command, in the order
// an allow list carries them.
var CommandToolNames = []string{RunCommandToolName, WaitJobToolName, ReadOutputToolName, CancelJobToolName, WaitUntilToolName}

// SetCommandJobs wires the job manager the command tools run on, and the
// sandbox api_call shares with it. Nil leaves every command refused.
func (s *ChatService) SetCommandJobs(m *cmdrun.Manager) {
	s.commandJobs = m
}

var errCommandsUnavailable = errors.New("commands are not available on this server")

func RunCommandToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: RunCommandToolName,
		Description: "Run a bash script on the backend host, in a sandbox of its own: a separate user with no access " +
			"to nib's data or the Docker socket, an empty home directory (also the working directory) that is deleted " +
			"afterwards, and only PATH, HOME, TMPDIR and LANG in the environment. Pipes, redirects, && and $VAR all work. " +
			"Returns as soon as the script ends or wait_seconds pass. If it is still running you get its job_id and " +
			"the output so far: call wait_job to keep waiting rather than running a status command again, since one " +
			"long wait costs one round. Output past 32 KiB is cut to its head and tail; read_output reads the rest. " +
			"The script is killed after timeout_seconds, and when your run ends.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "required": ["script"],
  "properties": {
    "script": {"type": "string", "description": "The bash script to run."},
    "wait_seconds": {"type": "integer", "description": "How long to wait for it before returning, 0-900. Default 30."},
    "timeout_seconds": {"type": "integer", "description": "When to kill it, 1-3600. Default 600."}
  }
}`),
	}
}

func WaitJobToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: WaitJobToolName,
		Description: "Wait for a job started by run_command to end, for up to wait_seconds (max 900), and return its " +
			"status, exit code and the output it wrote since you last read it. One call can wait fifteen minutes " +
			"for a rollout or a build; prefer that to polling.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "required": ["job_id"],
  "properties": {
    "job_id": {"type": "string", "description": "The job_id run_command returned."},
    "wait_seconds": {"type": "integer", "description": "How long to wait, 1-900. Default 300."},
    "from_offset": {"type": "integer", "description": "Return output from this byte offset. Default: where your last result ended."}
  }
}`),
	}
}

func ReadOutputToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: ReadOutputToolName,
		Description: "Read a job's full output page by page (up to 32 KiB per call), or with grep, only the lines " +
			"matching a regular expression, numbered from offset. Works while the job runs and after it ends.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "required": ["job_id"],
  "properties": {
    "job_id": {"type": "string", "description": "The job to read."},
    "offset": {"type": "integer", "description": "Byte offset to start at. Default 0."},
    "limit": {"type": "integer", "description": "Bytes to return, at most 32768. Default 32768."},
    "grep": {"type": "string", "description": "Optional regular expression (RE2); only matching lines are returned."}
  }
}`),
	}
}

func CancelJobToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name:        CancelJobToolName,
		Description: "Stop a running job: SIGTERM, then SIGKILL after ten seconds, to everything it started.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "required": ["job_id"],
  "properties": {
    "job_id": {"type": "string", "description": "The job to stop."}
  }
}`),
	}
}

func WaitUntilToolDef() llm.ToolDef {
	return llm.ToolDef{
		Name: WaitUntilToolName,
		Description: "Rerun a short bash script every interval_seconds until it exits 0 (until: exit0) or its output " +
			"matches pattern (until: match), or timeout_seconds pass, without spending a round per check: a rollout " +
			"finishing, a DNS record appearing, an instance becoming ready. It runs in the same sandbox as " +
			"run_command and returns how it ended, the number of attempts and the last attempt's output.",
		Parameters: json.RawMessage(`{
  "type": "object",
  "required": ["script", "until"],
  "properties": {
    "script": {"type": "string", "description": "The check to rerun. Keep it short: each attempt is limited to two minutes."},
    "until": {"type": "string", "enum": ["exit0", "match"], "description": "exit0: stop when the script exits 0. match: stop when its output matches pattern."},
    "pattern": {"type": "string", "description": "Regular expression (RE2) for until: match."},
    "interval_seconds": {"type": "integer", "description": "Pause between attempts, 2-300. Default 10."},
    "timeout_seconds": {"type": "integer", "description": "Give up after this long, 1-900. Default 300."}
  }
}`),
	}
}

// commandScope is what the command tools of one catalog share: who owns the
// jobs they start, and what to record them against.
type commandScope struct {
	s         *ChatService
	owner     string
	planID    uuid.UUID
	dialogID  uuid.UUID
	actionKey string
	mode      string
}

func registerCommandTools(s *ChatService, catalog *toolCatalog, b toolBinding, allow map[string]struct{}) {
	owner := b.dialogID.String()
	if b.dialogID == uuid.Nil {
		// The stateless chat endpoint has no dialog; a scope per catalog keeps
		// two such requests out of each other's jobs.
		owner = "stateless-" + uuid.NewString()
	}
	scope := &commandScope{s: s, owner: owner, planID: b.planID, dialogID: b.dialogID, actionKey: b.actionKey, mode: b.mode}

	tools := []struct {
		name    string
		def     func() llm.ToolDef
		handler localToolHandler
	}{
		{RunCommandToolName, RunCommandToolDef, scope.runCommand},
		{WaitJobToolName, WaitJobToolDef, scope.waitJob},
		{ReadOutputToolName, ReadOutputToolDef, scope.readOutput},
		{CancelJobToolName, CancelJobToolDef, scope.cancelJob},
		{WaitUntilToolName, WaitUntilToolDef, scope.waitUntil},
	}
	for _, t := range tools {
		if !localToolAllowed(allow, t.name) {
			continue
		}
		catalog.localHandlers[t.name] = t.handler
		catalog.tools = append(catalog.tools, t.def())
	}
}

// commandJobResult is what run_command, wait_job and cancel_job return.
type commandJobResult struct {
	JobID           string `json:"job_id"`
	Status          string `json:"status"`
	ExitCode        *int   `json:"exit_code,omitempty"`
	Reason          string `json:"reason,omitempty"`
	Output          string `json:"output"`
	OutputTruncated bool   `json:"output_truncated,omitempty"`
	Offset          int64  `json:"offset"`
	NextOffset      int64  `json:"next_offset"`
	TotalBytes      int64  `json:"total_bytes"`
	OutputCapped    bool   `json:"output_capped,omitempty"`
	Hint            string `json:"hint,omitempty"`
}

func (c *commandScope) manager() (*cmdrun.Manager, error) {
	if c.s.commandJobs == nil {
		return nil, errCommandsUnavailable
	}
	return c.s.commandJobs, nil
}

func (c *commandScope) runCommand(ctx context.Context, args map[string]any) (string, error) {
	script := stringArg(args, "script")
	if strings.TrimSpace(script) == "" {
		return "script is empty", nil
	}
	wait, msg := boundedSeconds(args, "wait_seconds", defaultCommandWaitSeconds, 0, maxCommandWaitSeconds)
	if msg != "" {
		return msg, nil
	}
	timeout, msg := boundedSeconds(args, "timeout_seconds", defaultCommandTimeoutSeconds, 1, maxCommandTimeoutSeconds)
	if msg != "" {
		return msg, nil
	}
	m, err := c.manager()
	if err != nil {
		return err.Error(), nil
	}
	job, err := m.Start(ctx, c.startSpec(script, timeout, nil))
	if err != nil {
		return commandStartRefusal(err), nil
	}
	if wait > 0 {
		job.Wait(ctx, wait)
	}
	return marshalCommandResult(jobResult(job, 0))
}

func (c *commandScope) waitJob(ctx context.Context, args map[string]any) (string, error) {
	job, msg := c.lookup(args)
	if msg != "" {
		return msg, nil
	}
	wait, msg := boundedSeconds(args, "wait_seconds", defaultWaitJobSeconds, 1, maxCommandWaitSeconds)
	if msg != "" {
		return msg, nil
	}
	from := job.Delivered()
	if v, ok, msg := intArg(args, "from_offset"); msg != "" {
		return msg, nil
	} else if ok {
		from = int64(max(v, 0))
	}
	job.Wait(ctx, wait)
	return marshalCommandResult(jobResult(job, from))
}

func (c *commandScope) cancelJob(ctx context.Context, args map[string]any) (string, error) {
	job, msg := c.lookup(args)
	if msg != "" {
		return msg, nil
	}
	job.Cancel(cmdrun.ReasonAgent)
	job.Wait(ctx, cancelJobWait)
	return marshalCommandResult(jobResult(job, job.Delivered()))
}

// readOutputResult is read_output's answer.
type readOutputResult struct {
	JobID      string `json:"job_id"`
	Status     string `json:"status"`
	Offset     int64  `json:"offset"`
	NextOffset int64  `json:"next_offset"`
	TotalBytes int64  `json:"total_bytes"`
	EOF        bool   `json:"eof"`
	Output     string `json:"output"`
	Matches    *int   `json:"matches,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

func (c *commandScope) readOutput(_ context.Context, args map[string]any) (string, error) {
	job, msg := c.lookup(args)
	if msg != "" {
		return msg, nil
	}
	offset := int64(0)
	if v, ok, msg := intArg(args, "offset"); msg != "" {
		return msg, nil
	} else if ok {
		offset = int64(max(v, 0))
	}
	limit := int64(commandResultMaxBytes)
	if v, ok, msg := intArg(args, "limit"); msg != "" {
		return msg, nil
	} else if ok && v > 0 && int64(v) < limit {
		limit = int64(v)
	}
	var re *regexp.Regexp
	if pattern := stringArg(args, "grep"); pattern != "" {
		var err error
		if re, err = regexp.Compile(pattern); err != nil {
			return fmt.Sprintf("invalid grep pattern: %v", err), nil
		}
	}

	status := string(job.Snapshot().Status)
	if re == nil {
		data, size, err := job.Read(offset, limit)
		if err != nil {
			return err.Error(), nil
		}
		next := offset + int64(len(data))
		return marshalCommandResult(readOutputResult{
			JobID: job.ID().String(), Status: status, Offset: offset, NextOffset: next,
			TotalBytes: size, EOF: next >= size, Output: strings.ToValidUTF8(string(data), "\uFFFD"),
		})
	}

	// A grep scans everything from offset, which is bounded by the 10 MiB a
	// job may keep.
	data, size, err := job.Read(offset, math.MaxInt32)
	if err != nil {
		return err.Error(), nil
	}
	var out strings.Builder
	matches, truncated := 0, false
	for i, line := range bytes.Split(data, []byte("\n")) {
		if !re.Match(line) {
			continue
		}
		if len(line) > maxGrepLineBytes {
			// One minified JSON line can be the whole log; a match must still
			// show up rather than overflow the page and vanish.
			line = append(line[:maxGrepLineBytes:maxGrepLineBytes], " ... [line cut]"...)
		}
		entry := strconv.Itoa(i+1) + ": " + strings.ToValidUTF8(string(line), "\uFFFD") + "\n"
		if int64(out.Len()+len(entry)) > limit || matches >= maxGrepMatches {
			truncated = true
			break
		}
		out.WriteString(entry)
		matches++
	}
	return marshalCommandResult(readOutputResult{
		JobID: job.ID().String(), Status: status, Offset: offset, NextOffset: size,
		TotalBytes: size, EOF: true, Output: out.String(), Matches: &matches, Truncated: truncated,
	})
}

// waitUntilResult is wait_until's answer.
type waitUntilResult struct {
	JobID        string `json:"job_id"`
	Status       string `json:"status"`
	Reason       string `json:"reason,omitempty"`
	Attempts     int    `json:"attempts"`
	LastExitCode *int   `json:"last_exit_code,omitempty"`
	LastOutput   string `json:"last_output"`
}

func (c *commandScope) waitUntil(ctx context.Context, args map[string]any) (string, error) {
	script := stringArg(args, "script")
	if strings.TrimSpace(script) == "" {
		return "script is empty", nil
	}
	until := &cmdrun.UntilSpec{}
	switch stringArg(args, "until") {
	case "exit0":
	case "match":
		pattern := stringArg(args, "pattern")
		if pattern == "" {
			return "until: match needs a pattern", nil
		}
		re, err := regexp.Compile(pattern)
		if err != nil {
			return fmt.Sprintf("invalid pattern: %v", err), nil
		}
		until.Pattern = re
	default:
		return `until must be "exit0" or "match"`, nil
	}
	interval, msg := boundedSeconds(args, "interval_seconds", defaultUntilIntervalSeconds, minUntilIntervalSeconds, maxUntilIntervalSeconds)
	if msg != "" {
		return msg, nil
	}
	timeout, msg := boundedSeconds(args, "timeout_seconds", defaultUntilTimeoutSeconds, 1, maxUntilTimeoutSeconds)
	if msg != "" {
		return msg, nil
	}
	until.Interval = interval
	until.AttemptTimeout = min(timeout, maxUntilAttemptTimeout)

	m, err := c.manager()
	if err != nil {
		return err.Error(), nil
	}
	job, err := m.Start(ctx, c.startSpec(script, timeout, until))
	if err != nil {
		return commandStartRefusal(err), nil
	}
	// The job ends by its own deadline at the latest, and with ctx otherwise.
	job.Wait(ctx, timeout+cancelJobWait)

	run := job.Snapshot()
	st := job.Until()
	res := waitUntilResult{
		JobID:      run.ID.String(),
		Status:     untilStatus(run.Status),
		Reason:     run.Reason,
		Attempts:   st.Attempts,
		LastOutput: strings.ToValidUTF8(string(st.LastOutput), "\uFFFD"),
	}
	if st.Attempts > 0 {
		code := st.LastExit
		res.LastExitCode = &code
	}
	return marshalCommandResult(res)
}

func untilStatus(s domain.CommandRunStatus) string {
	switch s {
	case domain.CommandRunSucceeded:
		return "met"
	case domain.CommandRunRunning:
		return "running"
	}
	return string(s)
}

func (c *commandScope) startSpec(script string, timeout time.Duration, until *cmdrun.UntilSpec) cmdrun.StartSpec {
	return cmdrun.StartSpec{
		Owner:     c.owner,
		PlanID:    c.planID,
		DialogID:  c.dialogID,
		ActionKey: c.actionKey,
		Mode:      c.mode,
		Script:    script,
		Timeout:   timeout,
		Until:     until,
	}
}

func (c *commandScope) lookup(args map[string]any) (*cmdrun.Job, string) {
	raw := strings.TrimSpace(stringArg(args, "job_id"))
	if raw == "" {
		return nil, "job_id is empty"
	}
	id, err := uuid.Parse(raw)
	if err != nil {
		return nil, fmt.Sprintf("job_id %q is not a job id", raw)
	}
	m, err := c.manager()
	if err != nil {
		return nil, err.Error()
	}
	job, err := m.Lookup(c.owner, id)
	if err != nil {
		return nil, fmt.Sprintf("no job %s: a job is visible only to the agent run that started it, and only until that run ends", raw)
	}
	return job, ""
}

// jobResult reports a job with the output it wrote from offset from, cut to
// its head and tail when it is over the result limit.
func jobResult(job *cmdrun.Job, from int64) commandJobResult {
	run := job.Snapshot()
	size := job.Size()
	from = min(max(from, 0), size)
	res := commandJobResult{
		JobID:        run.ID.String(),
		Status:       string(run.Status),
		ExitCode:     run.ExitCode,
		Reason:       run.Reason,
		Offset:       from,
		NextOffset:   size,
		TotalBytes:   size,
		OutputCapped: run.OutputCapped,
	}
	if size-from <= commandResultMaxBytes {
		data, _, _ := job.Read(from, size-from)
		res.Output = strings.ToValidUTF8(string(data), "\uFFFD")
	} else {
		head, _, _ := job.Read(from, commandResultHeadBytes)
		tailFrom := size - commandResultTailBytes
		tail, _, _ := job.Read(tailFrom, commandResultTailBytes)
		omittedFrom := from + int64(len(head))
		res.Output = strings.ToValidUTF8(string(head), "\uFFFD") +
			fmt.Sprintf("\n[... %d bytes omitted; read_output with offset %d reads them ...]\n", tailFrom-omittedFrom, omittedFrom) +
			strings.ToValidUTF8(string(tail), "\uFFFD")
		res.OutputTruncated = true
	}
	job.SetDelivered(size)

	switch {
	case run.Status == domain.CommandRunRunning:
		res.Hint = "still running: call wait_job with this job_id to keep waiting, or cancel_job to stop it"
	case res.OutputTruncated:
		res.Hint = "the middle of the output was cut; read_output with grep or an offset reads it"
	}
	if run.OutputCapped {
		res.Hint = strings.TrimSpace(res.Hint + " Output past 10 MiB was not kept.")
	}
	return res
}

func commandStartRefusal(err error) string {
	return "command not started: " + err.Error()
}

func marshalCommandResult(v any) (string, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("marshal command result: %w", err)
	}
	return string(b), nil
}

// boundedSeconds reads an optional whole number of seconds and refuses one
// outside [lo, hi] with a message for the model.
func boundedSeconds(args map[string]any, key string, def, lo, hi int) (time.Duration, string) {
	v, ok, msg := intArg(args, key)
	if msg != "" {
		return 0, msg
	}
	if !ok {
		v = def
	}
	if v < lo || v > hi {
		return 0, fmt.Sprintf("%s must be between %d and %d, got %d", key, lo, hi, v)
	}
	return time.Duration(v) * time.Second, ""
}

// intArg reads an optional integer argument. Models send numbers as JSON
// numbers or, now and then, as strings.
func intArg(args map[string]any, key string) (int, bool, string) {
	raw, present := args[key]
	if !present || raw == nil {
		return 0, false, ""
	}
	switch v := raw.(type) {
	case int:
		return v, true, ""
	case int64:
		return int(v), true, ""
	case json.Number:
		n, err := strconv.Atoi(v.String())
		if err != nil {
			return 0, false, fmt.Sprintf("%s must be a whole number", key)
		}
		return n, true, ""
	case float64:
		if v != math.Trunc(v) || math.Abs(v) > math.MaxInt32 {
			return 0, false, fmt.Sprintf("%s must be a whole number", key)
		}
		return int(v), true, ""
	case string:
		if strings.TrimSpace(v) == "" {
			return 0, false, ""
		}
		n, err := strconv.Atoi(strings.TrimSpace(v))
		if err != nil {
			return 0, false, fmt.Sprintf("%s must be a whole number", key)
		}
		return n, true, ""
	}
	return 0, false, fmt.Sprintf("%s must be a whole number", key)
}
