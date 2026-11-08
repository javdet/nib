package cmdrun

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

// logFile is a job's output on disk, capped so a runaway command cannot fill
// the data volume. What goes past the cap is counted and dropped.
type logFile struct {
	mu       sync.Mutex
	f        *os.File
	max      int64
	stored   int64
	produced int64
}

func createLog(path string, max int64) (*logFile, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, fmt.Errorf("create log directory: %w", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create log: %w", err)
	}
	return &logFile{f: f, max: max}, nil
}

func (l *logFile) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.produced += int64(len(p))
	if l.f == nil {
		return len(p), nil
	}
	if room := l.max - l.stored; room > 0 {
		chunk := p
		if int64(len(chunk)) > room {
			chunk = chunk[:room]
		}
		n, err := l.f.Write(chunk)
		l.stored += int64(n)
		if err != nil {
			// The job keeps running; its output simply stops being kept.
			_ = l.f.Close()
			l.f = nil
		}
	}
	return len(p), nil
}

// sizes returns the bytes kept and the bytes the job produced.
func (l *logFile) sizes() (stored, produced int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.stored, l.produced
}

func (l *logFile) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f != nil {
		_ = l.f.Close()
		l.f = nil
	}
}

// ErrLogGone means the run's output is no longer on disk: retention removed
// it, or it was never written.
var ErrLogGone = errors.New("the command's output is no longer kept")

// readRange reads up to limit bytes at offset from the log at path, never past
// size, which is what the writer has committed.
func readRange(path string, offset, limit, size int64) ([]byte, error) {
	if offset < 0 {
		offset = 0
	}
	if offset >= size || limit <= 0 {
		return nil, nil
	}
	if offset+limit > size {
		limit = size - offset
	}
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrLogGone
		}
		return nil, fmt.Errorf("open log: %w", err)
	}
	defer f.Close()
	buf := make([]byte, limit)
	n, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("read log: %w", err)
	}
	return buf[:n], nil
}
