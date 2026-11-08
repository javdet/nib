package postgres

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

func TestSecretRepoCRUD(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewSecretRepo(pool)

	// Not valid UTF-8 on purpose: the column is bytea and must take ciphertext as is.
	cipher := []byte{0x00, 0xff, 0x10, 0x80}
	gh, err := repo.Create(ctx, domain.PromptSecret{Scope: "global", Name: "GH_TOKEN", Description: "github"}, cipher)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if gh.ID == uuid.Nil || gh.Description != "github" {
		t.Fatalf("Create = %+v, want a generated id and the description", gh)
	}
	if _, err := repo.Create(ctx, domain.PromptSecret{Scope: "global", Name: "GH_TOKEN"}, []byte("x")); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("duplicate Create = %v, want ErrAlreadyExists", err)
	}
	if _, err := repo.Create(ctx, domain.PromptSecret{Scope: "global", ScopeName: "web", Name: "BAD"}, []byte("x")); err == nil || errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("Create(global with scope name) = %v, want the scope CHECK to refuse it", err)
	}
	web, err := repo.Create(ctx, domain.PromptSecret{Scope: "project", ScopeName: "web", Name: "GH_TOKEN"}, []byte("web"))
	if err != nil {
		t.Fatalf("Create(project): %v", err)
	}

	got, err := repo.GetEncrypted(ctx, gh.ID)
	if err != nil {
		t.Fatalf("GetEncrypted: %v", err)
	}
	if !bytes.Equal(got, cipher) {
		t.Fatalf("GetEncrypted = %x, want %x", got, cipher)
	}
	got, err = repo.GetEncryptedByName(ctx, "project", "web", "GH_TOKEN")
	if err != nil {
		t.Fatalf("GetEncryptedByName: %v", err)
	}
	if string(got) != "web" {
		t.Fatalf("GetEncryptedByName = %q, want the project secret's bytes", got)
	}

	byName, err := repo.GetByName(ctx, "global", "", "GH_TOKEN")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if byName.ID != gh.ID {
		t.Fatalf("GetByName = %s, want %s", byName.ID, gh.ID)
	}
	if byID, err := repo.GetByID(ctx, web.ID); err != nil || byID.ScopeName != "web" {
		t.Fatalf("GetByID = %+v, %v, want the project secret", byID, err)
	}

	updated, err := repo.Update(ctx, gh.ID, domain.PromptSecret{Scope: "global", Name: "GITHUB_TOKEN", Description: "renamed"}, []byte("new"))
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Name != "GITHUB_TOKEN" || !updated.UpdatedAt.After(gh.UpdatedAt) {
		t.Fatalf("Update = %+v, want the new name and a later updated_at", updated)
	}
	if got, err := repo.GetEncrypted(ctx, gh.ID); err != nil || string(got) != "new" {
		t.Fatalf("GetEncrypted after Update = %q, %v, want the new ciphertext", got, err)
	}
	if _, err := repo.Update(ctx, web.ID, domain.PromptSecret{Scope: "global", Name: "GITHUB_TOKEN"}, []byte("x")); !errors.Is(err, repository.ErrAlreadyExists) {
		t.Fatalf("Update into an existing name = %v, want ErrAlreadyExists", err)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].Scope != "global" || list[1].Scope != "project" {
		t.Fatalf("List = %+v, want global then project", list)
	}

	if err := repo.Delete(ctx, gh.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	unknown := uuid.New()
	for name, err := range map[string]error{
		"GetByID":            errOnly(repo.GetByID(ctx, unknown)),
		"GetByName":          errOnly(repo.GetByName(ctx, "global", "", "GH_TOKEN")),
		"GetEncrypted":       errOnly(repo.GetEncrypted(ctx, gh.ID)),
		"GetEncryptedByName": errOnly(repo.GetEncryptedByName(ctx, "global", "", "GITHUB_TOKEN")),
		"Update":             errOnly(repo.Update(ctx, unknown, domain.PromptSecret{Scope: "global", Name: "Z"}, nil)),
		"Delete":             repo.Delete(ctx, unknown),
	} {
		if !errors.Is(err, repository.ErrNotFound) {
			t.Errorf("%s on a missing secret = %v, want ErrNotFound", name, err)
		}
	}
}

func errOnly[T any](_ T, err error) error { return err }
