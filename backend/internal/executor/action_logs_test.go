package executor

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/errdefs"
	"github.com/docker/docker/pkg/stdcopy"
)

// fakeLogsClient records what the log read asked the daemon for and replays a
// canned multiplexed stream.
type fakeLogsClient struct {
	body []byte
	err  error

	targets []string
	opts    []container.LogsOptions
	closed  bool
}

func (f *fakeLogsClient) ContainerLogs(_ context.Context, target string, opts container.LogsOptions) (io.ReadCloser, error) {
	f.targets = append(f.targets, target)
	f.opts = append(f.opts, opts)
	if f.err != nil {
		return nil, f.err
	}
	return &trackingReadCloser{Reader: bytes.NewReader(f.body), closed: &f.closed}, nil
}

// muxLog builds the frame format the daemon sends for a container with no TTY,
// so the fixtures exercise the demux rather than assuming plain bytes.
func muxLog(frames ...[2]string) []byte {
	var buf bytes.Buffer
	for _, f := range frames {
		stream := stdcopy.Stdout
		if f[0] == "stderr" {
			stream = stdcopy.Stderr
		}
		w := stdcopy.NewStdWriter(&buf, stream)
		if _, err := w.Write([]byte(f[1])); err != nil {
			panic(err)
		}
	}
	return buf.Bytes()
}

func TestFetchActionLogs(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("x", actionLogsMaxBytes) + "\nlast line\n"

	tests := []struct {
		name          string
		body          []byte
		want          string
		wantTruncated bool
	}{
		{
			name: "stdout only",
			body: muxLog([2]string{"stdout", "cloning repository\n"}),
			want: "cloning repository\n",
		},
		{
			name: "stdout and stderr interleaved in write order",
			body: muxLog(
				[2]string{"stdout", "step one\n"},
				[2]string{"stderr", "warning: slow\n"},
				[2]string{"stdout", "step two\n"},
			),
			want: "step one\nwarning: slow\nstep two\n",
		},
		{
			name: "empty stream",
			body: muxLog(),
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			cli := &fakeLogsClient{body: tt.body}
			got, err := fetchActionLogs(context.Background(), cli, "nib-12345678")
			if err != nil {
				t.Fatalf("fetchActionLogs() error = %v, want nil", err)
			}
			if got.Logs != tt.want {
				t.Fatalf("Logs = %q, want %q", got.Logs, tt.want)
			}
			if got.Truncated != tt.wantTruncated {
				t.Fatalf("Truncated = %v, want %v", got.Truncated, tt.wantTruncated)
			}
			if !cli.closed {
				t.Fatal("log stream was not closed")
			}
		})
	}

	t.Run("oversize output keeps the newest lines", func(t *testing.T) {
		t.Parallel()

		cli := &fakeLogsClient{body: muxLog([2]string{"stdout", long})}
		got, err := fetchActionLogs(context.Background(), cli, "nib-12345678")
		if err != nil {
			t.Fatalf("fetchActionLogs() error = %v, want nil", err)
		}
		if !got.Truncated {
			t.Fatal("Truncated = false, want true")
		}
		if len(got.Logs) > actionLogsMaxBytes {
			t.Fatalf("len(Logs) = %d, want <= %d", len(got.Logs), actionLogsMaxBytes)
		}
		if !strings.HasSuffix(got.Logs, "last line\n") {
			t.Fatalf("Logs does not end with the newest line: %q", tail(got.Logs))
		}
		// The head is what gets dropped, so the snapshot must not open mid-line.
		if strings.HasPrefix(got.Logs, "x") {
			t.Fatal("Logs starts mid-line, want the cut aligned to a newline")
		}
	})

	t.Run("options ask for a bounded snapshot of both streams", func(t *testing.T) {
		t.Parallel()

		cli := &fakeLogsClient{body: muxLog()}
		if _, err := fetchActionLogs(context.Background(), cli, "nib-12345678"); err != nil {
			t.Fatalf("fetchActionLogs() error = %v, want nil", err)
		}
		if len(cli.opts) != 1 {
			t.Fatalf("ContainerLogs called %d times, want 1", len(cli.opts))
		}
		opts := cli.opts[0]
		if !opts.ShowStdout || !opts.ShowStderr {
			t.Fatalf("ShowStdout/ShowStderr = %v/%v, want both true", opts.ShowStdout, opts.ShowStderr)
		}
		if opts.Follow {
			t.Fatal("Follow = true, want a snapshot")
		}
		if opts.Tail != "2000" {
			t.Fatalf("Tail = %q, want %q", opts.Tail, "2000")
		}
	})

	t.Run("missing container", func(t *testing.T) {
		t.Parallel()

		cli := &fakeLogsClient{err: errdefs.NotFound(errors.New("no such container"))}
		_, err := fetchActionLogs(context.Background(), cli, "nib-12345678")
		if !errors.Is(err, ErrActionContainerGone) {
			t.Fatalf("error = %v, want ErrActionContainerGone", err)
		}
	})

	t.Run("daemon failure is passed on", func(t *testing.T) {
		t.Parallel()

		cli := &fakeLogsClient{err: errors.New("dial unix: permission denied")}
		_, err := fetchActionLogs(context.Background(), cli, "nib-12345678")
		if err == nil || errors.Is(err, ErrActionContainerGone) {
			t.Fatalf("error = %v, want the daemon failure", err)
		}
	})
}

func tail(s string) string {
	if len(s) > 60 {
		return s[len(s)-60:]
	}
	return s
}

func TestReadActionLogsByType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cfg  Config
		req  ActionLogsRequest
		want error
	}{
		{
			name: "no target",
			cfg:  Config{Type: TypeLocal},
			req:  ActionLogsRequest{},
			want: ErrLogTargetRequired,
		},
		{
			name: "disabled executor",
			cfg:  Config{Type: TypeDisabled},
			req:  ActionLogsRequest{JobName: "nib-12345678"},
			want: ErrExecutorDisabled,
		},
		{
			name: "remote kubernetes",
			cfg:  Config{Type: TypeRemote, Platform: PlatformKubernetes},
			req:  ActionLogsRequest{JobName: "nib-12345678"},
			want: ErrLogsUnsupported,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			store := NewConfigStore(t.TempDir(), "executor.json", "http://localhost:8080")
			if err := store.Set(tt.cfg); err != nil {
				t.Fatalf("set config: %v", err)
			}

			svc := NewService(store, Secrets{})
			_, err := svc.ReadActionLogs(context.Background(), tt.req)
			if !errors.Is(err, tt.want) {
				t.Fatalf("error = %v, want %v", err, tt.want)
			}
		})
	}
}
