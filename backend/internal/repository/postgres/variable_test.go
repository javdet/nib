package postgres

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

func TestVariableRepoCRUD(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewVariableRepo(pool)

	company, err := repo.Create(ctx, domain.PromptVariable{
		Scope: "global", Name: "CompanyName", Description: "who", Value: "Acme", Kind: "string", Deletable: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if company.ID == uuid.Nil || company.CreatedAt.IsZero() {
		t.Fatalf("Create = %+v, want a generated id and timestamps", company)
	}
	if _, err := repo.Create(ctx, domain.PromptVariable{Scope: "global", Name: "CompanyName", Kind: "string"}); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("duplicate Create = %v, want ErrAlreadyExists", err)
	}

	// One name may exist once per (scope, scope_name).
	web, err := repo.Create(ctx, domain.PromptVariable{Scope: "project", ScopeName: "web", Name: "Repo", Value: "w", Kind: "string"})
	if err != nil {
		t.Fatalf("Create(project web): %v", err)
	}
	api, err := repo.Create(ctx, domain.PromptVariable{Scope: "project", ScopeName: "api", Name: "Repo", Value: "a", Kind: "string"})
	if err != nil {
		t.Fatalf("Create(project api): %v", err)
	}

	// The schema enforces the scope contract the service validates.
	for name, v := range map[string]domain.PromptVariable{
		"global with scope name": {Scope: "global", ScopeName: "web", Name: "X", Kind: "string"},
		"project without a name": {Scope: "project", Name: "X", Kind: "string"},
		"unknown scope":          {Scope: "team", ScopeName: "a", Name: "X", Kind: "string"},
		"unknown kind":           {Scope: "global", Name: "X", Kind: "map"},
	} {
		if _, err := repo.Create(ctx, v); err == nil || errors.Is(err, repository.ErrAlreadyExists) {
			t.Errorf("Create(%s) = %v, want a constraint error", name, err)
		}
	}

	got, err := repo.GetByID(ctx, company.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.Value != "Acme" || got.Description != "who" || !got.Deletable {
		t.Fatalf("GetByID = %+v, want the created variable", got)
	}

	updated, err := repo.Update(ctx, company.ID, domain.PromptVariable{
		Scope: "global", Name: "CompanyName", Description: "who", Value: "Acme Corp", Kind: "string", Deletable: true,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Value != "Acme Corp" || !updated.UpdatedAt.After(company.UpdatedAt) {
		t.Fatalf("Update = %+v, want the new value and a later updated_at", updated)
	}
	if _, err := repo.Update(ctx, api.ID, domain.PromptVariable{Scope: "project", ScopeName: "web", Name: "Repo", Kind: "string"}); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("Update into an existing name = %v, want ErrAlreadyExists", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	var order []string
	for _, v := range list {
		order = append(order, v.Scope+"/"+v.ScopeName+"/"+v.Name)
	}
	// Ordered by scope, then name; the two project rows share both.
	if len(order) != 3 || order[0] != "global//CompanyName" {
		t.Fatalf("List = %v, want global first, then the two project rows", order)
	}

	if err := repo.Delete(ctx, web.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := repo.GetByID(ctx, web.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetByID after Delete = %v, want ErrNotFound", err)
	}

	unknown := uuid.New()
	if err := repo.Delete(ctx, unknown); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Delete unknown = %v, want ErrNotFound", err)
	}
	if _, err := repo.Update(ctx, unknown, domain.PromptVariable{Scope: "global", Name: "Y", Kind: "string"}); !errors.Is(err, repository.ErrNotFound) {
		t.Errorf("Update unknown = %v, want ErrNotFound", err)
	}
}

// The boot-time writers must never clobber what an operator set.
func TestVariableRepoEnsureAndUpsert(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewVariableRepo(pool)

	edited, err := repo.Create(ctx, domain.PromptVariable{
		Scope: "global", Name: "Wiki", Description: "operator's", Value: "https://wiki", Kind: "string", Deletable: true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.EnsureExists(ctx, domain.PromptVariable{Scope: "global", Name: "Wiki", Value: "default", Kind: "string"}); err != nil {
		t.Fatalf("EnsureExists: %v", err)
	}
	if got := mustGetVariable(t, repo, edited.ID); got.Value != "https://wiki" || got.Description != "operator's" {
		t.Fatalf("after EnsureExists = %+v, want the operator's row untouched", got)
	}

	// EnsureBuiltin claims the name: the row becomes undeletable and takes the
	// built-in's kind, but keeps the operator's value.
	if err := repo.EnsureBuiltin(ctx, domain.PromptVariable{Scope: "global", Name: "Wiki", Value: "default", Kind: "list"}); err != nil {
		t.Fatalf("EnsureBuiltin: %v", err)
	}
	got := mustGetVariable(t, repo, edited.ID)
	if got.Deletable || got.Kind != "list" || got.Value != "https://wiki" {
		t.Fatalf("after EnsureBuiltin = %+v, want deletable=false, kind=list, value kept", got)
	}

	if err := repo.EnsureBuiltin(ctx, domain.PromptVariable{Scope: "global", Name: "Messenger", Value: "slack", Kind: "string"}); err != nil {
		t.Fatalf("EnsureBuiltin (new): %v", err)
	}
	if err := repo.EnsureExists(ctx, domain.PromptVariable{Scope: "global", Name: "hostOS", Value: "linux", Kind: "string", Deletable: true}); err != nil {
		t.Fatalf("EnsureExists (new): %v", err)
	}

	up, err := repo.Upsert(ctx, domain.PromptVariable{Scope: "global", Name: "hostOS", Description: "ignored", Value: "alpine", Kind: "string"})
	if err != nil {
		t.Fatalf("Upsert existing: %v", err)
	}
	if up.Value != "alpine" || up.Description != "" || !up.Deletable {
		t.Fatalf("Upsert existing = %+v, want only the value changed", up)
	}
	fresh, err := repo.Upsert(ctx, domain.PromptVariable{Scope: "global", Name: "hostShell", Value: "bash", Kind: "string"})
	if err != nil {
		t.Fatalf("Upsert new: %v", err)
	}
	if fresh.ID == uuid.Nil || fresh.Value != "bash" {
		t.Fatalf("Upsert new = %+v, want an inserted row", fresh)
	}

	all, err := repo.LoadAll(ctx, nil)
	if err != nil {
		t.Fatalf("LoadAll: %v", err)
	}
	if all["global"]["Messenger"] != "slack" {
		t.Fatalf("LoadAll global.Messenger = %v, want slack", all["global"]["Messenger"])
	}
}

func TestVariableRepoLoadAll(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewVariableRepo(pool)

	for _, v := range []domain.PromptVariable{
		{Scope: "global", Name: "Company", Value: "Acme", Kind: "string"},
		{Scope: "global", Name: "Clouds", Value: `["aws","gcp"]`, Kind: "list"},
		{Scope: "global", Name: "Broken", Value: `not json`, Kind: "list"},
		{Scope: "project", ScopeName: "web", Name: "Repo", Value: "web-repo", Kind: "string"},
		{Scope: "project", ScopeName: "api", Name: "Repo", Value: "api-repo", Kind: "string"},
	} {
		if _, err := repo.Create(ctx, v); err != nil {
			t.Fatalf("Create %s/%s: %v", v.Scope, v.Name, err)
		}
	}

	tests := []struct {
		name       string
		scopeNames map[string]string
		wantRepo   string
	}{
		// With no active project the pick is the first scope_name in order, so it
		// does not change after an UPDATE or a VACUUM.
		{name: "no selection", scopeNames: nil, wantRepo: "api-repo"},
		{name: "selected first", scopeNames: map[string]string{"project": "api"}, wantRepo: "api-repo"},
		{name: "selected later", scopeNames: map[string]string{"project": "web"}, wantRepo: "web-repo"},
		{name: "selection with no row", scopeNames: map[string]string{"project": "ops"}, wantRepo: "api-repo"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.LoadAll(ctx, tt.scopeNames)
			if err != nil {
				t.Fatalf("LoadAll: %v", err)
			}
			if got["project"]["Repo"] != tt.wantRepo {
				t.Fatalf("project.Repo = %v, want %q", got["project"]["Repo"], tt.wantRepo)
			}
			if got["global"]["Company"] != "Acme" {
				t.Fatalf("global.Company = %v, want Acme", got["global"]["Company"])
			}
			if want := []string{"aws", "gcp"}; !reflect.DeepEqual(got["global"]["Clouds"], want) {
				t.Fatalf("global.Clouds = %#v, want %#v", got["global"]["Clouds"], want)
			}
			// A list that does not parse renders as empty rather than failing every prompt.
			if want := []string{}; !reflect.DeepEqual(got["global"]["Broken"], want) {
				t.Fatalf("global.Broken = %#v, want %#v", got["global"]["Broken"], want)
			}
		})
	}
}

func mustGetVariable(t *testing.T, repo *VariableRepo, id uuid.UUID) domain.PromptVariable {
	t.Helper()
	v, err := repo.GetByID(context.Background(), id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	return v
}
