package executor

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
)

const (
	// actionLogsTailLines is how much history the daemon is asked for. An agent
	// run emits a lot and the snapshot is re-read every few seconds, so the
	// whole history is never shipped.
	actionLogsTailLines = 2000
	// actionLogsMaxBytes caps what a single read returns, after the tail limit.
	// A single line can be arbitrarily long, so the line count alone bounds
	// nothing.
	actionLogsMaxBytes = 256 * 1024
)

// logsClient is the slice of the Docker client the log read needs.
type logsClient interface {
	ContainerLogs(ctx context.Context, container string, options container.LogsOptions) (io.ReadCloser, error)
}

// ActionLogsRequest identifies the container of an action run. Either
// identifier is enough, exactly as for StopActionRequest: a local run names its
// container after the job.
type ActionLogsRequest struct {
	JobName     string
	ContainerID string
	// Namespace is carried for symmetry with StopActionRequest and is unused
	// while logs are a local-docker-only feature.
	Namespace string
}

// ActionLogsResult is a point-in-time snapshot of a container's output.
type ActionLogsResult struct {
	Logs string `json:"logs"`
	// Truncated says the oldest output was dropped to fit the cap, so the
	// reader is not told a partial transcript is the whole one.
	Truncated bool `json:"truncated"`
}

// ReadActionLogs returns what the agent-runner container has written so far.
//
// It is deliberately a snapshot rather than a follow: the caller polls, so a
// held-open stream would tie a request to the life of the container. No metric
// is recorded either -- the executor counters describe operator-initiated runs
// and stops, and a poll would drown them.
func (s *Service) ReadActionLogs(ctx context.Context, req ActionLogsRequest) (ActionLogsResult, error) {
	if strings.TrimSpace(req.JobName) == "" && strings.TrimSpace(req.ContainerID) == "" {
		return ActionLogsResult{}, ErrLogTargetRequired
	}

	cfg, err := s.config.Get()
	if err != nil {
		return ActionLogsResult{}, err
	}

	switch cfg.Type {
	case TypeDisabled:
		return ActionLogsResult{}, ErrExecutorDisabled
	case TypeLocal:
		return readActionLogsLocal(ctx, req)
	case TypeRemote:
		// Not ErrNotImplemented: remote Kubernetes runs and stops actions fine,
		// so the message has to say that logs in particular are local-only.
		return ActionLogsResult{}, fmt.Errorf("%w: remote %s", ErrLogsUnsupported, cfg.Platform)
	default:
		return ActionLogsResult{}, ErrInvalidType
	}
}

func readActionLogsLocal(ctx context.Context, req ActionLogsRequest) (ActionLogsResult, error) {
	target := strings.TrimSpace(req.ContainerID)
	if target == "" {
		target = strings.TrimSpace(req.JobName)
	}

	cli, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		return ActionLogsResult{}, fmt.Errorf("create docker client: %w", err)
	}
	defer cli.Close()

	return fetchActionLogs(ctx, cli, target)
}

func fetchActionLogs(ctx context.Context, cli logsClient, target string) (ActionLogsResult, error) {
	rc, err := cli.ContainerLogs(ctx, target, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Tail:       strconv.Itoa(actionLogsTailLines),
		Follow:     false,
	})
	if err != nil {
		// The opposite of stopActionLocal, where a container already gone is the
		// outcome asked for: here it means there is nothing left to show, and
		// the caller has to stop asking.
		if client.IsErrNotFound(err) {
			return ActionLogsResult{}, fmt.Errorf("%w: %s", ErrActionContainerGone, target)
		}
		return ActionLogsResult{}, fmt.Errorf("read container logs %q: %w", target, err)
	}
	defer rc.Close()

	// Action containers are created without a TTY (see runActionLocal), so the
	// stream is the multiplexed frame format and has to be demuxed. Both streams
	// go into one buffer on purpose: the operator wants the transcript in
	// emission order, not stdout and stderr side by side.
	var buf bytes.Buffer
	if _, err := stdcopy.StdCopy(&buf, &buf, rc); err != nil {
		return ActionLogsResult{}, fmt.Errorf("read container logs %q: %w", target, err)
	}

	logs, truncated := trimActionLogs(buf.Bytes())
	return ActionLogsResult{Logs: logs, Truncated: truncated}, nil
}

// trimActionLogs keeps the newest actionLogsMaxBytes of output.
//
// The cut is made here rather than with an io.LimitReader on the stream for two
// reasons: a limit keeps the oldest bytes, which is the opposite of what someone
// watching a live run wants, and stopping mid-frame makes StdCopy fail.
func trimActionLogs(b []byte) (string, bool) {
	truncated := false
	if len(b) > actionLogsMaxBytes {
		b = b[len(b)-actionLogsMaxBytes:]
		truncated = true
		// Start on a line boundary so the snapshot does not open mid-sentence.
		if i := bytes.IndexByte(b, '\n'); i >= 0 {
			b = b[i+1:]
		}
	}
	// The cap can split a multi-byte rune, and agent output is not guaranteed
	// to be text at all; fix it here rather than letting the JSON encoder do it
	// silently.
	return strings.ToValidUTF8(string(b), "�"), truncated
}
