package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

type memoryVariableRepo struct {
	vars map[string]map[string]string
}

func newMemoryVariableRepo() *memoryVariableRepo {
	return &memoryVariableRepo{vars: map[string]map[string]string{}}
}

func (r *memoryVariableRepo) List(context.Context) ([]domain.PromptVariable, error) {
	return nil, nil
}
func (r *memoryVariableRepo) GetByID(context.Context, uuid.UUID) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (r *memoryVariableRepo) Create(context.Context, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (r *memoryVariableRepo) Update(context.Context, uuid.UUID, domain.PromptVariable) (domain.PromptVariable, error) {
	return domain.PromptVariable{}, nil
}
func (r *memoryVariableRepo) Delete(context.Context, uuid.UUID) error { return nil }
func (r *memoryVariableRepo) EnsureExists(context.Context, domain.PromptVariable) error {
	return nil
}
func (r *memoryVariableRepo) EnsureBuiltin(ctx context.Context, v domain.PromptVariable) error {
	_, err := r.Upsert(ctx, v)
	return err
}
func (r *memoryVariableRepo) Upsert(_ context.Context, v domain.PromptVariable) (domain.PromptVariable, error) {
	if r.vars[v.Scope] == nil {
		r.vars[v.Scope] = map[string]string{}
	}
	r.vars[v.Scope][v.Name] = v.Value
	return v, nil
}
func (r *memoryVariableRepo) LoadAll(context.Context, map[string]string) (map[string]map[string]any, error) {
	out := make(map[string]map[string]any, len(r.vars))
	for scope, names := range r.vars {
		scopeVars := make(map[string]any, len(names))
		for name, value := range names {
			scopeVars[name] = value
		}
		out[scope] = scopeVars
	}
	return out, nil
}

func TestCompanyServiceEnsureDefaultsIssueProject(t *testing.T) {
	t.Parallel()

	repo := newMemoryVariableRepo()
	svc := NewCompanyService(repo)
	ctx := context.Background()

	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}

	got, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.IssueProject != "DEVOPS" {
		t.Fatalf("Get() IssueProject = %q, want %q", got.IssueProject, "DEVOPS")
	}
}

func TestCompanyServiceSaveAndGetGitIdentity(t *testing.T) {
	t.Parallel()

	repo := newMemoryVariableRepo()
	svc := NewCompanyService(repo)
	ctx := context.Background()

	if err := svc.EnsureDefaults(ctx); err != nil {
		t.Fatalf("EnsureDefaults() error = %v", err)
	}

	want := CompanyInfo{
		CompanyName:          "Acme",
		VersionControlSystem: "GitHub",
		GitBaseURL:           "https://github.com/acme",
		GitUsername:          "agent-runner",
		GitEmail:             "agent-runner@localhost",
		IssueProject:         "DEVOPS",
	}

	saved, err := svc.Save(ctx, want)
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.GitUsername != want.GitUsername {
		t.Fatalf("Save() GitUsername = %q, want %q", saved.GitUsername, want.GitUsername)
	}
	if saved.GitEmail != want.GitEmail {
		t.Fatalf("Save() GitEmail = %q, want %q", saved.GitEmail, want.GitEmail)
	}

	got, err := svc.Get(ctx)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.GitUsername != want.GitUsername {
		t.Fatalf("Get() GitUsername = %q, want %q", got.GitUsername, want.GitUsername)
	}
	if got.GitEmail != want.GitEmail {
		t.Fatalf("Get() GitEmail = %q, want %q", got.GitEmail, want.GitEmail)
	}
	if got.IssueProject != want.IssueProject {
		t.Fatalf("Get() IssueProject = %q, want %q", got.IssueProject, want.IssueProject)
	}
}

func TestCompanyServiceSaveInvalidGitEmail(t *testing.T) {
	t.Parallel()

	svc := NewCompanyService(newMemoryVariableRepo())

	_, err := svc.Save(context.Background(), CompanyInfo{
		GitEmail: "not-an-email",
	})
	if err == nil {
		t.Fatal("Save() error = nil, want ErrInvalidGitEmail")
	}
	if !errors.Is(err, ErrInvalidGitEmail) {
		t.Fatalf("Save() error = %v, want ErrInvalidGitEmail", err)
	}
}

func TestCompanyServiceSaveTrimsGitIdentity(t *testing.T) {
	t.Parallel()

	repo := newMemoryVariableRepo()
	svc := NewCompanyService(repo)

	saved, err := svc.Save(context.Background(), CompanyInfo{
		GitUsername: "  agent-runner  ",
		GitEmail:    "  agent-runner@localhost  ",
	})
	if err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if saved.GitUsername != "agent-runner" {
		t.Fatalf("Save() GitUsername = %q, want trimmed value", saved.GitUsername)
	}
	if saved.GitEmail != "agent-runner@localhost" {
		t.Fatalf("Save() GitEmail = %q, want trimmed value", saved.GitEmail)
	}
}
