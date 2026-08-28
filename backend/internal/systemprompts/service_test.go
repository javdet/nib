package systemprompts

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/javdet/nib/internal/repository"
)

func newTestService(t *testing.T) *Service {
	t.Helper()
	return NewService(t.TempDir(), "")
}

func writeOverride(t *testing.T, s *Service, name, content string) {
	t.Helper()
	if err := os.MkdirAll(s.Dir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(s.OverridePath(name), []byte(content), 0o600); err != nil {
		t.Fatalf("write override %s: %v", name, err)
	}
}

func TestGet_EmbeddedDefault(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	want, ok := Default("plan")
	if !ok {
		t.Fatal("no embedded plan prompt")
	}

	got, err := s.Get("plan")
	if err != nil {
		t.Fatalf("Get(plan) = %v", err)
	}
	if got.Source != SourceDefault {
		t.Errorf("Source = %q, want %q", got.Source, SourceDefault)
	}
	if got.Content != want {
		t.Error("content does not match the embedded prompt")
	}
	if got.Name != "plan" {
		t.Errorf("Name = %q, want plan", got.Name)
	}
}

func TestGet_Override(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	writeOverride(t, s, EditableName, "MARKER")

	got, err := s.Get(EditableName)
	if err != nil {
		t.Fatalf("Get(%s) = %v", EditableName, err)
	}
	if got.Source != SourceOverride {
		t.Errorf("Source = %q, want %q", got.Source, SourceOverride)
	}
	if got.Content != "MARKER" {
		t.Errorf("Content = %q, want MARKER", got.Content)
	}
}

func TestGet_UnknownName(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	if _, err := s.Get("nosuchprompt"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("Get(nosuchprompt) = %v, want ErrNotFound", err)
	}
}

func TestGet_InvalidName(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	for _, name := range []string{"", "../x", "x/y", "has.dot"} {
		if _, err := s.Get(name); !errors.Is(err, ErrInvalidName) {
			t.Errorf("Get(%q) = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestSet_OnlyDiscuss(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		prompt  string
		wantErr error
	}{
		{name: "discuss is editable", prompt: EditableName, wantErr: nil},
		{name: "plan is not", prompt: "plan", wantErr: ErrNotEditable},
		{name: "incident is not", prompt: "incident", wantErr: ErrNotEditable},
		{name: "unknown name is not", prompt: "whatever", wantErr: ErrNotEditable},
		// Name validation must run before the editability check.
		{name: "traversal is invalid, not merely uneditable", prompt: "../x", wantErr: ErrInvalidName},
		{name: "empty is invalid", prompt: "", wantErr: ErrInvalidName},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			s := newTestService(t)
			err := s.Set(tt.prompt, "body")

			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("Set(%q) = %v, want nil", tt.prompt, err)
				}
				if _, statErr := os.Stat(s.OverridePath(tt.prompt)); statErr != nil {
					t.Fatalf("override file not written: %v", statErr)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Set(%q) = %v, want %v", tt.prompt, err, tt.wantErr)
			}
			entries, _ := os.ReadDir(s.Dir())
			if len(entries) != 0 {
				t.Fatalf("rejected write left files behind: %v", entries)
			}
		})
	}
}

func TestSet_CreatesDirectory(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	if err := os.RemoveAll(s.Dir()); err != nil {
		t.Fatalf("remove dir: %v", err)
	}
	if err := s.Set(EditableName, "body"); err != nil {
		t.Fatalf("Set = %v", err)
	}
	if _, err := os.Stat(s.OverridePath(EditableName)); err != nil {
		t.Fatalf("override not written: %v", err)
	}
}

func TestSet_ThenGetReadsOverride(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	if err := s.Set(EditableName, "MARKER-1"); err != nil {
		t.Fatalf("Set = %v", err)
	}

	got, err := s.Get(EditableName)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if got.Content != "MARKER-1" || got.Source != SourceOverride {
		t.Fatalf("Get = %+v, want MARKER-1 from override", got)
	}

	has, err := s.HasOverride(EditableName)
	if err != nil || !has {
		t.Fatalf("HasOverride = (%v, %v), want (true, nil)", has, err)
	}
}

func TestReset_RestoresDefault(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	if err := s.Set(EditableName, "MARKER-1"); err != nil {
		t.Fatalf("Set = %v", err)
	}
	if err := s.Reset(EditableName); err != nil {
		t.Fatalf("Reset = %v", err)
	}

	got, err := s.Get(EditableName)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if got.Source != SourceDefault {
		t.Fatalf("Source = %q, want %q", got.Source, SourceDefault)
	}
	want, _ := Default(EditableName)
	if got.Content != want {
		t.Error("content does not match the embedded prompt after reset")
	}
	if has, _ := s.HasOverride(EditableName); has {
		t.Error("HasOverride = true after reset")
	}
}

func TestReset_Idempotent(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	for i := 0; i < 2; i++ {
		if err := s.Reset(EditableName); err != nil {
			t.Fatalf("Reset #%d = %v, want nil", i+1, err)
		}
	}
}

func TestReset_NonEditable(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	writeOverride(t, s, "plan", "operator edit")

	if err := s.Reset("plan"); !errors.Is(err, ErrNotEditable) {
		t.Fatalf("Reset(plan) = %v, want ErrNotEditable", err)
	}
	if _, err := os.Stat(s.OverridePath("plan")); err != nil {
		t.Fatalf("rejected reset deleted the file anyway: %v", err)
	}
}

func TestPrepare_PrunesRedundantOverride(t *testing.T) {
	t.Parallel()

	embedded, _ := Default(EditableName)
	tests := map[string]string{
		"exact":            embedded,
		"trailing newline": embedded + "\n",
		"trimmed":          strings.TrimRight(embedded, "\n"),
	}

	for name, content := range tests {
		content := content
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			s := newTestService(t)
			writeOverride(t, s, EditableName, content)

			hk, err := s.Prepare()
			if err != nil {
				t.Fatalf("Prepare = %v", err)
			}
			if !hk.Pruned {
				t.Error("Pruned = false, want true")
			}
			if hk.OverrideActive {
				t.Error("OverrideActive = true, want false")
			}
			if _, err := os.Stat(s.OverridePath(EditableName)); !errors.Is(err, os.ErrNotExist) {
				t.Errorf("override still present: %v", err)
			}
		})
	}
}

func TestPrepare_KeepsEditedOverride(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	writeOverride(t, s, EditableName, "a genuinely edited prompt")

	hk, err := s.Prepare()
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if hk.Pruned {
		t.Error("Pruned = true, want false")
	}
	if !hk.OverrideActive {
		t.Error("OverrideActive = false, want true")
	}
	if hk.OverridePath != filepath.Join(s.Dir(), EditableName+".md") {
		t.Errorf("OverridePath = %q", hk.OverridePath)
	}

	got, err := s.Get(EditableName)
	if err != nil {
		t.Fatalf("Get = %v", err)
	}
	if got.Source != SourceOverride {
		t.Errorf("Source = %q, want %q", got.Source, SourceOverride)
	}
}

func TestPrepare_ReportsStaleFilesWithoutDeleting(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	stale := []string{"plan.md", "default.md", ".active.json", "actions.json"}
	if err := os.MkdirAll(s.Dir(), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, name := range stale {
		if err := os.WriteFile(filepath.Join(s.Dir(), name), []byte("x"), 0o600); err != nil {
			t.Fatalf("seed %s: %v", name, err)
		}
	}

	hk, err := s.Prepare()
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if len(hk.Stale) != len(stale) {
		t.Fatalf("Stale = %v, want %d entries", hk.Stale, len(stale))
	}
	for _, name := range stale {
		if _, err := os.Stat(filepath.Join(s.Dir(), name)); err != nil {
			t.Errorf("Prepare deleted %s: %v", name, err)
		}
	}
}

func TestPrepare_FreshVolumeCreatesDir(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	hk, err := s.Prepare()
	if err != nil {
		t.Fatalf("Prepare = %v", err)
	}
	if hk.Pruned || hk.OverrideActive || len(hk.Stale) != 0 {
		t.Errorf("Prepare on a fresh volume = %+v, want zero findings", hk)
	}
	info, err := os.Stat(s.Dir())
	if err != nil || !info.IsDir() {
		t.Fatalf("prompts directory not created: %v", err)
	}
}

func TestService_ConcurrentSetResetGet(t *testing.T) {
	t.Parallel()

	s := newTestService(t)
	embedded, _ := Default(EditableName)
	bodies := []string{"body-0", "body-1", "body-2", "body-3"}
	allowed := map[string]bool{embedded: true}
	for _, b := range bodies {
		allowed[b] = true
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			for n := 0; n < 20; n++ {
				switch (i + n) % 3 {
				case 0:
					if err := s.Set(EditableName, bodies[i%len(bodies)]); err != nil {
						t.Errorf("Set: %v", err)
						return
					}
				case 1:
					if err := s.Reset(EditableName); err != nil {
						t.Errorf("Reset: %v", err)
						return
					}
				default:
					got, err := s.Get(EditableName)
					if err != nil {
						t.Errorf("Get: %v", err)
						return
					}
					if !allowed[got.Content] {
						t.Errorf("Get returned a torn or unexpected body: %q", got.Content)
						return
					}
				}
			}
		}()
	}
	wg.Wait()
}
