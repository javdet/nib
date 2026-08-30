package kbdoc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestServiceGet(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		collection string
		seed       string // written to {collection}.md before the read
		wantSource Source
		want       string
		wantErr    error
	}{
		{
			name:       "no file falls back to the skeleton",
			collection: "default",
			wantSource: SourceTemplate,
			want:       Skeleton(),
		},
		{
			name:       "uploaded file wins",
			collection: "default",
			seed:       "# Infra\nprod runs in fra1\n",
			wantSource: SourceUploaded,
			want:       "# Infra\nprod runs in fra1\n",
		},
		{
			name:       "dotted collection name is allowed",
			collection: "kiss2.prod",
			seed:       "# Dotted\n",
			wantSource: SourceUploaded,
			want:       "# Dotted\n",
		},
		{
			name:       "empty name rejected",
			collection: "",
			wantErr:    ErrInvalidName,
		},
		{
			name:       "traversal rejected",
			collection: "../../etc/passwd",
			wantErr:    ErrInvalidName,
		},
		{
			name:       "parent directory rejected",
			collection: "..",
			wantErr:    ErrInvalidName,
		},
		{
			name:       "separator rejected",
			collection: "a/b",
			wantErr:    ErrInvalidName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := NewService(t.TempDir(), "")
			if tt.seed != "" {
				if err := svc.Save(tt.collection, tt.seed); err != nil {
					t.Fatalf("seed: %v", err)
				}
			}

			doc, err := svc.Get(tt.collection)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Get() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Get() unexpected error: %v", err)
			}
			if doc.Content != tt.want {
				t.Errorf("Content = %q, want %q", doc.Content, tt.want)
			}
			if doc.Source != tt.wantSource {
				t.Errorf("Source = %q, want %q", doc.Source, tt.wantSource)
			}
			if doc.Collection != tt.collection {
				t.Errorf("Collection = %q, want %q", doc.Collection, tt.collection)
			}
			if tt.wantSource == SourceUploaded && doc.UpdatedAt.IsZero() {
				t.Error("UpdatedAt is zero for an uploaded document")
			}
			if tt.wantSource == SourceTemplate && !doc.UpdatedAt.IsZero() {
				t.Error("UpdatedAt is set for the template")
			}
		})
	}
}

// The template is served, never seeded: an install that has not uploaded
// anything must keep tracking the skeleton shipped by the current image.
func TestServiceGetDoesNotWriteTheTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	svc := NewService(dir, "")

	if _, err := svc.Get("default"); err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}

	if _, err := os.Stat(filepath.Join(svc.Dir(), "default.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("template was written to disk: stat err = %v", err)
	}
}

func TestServiceSaveOverwrites(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir(), "")

	if err := svc.Save("default", "first"); err != nil {
		t.Fatalf("Save() first: %v", err)
	}
	if err := svc.Save("default", "second"); err != nil {
		t.Fatalf("Save() second: %v", err)
	}

	doc, err := svc.Get("default")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if doc.Content != "second" {
		t.Errorf("Content = %q, want %q", doc.Content, "second")
	}
}

func TestServiceSaveRejectsInvalidName(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir(), "")

	if err := svc.Save("../escape", "x"); !errors.Is(err, ErrInvalidName) {
		t.Fatalf("Save() error = %v, want %v", err, ErrInvalidName)
	}
}

// Collections are independent: uploading for one project must not disturb another.
func TestServiceCollectionsAreIndependent(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir(), "")

	if err := svc.Save("alpha", "alpha doc"); err != nil {
		t.Fatalf("Save(alpha): %v", err)
	}
	if err := svc.Save("beta", "beta doc"); err != nil {
		t.Fatalf("Save(beta): %v", err)
	}

	for collection, want := range map[string]string{"alpha": "alpha doc", "beta": "beta doc"} {
		doc, err := svc.Get(collection)
		if err != nil {
			t.Fatalf("Get(%s): %v", collection, err)
		}
		if doc.Content != want {
			t.Errorf("Get(%s) = %q, want %q", collection, doc.Content, want)
		}
	}
}

func TestSkeleton(t *testing.T) {
	t.Parallel()

	if err := ValidateEmbeddedSkeleton(); err != nil {
		t.Fatalf("ValidateEmbeddedSkeleton() = %v", err)
	}

	// The headings are the contract with the operator filling the template in;
	// dropping one silently would leave a gap the agent never learns about.
	for _, heading := range []string{
		"## Project Overview",
		"## Environments",
		"## Traffic Routing",
		"## Data Layer",
		"## Infrastructure as Code",
		"## CI/CD",
		"## Secrets Management",
		"## Observability",
		"## Constraints and Guardrails",
	} {
		if !strings.Contains(Skeleton(), heading) {
			t.Errorf("skeleton is missing %q", heading)
		}
	}
}

func TestResolveDir(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dataDir string
		docsDir string
		want    string
	}{
		{name: "empty uses the default under dataDir", dataDir: "/data", want: "/data/knowledgebase"},
		{name: "relative resolves under dataDir", dataDir: "/data", docsDir: "kb", want: "/data/kb"},
		{name: "absolute is used as-is", dataDir: "/data", docsDir: "/srv/kb", want: "/srv/kb"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := ResolveDir(tt.dataDir, tt.docsDir); got != tt.want {
				t.Errorf("ResolveDir() = %q, want %q", got, tt.want)
			}
		})
	}
}

// Get degrades to the template rather than failing when the stored document
// cannot be read, so a sick volume does not break the knowledge base view.
func TestServiceGetUnreadableFileFallsBack(t *testing.T) {
	t.Parallel()

	svc := NewService(t.TempDir(), "")
	if err := svc.Save("default", "stored"); err != nil {
		t.Fatalf("Save(): %v", err)
	}

	path := filepath.Join(svc.Dir(), "default.md")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	// Running as root defeats the permission bit; skip rather than assert.
	if _, err := os.ReadFile(path); err == nil {
		t.Skip("file is still readable (running as root)")
	}

	doc, err := svc.Get("default")
	if err != nil {
		t.Fatalf("Get() unexpected error: %v", err)
	}
	if doc.Source != SourceTemplate {
		t.Errorf("Source = %q, want %q", doc.Source, SourceTemplate)
	}
}
