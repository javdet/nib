package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestCreateSubjectsToolDef(t *testing.T) {
	t.Parallel()
	def := CreateSubjectsToolDef()
	if def.Name != CreateSubjectsToolName {
		t.Fatalf("name = %q, want %q", def.Name, CreateSubjectsToolName)
	}
	var schema struct {
		Required []string `json:"required"`
	}
	if err := json.Unmarshal(def.Parameters, &schema); err != nil {
		t.Fatalf("unmarshal parameters: %v", err)
	}
	if len(schema.Required) != 1 || schema.Required[0] != "subjects" {
		t.Fatalf("required = %#v, want [subjects]", schema.Required)
	}
}

func TestCreateSubjectsHandler(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	dialogID := uuid.New()

	t.Run("saves normalized subjects", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.createSubjectsHandler(dialogID)

		out, err := handler(ctx, map[string]any{
			"subjects": []any{"Postgres", " nginx ", "postgres", ""},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		want := []string{"postgres", "nginx"}
		if len(repo.subjects) != len(want) {
			t.Fatalf("subjects = %#v, want %#v", repo.subjects, want)
		}
		for i, subject := range want {
			if repo.subjects[i] != subject {
				t.Fatalf("subjects[%d] = %q, want %q", i, repo.subjects[i], subject)
			}
		}
		if out != "Subjects saved: postgres, nginx" {
			t.Fatalf("out = %q", out)
		}
	})

	t.Run("missing subjects", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.createSubjectsHandler(dialogID)

		out, err := handler(ctx, map[string]any{})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "subjects is required" {
			t.Fatalf("out = %q", out)
		}
		if repo.subjects != nil {
			t.Fatalf("subjects = %#v, want nil", repo.subjects)
		}
	})

	t.Run("empty subjects after normalization", func(t *testing.T) {
		t.Parallel()
		repo := &fakeDialogRepo{}
		svc := &ChatService{dialogRepo: repo}
		handler := svc.createSubjectsHandler(dialogID)

		out, err := handler(ctx, map[string]any{
			"subjects": []any{"", "   "},
		})
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if out != "subjects must contain at least one non-empty word" {
			t.Fatalf("out = %q", out)
		}
	})
}

func TestAddLocalTools_includesCreateSubjectsForDialog(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{CreateSubjectsToolName: {}}

	svc.addLocalTools(&catalog, allow, uuid.New())

	if _, ok := catalog.localHandlers[CreateSubjectsToolName]; !ok {
		t.Fatal("expected create_subjects handler")
	}
	found := false
	for _, def := range catalog.tools {
		if def.Name == CreateSubjectsToolName {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected create_subjects in tool defs")
	}
}

func TestAddLocalTools_excludesCreateSubjectsWithoutAllowList(t *testing.T) {
	t.Parallel()
	svc := &ChatService{}
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	allow := map[string]struct{}{"other_tool": {}}

	svc.addLocalTools(&catalog, allow, uuid.New())

	if _, ok := catalog.localHandlers[CreateSubjectsToolName]; ok {
		t.Fatal("did not expect create_subjects handler when not in allow list")
	}
}

func TestNormalizeSubjects(t *testing.T) {
	t.Parallel()
	got := normalizeSubjects([]string{"Postgres", " nginx ", "POSTGRES", "", "  "})
	want := []string{"postgres", "nginx"}
	if len(got) != len(want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
