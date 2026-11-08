package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCloseDataDir(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		mode os.FileMode
	}{
		{name: "world-readable volume is closed", mode: 0o755},
		{name: "group-readable volume is closed", mode: 0o750},
		{name: "already closed is left alone", mode: 0o700},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			dir := filepath.Join(t.TempDir(), "data")
			if err := os.Mkdir(dir, tt.mode); err != nil {
				t.Fatal(err)
			}
			if err := os.Chmod(dir, tt.mode); err != nil {
				t.Fatal(err)
			}
			if err := closeDataDir(dir); err != nil {
				t.Fatalf("closeDataDir: %v", err)
			}
			fi, err := os.Stat(dir)
			if err != nil {
				t.Fatal(err)
			}
			if got := fi.Mode().Perm(); got != 0o700 {
				t.Fatalf("mode = %v, want 0700", got)
			}
		})
	}
	if err := closeDataDir(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("closeDataDir on a missing directory returned nil")
	}
}
