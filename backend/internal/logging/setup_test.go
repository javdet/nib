package logging

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/config"
)

// Setup swaps the process-wide slog default, so the tests below cannot run in
// parallel with each other: whoever calls SetDefault last owns every slog call
// made after it, whichever test made it.
func TestSetup_disabled(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	cleanup, err := Setup(config.LogConfig{Enabled: false})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(cleanup)

	slog.Info("stderr only test")
}

func TestSetup_enabled_writesToFile(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	dir := t.TempDir()
	logPath := filepath.Join(dir, "nib.stderr.log")

	cleanup, err := Setup(config.LogConfig{
		Enabled: true,
		File:    logPath,
	})
	if err != nil {
		t.Fatalf("Setup: %v", err)
	}
	t.Cleanup(cleanup)

	const msg = "mirror to file"
	slog.Info(msg, "component", "logging_test")

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, msg) {
		t.Fatalf("log file missing message %q:\n%s", msg, content)
	}
	if !strings.Contains(content, "logging_test") {
		t.Fatalf("log file missing attribute:\n%s", content)
	}
}

func TestSetup_levelFilter(t *testing.T) {
	const debugMsg = "debug-only payload"
	const infoMsg = "info-level payload"

	t.Run("info excludes debug from file", func(t *testing.T) {
		prev := slog.Default()
		t.Cleanup(func() { slog.SetDefault(prev) })

		dir := t.TempDir()
		logPath := filepath.Join(dir, "nib.stderr.log")

		cleanup, err := Setup(config.LogConfig{
			Enabled: true,
			File:    logPath,
			Level:   "info",
		})
		if err != nil {
			t.Fatalf("Setup: %v", err)
		}
		t.Cleanup(cleanup)

		slog.Debug(debugMsg)
		slog.Info(infoMsg)

		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		content := string(data)
		if strings.Contains(content, debugMsg) {
			t.Fatalf("log file should not contain debug message at info level:\n%s", content)
		}
		if !strings.Contains(content, infoMsg) {
			t.Fatalf("log file missing info message:\n%s", content)
		}
	})

	t.Run("debug includes debug in file", func(t *testing.T) {
		prev := slog.Default()
		t.Cleanup(func() { slog.SetDefault(prev) })

		dir := t.TempDir()
		logPath := filepath.Join(dir, "nib.stderr.log")

		cleanup, err := Setup(config.LogConfig{
			Enabled: true,
			File:    logPath,
			Level:   "debug",
		})
		if err != nil {
			t.Fatalf("Setup: %v", err)
		}
		t.Cleanup(cleanup)

		slog.Debug(debugMsg)

		data, err := os.ReadFile(logPath)
		if err != nil {
			t.Fatalf("ReadFile: %v", err)
		}
		content := string(data)
		if !strings.Contains(content, debugMsg) {
			t.Fatalf("log file missing debug message at debug level:\n%s", content)
		}
	})
}

func TestSetup_enabled_openFailure(t *testing.T) {
	prev := slog.Default()
	t.Cleanup(func() { slog.SetDefault(prev) })

	_, err := Setup(config.LogConfig{
		Enabled: true,
		File:    filepath.Join(t.TempDir(), "missing", "nested", "log.log"),
	})
	if err == nil {
		t.Fatal("expected error opening log file in non-existent parent")
	}
	if !strings.Contains(err.Error(), "logging: open log file") {
		t.Fatalf("unexpected error: %v", err)
	}
}
