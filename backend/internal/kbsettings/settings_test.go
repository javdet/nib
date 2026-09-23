package kbsettings

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFile(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		dataDir      string
		settingsFile string
		want         string
	}{
		{name: "default", dataDir: "/data", settingsFile: "", want: "/data/knowledge.json"},
		{name: "blank is the default", dataDir: "/data", settingsFile: "   ", want: "/data/knowledge.json"},
		{name: "relative joins under dataDir", dataDir: "/data", settingsFile: "kb/opts.json", want: "/data/kb/opts.json"},
		{name: "absolute is used as-is", dataDir: "/data", settingsFile: "/etc/nib/kb.json", want: "/etc/nib/kb.json"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveFile(tt.dataDir, tt.settingsFile); got != tt.want {
				t.Fatalf("ResolveFile(%q, %q) = %q, want %q", tt.dataDir, tt.settingsFile, got, tt.want)
			}
		})
	}
}

// TestGetOnAFreshVolumeIsOn is the upgrade contract: an install that has never
// seen the file gets the feature, rather than having to find the switch first.
func TestGetOnAFreshVolumeIsOn(t *testing.T) {
	t.Parallel()

	store := NewStore(t.TempDir(), "")
	got, err := store.Get()
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if !got.AutoUpdate {
		t.Fatal("AutoUpdate = false, want true on a volume with no settings file")
	}
	if _, err := os.Stat(store.Path()); err != nil {
		t.Fatalf("settings file was not created: %v", err)
	}
}

// TestGetWithTheKeyAbsentIsOn covers a file written before autoUpdate existed:
// decoding straight into a bool would read Go's zero value and silently turn the
// feature off on upgrade.
func TestGetWithTheKeyAbsentIsOn(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "knowledge.json")
	if err := os.WriteFile(path, []byte(`{"somethingElse": 1}`), 0o644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	got, err := NewStore(dir, "").Get()
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if !got.AutoUpdate {
		t.Fatal("AutoUpdate = false, want true when the key is absent")
	}
}

func TestSetGetRoundTrip(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir, "")
	if err := store.Set(Settings{AutoUpdate: false}); err != nil {
		t.Fatalf("Set err = %v", err)
	}

	got, err := store.Get()
	if err != nil {
		t.Fatalf("Get err = %v", err)
	}
	if got.AutoUpdate {
		t.Fatal("AutoUpdate = true, want false after Set")
	}

	// A second store over the same file reads what the first wrote: the value
	// lives on the volume, not in the process.
	reopened, err := NewStore(dir, "").Get()
	if err != nil {
		t.Fatalf("Get after reopen err = %v", err)
	}
	if reopened.AutoUpdate {
		t.Fatal("AutoUpdate = true after reopening, want false")
	}
}

// TestSetLeavesNoTempFile guards the rename-based write: a temp file left in the
// directory would be served to nobody but would accumulate on every save.
func TestSetLeavesNoTempFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	store := NewStore(dir, "")
	if err := store.Set(Settings{AutoUpdate: false}); err != nil {
		t.Fatalf("Set err = %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir err = %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".knowledge-") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Fatalf("len(entries) = %d, want 1 (just knowledge.json)", len(entries))
	}
}

func TestGetWithInvalidJSON(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "knowledge.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatalf("write settings file: %v", err)
	}

	_, err := NewStore(dir, "").Get()
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("Get err = %v, want ErrInvalidJSON", err)
	}
}
