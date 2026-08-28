package mode

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveDir(t *testing.T) {
	t.Parallel()

	data := "/var/data"
	tests := []struct {
		name          string
		dataDir       string
		allowToolsDir string
		want          string
	}{
		{
			name:          "empty uses data tools",
			dataDir:       data,
			allowToolsDir: "",
			want:          filepath.Join(data, "tools"),
		},
		{
			name:          "relative under data",
			dataDir:       data,
			allowToolsDir: "custom/tools",
			want:          filepath.Join(data, "custom/tools"),
		},
		{
			name:          "absolute unchanged",
			dataDir:       data,
			allowToolsDir: "/opt/tools",
			want:          "/opt/tools",
		},
		{
			name:          "whitespace treated as empty",
			dataDir:       data,
			allowToolsDir: "  \t ",
			want:          filepath.Join(data, "tools"),
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveDir(tt.dataDir, tt.allowToolsDir); got != tt.want {
				t.Errorf("ResolveDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestParseAllowToolsFile(t *testing.T) {
	t.Parallel()

	t.Run("happy path trim and dedup", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "allow.json")
		if err := os.WriteFile(p, []byte(`{"allow_tools":["  a  ","b","a", "b"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		got, err := parseAllowToolsFile(p)
		if err != nil {
			t.Fatalf("parseAllowToolsFile: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2: %#v", len(got), got)
		}
		if _, ok := got["a"]; !ok {
			t.Error("missing a")
		}
		if _, ok := got["b"]; !ok {
			t.Error("missing b")
		}
	})

	t.Run("empty allow_tools after trim", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "allow.json")
		if err := os.WriteFile(p, []byte(`{"allow_tools":["  ","\t"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := parseAllowToolsFile(p)
		if err == nil {
			t.Fatal("expected error")
		}
		if !strings.Contains(err.Error(), "empty after trimming") {
			t.Errorf("err = %v", err)
		}
	})

	t.Run("empty array", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "allow.json")
		if err := os.WriteFile(p, []byte(`{"allow_tools":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := parseAllowToolsFile(p)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("missing allow_tools key", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "allow.json")
		if err := os.WriteFile(p, []byte(`{"other":[]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := parseAllowToolsFile(p)
		if err == nil || !strings.Contains(err.Error(), "missing allow_tools") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("invalid JSON", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "allow.json")
		if err := os.WriteFile(p, []byte(`not json`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := parseAllowToolsFile(p)
		if err == nil || !strings.Contains(err.Error(), "invalid JSON") {
			t.Fatalf("err = %v", err)
		}
	})

	t.Run("allow_tools not array of strings", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "allow.json")
		if err := os.WriteFile(p, []byte(`{"allow_tools":123}`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := parseAllowToolsFile(p)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("top level must be object", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		p := filepath.Join(dir, "allow.json")
		if err := os.WriteFile(p, []byte(`["x"]`), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := parseAllowToolsFile(p)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestLoadAllowList(t *testing.T) {
	t.Parallel()

	t.Run("mode file when present", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		modePath := filepath.Join(dir, "plan.json")
		if err := os.WriteFile(modePath, []byte(`{"allow_tools":["x"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := LoadAllowList(dir, "plan")
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := a["x"]; !ok {
			t.Fatalf("got %#v", a)
		}
	})

	t.Run("mode file missing skipped", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		a, err := LoadAllowList(dir, "plan")
		if err != nil {
			t.Fatal(err)
		}
		if a != nil {
			t.Fatalf("want nil list, got %#v", a)
		}
	})

	t.Run("empty dir skipped", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		modePath := filepath.Join(dir, "plan.json")
		if err := os.WriteFile(modePath, []byte(`{"allow_tools":["x"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := LoadAllowList("", "plan")
		if err != nil {
			t.Fatal(err)
		}
		if a != nil {
			t.Fatalf("want nil, got %#v", a)
		}
	})

	t.Run("empty mode skipped", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		modePath := filepath.Join(dir, "plan.json")
		if err := os.WriteFile(modePath, []byte(`{"allow_tools":["x"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		a, err := LoadAllowList(dir, "")
		if err != nil {
			t.Fatal(err)
		}
		if a != nil {
			t.Fatalf("want nil, got %#v", a)
		}
	})
}
