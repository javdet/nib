package rules

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/javdet/nib/internal/repository"
)

func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	dataDir := t.TempDir()
	svc := NewService(dataDir, "")
	return svc, svc.Dir()
}

func newTestServiceWithRules(t *testing.T, rules map[string]string) *Service {
	t.Helper()
	dataDir := t.TempDir()
	rulesDir := filepath.Join(dataDir, "rules")
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range rules {
		if err := ValidateName(name); err != nil {
			t.Fatalf("test setup: invalid name %q: %v", name, err)
		}
		path := filepath.Join(rulesDir, name+mdSuffix)
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return NewService(dataDir, "")
}

func TestService_InitializeCreatesDir(t *testing.T) {
	svc, rulesDir := newTestService(t)

	got, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got.Rules) != 0 {
		t.Fatalf("Rules = %v, want empty", got.Rules)
	}

	info, err := os.Stat(rulesDir)
	if err != nil {
		t.Fatalf("rules dir should exist: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("rules path is not a directory")
	}
}

func TestService_CRUD(t *testing.T) {
	svc, _ := newTestService(t)
	_, _ = svc.List()

	if err := svc.Create("postgres", "# Postgres rules"); err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := svc.Create("postgres", "dup"); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("Create duplicate = %v, want ErrAlreadyExists", err)
	}

	r, err := svc.Get("postgres")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if r.Content != "# Postgres rules" {
		t.Fatalf("content = %q", r.Content)
	}

	if err := svc.Set("postgres", "# Updated"); err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	r, _ = svc.Get("postgres")
	if r.Content != "# Updated" {
		t.Fatalf("after Set content = %q", r.Content)
	}

	if err := svc.Rename("postgres", "github", "# GitHub rules"); err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if _, err := svc.Get("postgres"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("old name should be gone: %v", err)
	}
	r, _ = svc.Get("github")
	if r.Content != "# GitHub rules" {
		t.Fatalf("renamed content = %q", r.Content)
	}

	if err := svc.Delete("github"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := svc.Get("github"); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("after Delete = %v", err)
	}
}

func TestService_ListExistingRules(t *testing.T) {
	svc := newTestServiceWithRules(t, map[string]string{
		"postgres": "postgres body",
		"github":   "github body",
	})

	got, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got.Rules) != 2 || got.Rules[0] != "github" || got.Rules[1] != "postgres" {
		t.Fatalf("Rules = %v, want [github postgres]", got.Rules)
	}
}

func TestService_RejectPathTraversalNames(t *testing.T) {
	svc, _ := newTestService(t)
	_, _ = svc.List()

	bad := []string{"..", "../x", "foo/bar"}
	for _, name := range bad {
		err := svc.Create(name, "x")
		if err == nil {
			t.Fatalf("Create(%q) succeeded, want error", name)
		}
		if !errors.Is(err, ErrInvalidName) {
			t.Fatalf("Create(%q) error = %v, want ErrInvalidName", name, err)
		}
	}
}

func TestService_InvalidNameRejectedOnAllOperations(t *testing.T) {
	svc := newTestServiceWithRules(t, map[string]string{"ok": "body"})
	bad := []string{"..", "has.dot", "a/b"}

	for _, name := range bad {
		name := name
		t.Run(name, func(t *testing.T) {
			if _, err := svc.Get(name); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("Get() error = %v, want ErrInvalidName", err)
			}
			if err := svc.Set(name, "x"); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("Set() error = %v, want ErrInvalidName", err)
			}
			if err := svc.Delete(name); !errors.Is(err, ErrInvalidName) {
				t.Fatalf("Delete() error = %v, want ErrInvalidName", err)
			}
		})
	}
}

func TestService_SkipsHiddenFiles(t *testing.T) {
	svc := newTestServiceWithRules(t, map[string]string{"visible": "body"})
	hiddenPath := filepath.Join(svc.Dir(), ".hidden.md")
	if err := os.WriteFile(hiddenPath, []byte("hidden"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(got.Rules) != 1 || got.Rules[0] != "visible" {
		t.Fatalf("Rules = %v, want [visible]", got.Rules)
	}
}
