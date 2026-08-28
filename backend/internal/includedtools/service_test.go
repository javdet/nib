package includedtools

import (
	"os"
	"path/filepath"
	"testing"
)

func TestServiceGetSet(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	tools, err := svc.Get("discuss")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(tools) != 0 {
		t.Fatalf("expected empty tools, got %v", tools)
	}

	if err := svc.Set("discuss", []string{"b", "a", "a", " c "}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	got, err := svc.Get("discuss")
	if err != nil {
		t.Fatalf("Get after Set: %v", err)
	}
	want := []string{"a", "b", "c"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}

	all, err := svc.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if len(all["discuss"]) != 3 {
		t.Fatalf("GetAll discuss = %v", all["discuss"])
	}
}

func TestServiceInvalidMode(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if _, err := svc.Get("invalid"); err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if err := svc.Set("invalid", []string{"x"}); err == nil {
		t.Fatal("expected error for invalid mode")
	}
}

func TestServicePersistsToFile(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if err := svc.Set("plan", []string{"query-docs"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, defaultFileName))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("expected non-empty file")
	}

	svc2 := NewService(dir)
	got, err := svc2.Get("plan")
	if err != nil {
		t.Fatalf("Get from new service: %v", err)
	}
	if len(got) != 1 || got[0] != "query-docs" {
		t.Fatalf("got %v", got)
	}
}
