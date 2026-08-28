package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadKnowledgeBaseURI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const initial = `
knowledge_base:
  uri: postgres://u:p@db:5432/nib?sslmode=disable
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := LoadKnowledgeBaseURI(path)
	if err != nil {
		t.Fatalf("LoadKnowledgeBaseURI() error = %v", err)
	}
	want := "postgres://u:p@db:5432/nib?sslmode=disable"
	if got != want {
		t.Fatalf("LoadKnowledgeBaseURI() = %q, want %q", got, want)
	}
}

func TestLoadKnowledgeBaseURI_missingURI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("knowledge_base: {}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := LoadKnowledgeBaseURI(path)
	if err == nil || !strings.Contains(err.Error(), "knowledge_base.uri") {
		t.Fatalf("LoadKnowledgeBaseURI() error = %v, want missing uri", err)
	}
}

func TestReadKnowledgeBaseURI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const initial = `
# keep this comment
projects:
  - name: myproject2
knowledge_base:
  uri: postgres://old@db:5432/nib?sslmode=disable
llm:
  model: gpt-test
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := ReadKnowledgeBaseURI(path)
	if err != nil {
		t.Fatalf("ReadKnowledgeBaseURI() error = %v", err)
	}
	want := "postgres://old@db:5432/nib?sslmode=disable"
	if got != want {
		t.Fatalf("ReadKnowledgeBaseURI() = %q, want %q", got, want)
	}
}

func TestReadKnowledgeBaseURI_missingSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("projects: []\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	got, err := ReadKnowledgeBaseURI(path)
	if err != nil {
		t.Fatalf("ReadKnowledgeBaseURI() error = %v", err)
	}
	if got != "" {
		t.Fatalf("ReadKnowledgeBaseURI() = %q, want empty", got)
	}
}

func TestWriteKnowledgeBaseURI_updatesPreservesOtherSections(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const initial = `
# keep this comment
projects:
  - name: myproject2
knowledge_base:
  uri: postgres://old@db:5432/nib?sslmode=disable
llm:
  model: gpt-test
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	const updated = "postgres://new@db:5432/nib?sslmode=disable"
	if err := WriteKnowledgeBaseURI(path, updated); err != nil {
		t.Fatalf("WriteKnowledgeBaseURI() error = %v", err)
	}

	got, err := ReadKnowledgeBaseURI(path)
	if err != nil {
		t.Fatalf("ReadKnowledgeBaseURI() after write error = %v", err)
	}
	if got != updated {
		t.Fatalf("uri after write = %q, want %q", got, updated)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	body := string(raw)
	for _, want := range []string{
		"# keep this comment",
		"projects:",
		"- name: myproject2",
		"llm:",
		"model: gpt-test",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("config after write missing %q\n---\n%s", want, body)
		}
	}
	if strings.Contains(body, "postgres://old@") {
		t.Errorf("config still contains old uri:\n%s", body)
	}
}

func TestWriteKnowledgeBaseURI_insertsSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	const initial = `
projects:
  - name: myproject2
`
	if err := os.WriteFile(path, []byte(initial), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	const uri = "postgres://u:p@postgres:5432/nib?sslmode=disable"
	if err := WriteKnowledgeBaseURI(path, uri); err != nil {
		t.Fatalf("WriteKnowledgeBaseURI() error = %v", err)
	}

	got, err := ReadKnowledgeBaseURI(path)
	if err != nil {
		t.Fatalf("ReadKnowledgeBaseURI() error = %v", err)
	}
	if got != uri {
		t.Fatalf("ReadKnowledgeBaseURI() = %q, want %q", got, uri)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if !strings.Contains(string(raw), "- name: myproject2") {
		t.Fatalf("projects block was dropped:\n%s", raw)
	}
}

func TestReadWriteKnowledgeBaseURI_errors(t *testing.T) {
	t.Parallel()

	if _, err := ReadKnowledgeBaseURI(""); err == nil {
		t.Fatal("ReadKnowledgeBaseURI(\"\") expected error")
	}
	if err := WriteKnowledgeBaseURI("", "x"); err == nil {
		t.Fatal("WriteKnowledgeBaseURI(\"\", ...) expected error")
	}

	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.yaml")
	if _, err := ReadKnowledgeBaseURI(missing); err == nil {
		t.Fatal("ReadKnowledgeBaseURI(missing) expected error")
	}
	if err := WriteKnowledgeBaseURI(missing, "x"); err == nil {
		t.Fatal("WriteKnowledgeBaseURI(missing) expected error")
	}
}
