package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/javdet/nib/internal/cmdrun"
	"github.com/javdet/nib/internal/config"
)

// newCommandJobs decides how agent commands run on this host and builds the
// manager they run on.
func newCommandJobs(cfg config.Config, rec cmdrun.Recorder) (*cmdrun.Manager, error) {
	mode, err := cmdrun.ParseIsolationMode(cfg.Commands.Isolation)
	if err != nil {
		return nil, err
	}
	sb, err := cmdrun.NewSandbox(cmdrun.SandboxConfig{
		Isolation: mode,
		UIDStart:  cfg.Commands.UIDStart,
		UIDCount:  cfg.Commands.UIDCount,
	})
	if err != nil {
		return nil, fmt.Errorf("command sandbox: %w", err)
	}

	if sb.Isolated() {
		if err := closeDataDir(cfg.DataDir); err != nil {
			// Under "required" a readable data volume is not isolation; under
			// "auto" the uid switch still keeps jobs off the backend's /proc and
			// the Docker socket, so it runs on with the gap said out loud.
			if mode == cmdrun.IsolationRequired {
				return nil, err
			}
			slog.Error("agent commands can read the data volume", "dir", cfg.DataDir, "error", err)
		}
	}
	sb.Sweep()

	switch {
	case sb.Refused() != nil:
		slog.Error("agent commands are refused", "reason", sb.Summary())
	case !sb.Isolated() && mode != cmdrun.IsolationOff:
		slog.Warn("agent commands are not isolated", "detail", sb.Summary())
	default:
		slog.Info("agent commands sandboxed", "detail", sb.Summary())
	}

	return cmdrun.NewManager(sb, cmdrun.ManagerConfig{
		LogDir: filepath.Join(cfg.DataDir, "command_logs"),
	}, rec), nil
}

// closeDataDir makes DATA_DIR 0700. A job runs as another uid, and the data
// volume holds the webhook key, mcp.json, the rules and every plan; the
// directory's own mode is what keeps a job from walking into any of it.
func closeDataDir(dir string) error {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return fmt.Errorf("resolve data dir: %w", err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return fmt.Errorf("stat data dir: %w", err)
	}
	if fi.Mode().Perm()&0o077 == 0 {
		return nil
	}
	if err := os.Chmod(abs, 0o700); err != nil {
		return fmt.Errorf("chmod data dir to 0700 (it is %v): %w", fi.Mode().Perm(), err)
	}
	return nil
}
