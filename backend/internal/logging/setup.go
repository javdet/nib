package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/javdet/nib/internal/config"
)

// Setup configures the default slog logger to emit JSON to stderr, optionally
// mirroring to cfg.File when cfg.Enabled is true (same flags as toolchain llmchat).
// Log level is read from cfg.Level (defaults to info; LOG_LEVEL env is applied at config load).
func Setup(cfg config.LogConfig) (cleanup func(), err error) {
	level, err := config.ParseLogLevel(cfg.Level)
	if err != nil {
		return nil, err
	}

	w := io.Writer(os.Stderr)
	var logFile *os.File

	if cfg.Enabled {
		f, err := os.OpenFile(cfg.File, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return nil, fmt.Errorf("logging: open log file %q: %w", cfg.File, err)
		}
		logFile = f
		w = io.MultiWriter(os.Stderr, f)
	}

	slog.SetDefault(slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})))

	return func() {
		if logFile != nil {
			_ = logFile.Close()
		}
	}, nil
}
