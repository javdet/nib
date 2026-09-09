package service

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/crypto"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

type fakeSecretRepo struct {
	secrets   map[uuid.UUID]domain.PromptSecret
	encrypted map[uuid.UUID][]byte
}

func newFakeSecretRepo() *fakeSecretRepo {
	return &fakeSecretRepo{
		secrets:   make(map[uuid.UUID]domain.PromptSecret),
		encrypted: make(map[uuid.UUID][]byte),
	}
}

func (f *fakeSecretRepo) List(ctx context.Context) ([]domain.PromptSecret, error) {
	out := make([]domain.PromptSecret, 0, len(f.secrets))
	for _, s := range f.secrets {
		out = append(out, s)
	}
	return out, nil
}

func (f *fakeSecretRepo) GetByID(ctx context.Context, id uuid.UUID) (domain.PromptSecret, error) {
	s, ok := f.secrets[id]
	if !ok {
		return domain.PromptSecret{}, repository.ErrNotFound
	}
	return s, nil
}

func (f *fakeSecretRepo) GetByName(ctx context.Context, scope, scopeName, name string) (domain.PromptSecret, error) {
	for _, s := range f.secrets {
		if s.Scope == scope && s.ScopeName == scopeName && s.Name == name {
			return s, nil
		}
	}
	return domain.PromptSecret{}, repository.ErrNotFound
}

func (f *fakeSecretRepo) GetEncrypted(ctx context.Context, id uuid.UUID) ([]byte, error) {
	ct, ok := f.encrypted[id]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return ct, nil
}

func (f *fakeSecretRepo) GetEncryptedByName(ctx context.Context, scope, scopeName, name string) ([]byte, error) {
	for id, s := range f.secrets {
		if s.Scope == scope && s.ScopeName == scopeName && s.Name == name {
			ct, ok := f.encrypted[id]
			if !ok {
				return nil, repository.ErrNotFound
			}
			return ct, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (f *fakeSecretRepo) Create(ctx context.Context, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error) {
	for _, existing := range f.secrets {
		if existing.Scope == s.Scope && existing.Name == s.Name {
			return domain.PromptSecret{}, repository.ErrAlreadyExists
		}
	}
	s.ID = uuid.New()
	f.secrets[s.ID] = s
	f.encrypted[s.ID] = append([]byte(nil), encrypted...)
	return s, nil
}

func (f *fakeSecretRepo) Update(ctx context.Context, id uuid.UUID, s domain.PromptSecret, encrypted []byte) (domain.PromptSecret, error) {
	if _, ok := f.secrets[id]; !ok {
		return domain.PromptSecret{}, repository.ErrNotFound
	}
	s.ID = id
	f.secrets[id] = s
	f.encrypted[id] = append([]byte(nil), encrypted...)
	return s, nil
}

func (f *fakeSecretRepo) Delete(ctx context.Context, id uuid.UUID) error {
	if _, ok := f.secrets[id]; !ok {
		return repository.ErrNotFound
	}
	delete(f.secrets, id)
	delete(f.encrypted, id)
	return nil
}

func testCipher(t *testing.T) *crypto.Cipher {
	t.Helper()
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}
	c, err := crypto.NewCipher(key)
	if err != nil {
		t.Fatalf("NewCipher() error = %v", err)
	}
	return c
}

func TestSecretServiceCreateWithoutCipher(t *testing.T) {
	svc := NewSecretService(newFakeSecretRepo(), nil)
	_, err := svc.Create(context.Background(), SecretInput{
		Scope: "global",
		Name:  "ApiToken",
		Value: "secret",
	})
	if !errors.Is(err, ErrSecretsEncryptionNotConfigured) {
		t.Fatalf("Create() error = %v, want %v", err, ErrSecretsEncryptionNotConfigured)
	}
}

func TestSecretServiceCreateRequiresValue(t *testing.T) {
	svc := NewSecretService(newFakeSecretRepo(), testCipher(t))
	_, err := svc.Create(context.Background(), SecretInput{
		Scope: "global",
		Name:  "ApiToken",
		Value: "   ",
	})
	if !errors.Is(err, ErrInvalidVariableName) {
		t.Fatalf("Create() error = %v, want %v", err, ErrInvalidVariableName)
	}
}

func TestSecretServiceCreateRequiresScopeNameForProject(t *testing.T) {
	svc := NewSecretService(newFakeSecretRepo(), testCipher(t))
	_, err := svc.Create(context.Background(), SecretInput{
		Scope: "project",
		Name:  "ApiToken",
		Value: "secret",
	})
	if !errors.Is(err, ErrInvalidVariableScope) {
		t.Fatalf("Create() error = %v, want %v", err, ErrInvalidVariableScope)
	}
}

func TestSecretServiceCreateWithProjectScope(t *testing.T) {
	svc := NewSecretService(newFakeSecretRepo(), testCipher(t))
	created, err := svc.Create(context.Background(), SecretInput{
		Scope:     "project",
		ScopeName: "myproject",
		Name:      "ApiToken",
		Value:     "secret",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if created.Scope != "project" || created.ScopeName != "myproject" {
		t.Fatalf("Create() = %+v, want project scope with scope name", created)
	}
}

func TestSecretServiceCreateAndUpdateKeepExistingValue(t *testing.T) {
	repo := newFakeSecretRepo()
	cipher := testCipher(t)
	svc := NewSecretService(repo, cipher)

	created, err := svc.Create(context.Background(), SecretInput{
		Scope:       "global",
		Name:        "ApiToken",
		Description: "token",
		Value:       "original-secret",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	originalCT := append([]byte(nil), repo.encrypted[created.ID]...)

	updated, err := svc.Update(context.Background(), created.ID, SecretInput{
		Scope:       "global",
		Name:        "ApiToken",
		Description: "updated description",
		Value:       "",
	})
	if err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if updated.Description != "updated description" {
		t.Fatalf("description = %q, want updated description", updated.Description)
	}
	if !bytesEqual(repo.encrypted[created.ID], originalCT) {
		t.Fatal("Update() with empty value changed ciphertext")
	}

	_, err = svc.Update(context.Background(), created.ID, SecretInput{
		Scope: "global",
		Name:  "ApiToken",
		Value: "new-secret",
	})
	if err != nil {
		t.Fatalf("Update() with new value error = %v", err)
	}
	if bytesEqual(repo.encrypted[created.ID], originalCT) {
		t.Fatal("Update() with new value did not change ciphertext")
	}
}

func TestSecretServiceGetValueByName(t *testing.T) {
	repo := newFakeSecretRepo()
	cipher := testCipher(t)
	svc := NewSecretService(repo, cipher)

	created, err := svc.Create(context.Background(), SecretInput{
		Scope: "global",
		Name:  "EXECUTOR_GIT_API_TOKEN",
		Value: "ghp_test_token",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	value, err := svc.GetValueByName(context.Background(), "global", "", "EXECUTOR_GIT_API_TOKEN")
	if err != nil {
		t.Fatalf("GetValueByName() error = %v", err)
	}
	if value != "ghp_test_token" {
		t.Fatalf("value = %q, want %q", value, "ghp_test_token")
	}

	_, err = svc.GetValueByName(context.Background(), "global", "", "missing")
	if !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetValueByName() missing error = %v, want %v", err, repository.ErrNotFound)
	}

	_ = created
}

func TestSecretServiceGetValueByNameWithoutCipher(t *testing.T) {
	svc := NewSecretService(newFakeSecretRepo(), nil)
	_, err := svc.GetValueByName(context.Background(), "global", "", "EXECUTOR_GIT_API_TOKEN")
	if !errors.Is(err, ErrSecretsEncryptionNotConfigured) {
		t.Fatalf("GetValueByName() error = %v, want %v", err, ErrSecretsEncryptionNotConfigured)
	}
}

func TestSecretServiceSetValueByName(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSecretRepo()
	svc := NewSecretService(repo, testCipher(t))
	const name = ExecutorKubernetesTokenSecretName

	exists, err := svc.ExistsByName(ctx, "", "", name)
	if err != nil {
		t.Fatalf("ExistsByName() error = %v", err)
	}
	if exists {
		t.Fatal("ExistsByName() = true before the secret was created")
	}

	// Empty scope defaults to global, matching GetValueByName.
	if err := svc.SetValueByName(ctx, "", "", name, "k8s token", "first-token"); err != nil {
		t.Fatalf("SetValueByName() create error = %v", err)
	}
	if exists, err = svc.ExistsByName(ctx, "", "", name); err != nil || !exists {
		t.Fatalf("ExistsByName() = %v, %v after create", exists, err)
	}

	if err := svc.SetValueByName(ctx, "global", "", name, "k8s token", "second-token"); err != nil {
		t.Fatalf("SetValueByName() update error = %v", err)
	}
	if len(repo.secrets) != 1 {
		t.Fatalf("secret count = %d, want 1 (update must not create a duplicate)", len(repo.secrets))
	}
	value, err := svc.GetValueByName(ctx, "global", "", name)
	if err != nil {
		t.Fatalf("GetValueByName() error = %v", err)
	}
	if value != "second-token" {
		t.Fatalf("value = %q, want second-token", value)
	}

	// A blank value keeps the stored token.
	if err := svc.SetValueByName(ctx, "global", "", name, "", ""); err != nil {
		t.Fatalf("SetValueByName() blank error = %v", err)
	}
	value, err = svc.GetValueByName(ctx, "global", "", name)
	if err != nil {
		t.Fatalf("GetValueByName() after blank error = %v", err)
	}
	if value != "second-token" {
		t.Fatalf("value after blank set = %q, want second-token", value)
	}
}

func TestSecretServiceSetValueByNameBlankOnMissingIsNoop(t *testing.T) {
	repo := newFakeSecretRepo()
	svc := NewSecretService(repo, testCipher(t))

	if err := svc.SetValueByName(context.Background(), "", "", "MISSING_TOKEN", "", ""); err != nil {
		t.Fatalf("SetValueByName() error = %v", err)
	}
	if len(repo.secrets) != 0 {
		t.Fatalf("secret count = %d, want 0", len(repo.secrets))
	}
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// A token pasted with the newline it was copied with must not be stored with
// it: substituted into an Authorization header, that byte makes net/http
// refuse the request outright.
func TestSecretServiceStripsTrailingNewlines(t *testing.T) {
	ctx := context.Background()
	repo := newFakeSecretRepo()
	svc := NewSecretService(repo, testCipher(t))

	created, err := svc.Create(ctx, SecretInput{
		Scope: "global",
		Name:  "GITHUB_TOKEN",
		Value: "ghp_test_token\r\n\n",
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	value, err := svc.GetValueByName(ctx, "global", "", "GITHUB_TOKEN")
	if err != nil {
		t.Fatalf("GetValueByName() error = %v", err)
	}
	if value != "ghp_test_token" {
		t.Fatalf("value = %q, want %q", value, "ghp_test_token")
	}

	if _, err := svc.Update(ctx, created.ID, SecretInput{
		Scope: "global",
		Name:  "GITHUB_TOKEN",
		Value: "ghp_rotated\n",
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	value, err = svc.GetValueByName(ctx, "global", "", "GITHUB_TOKEN")
	if err != nil {
		t.Fatalf("GetValueByName() after update error = %v", err)
	}
	if value != "ghp_rotated" {
		t.Fatalf("value after update = %q, want %q", value, "ghp_rotated")
	}
}

// Only the end is trimmed — a multi-line value keeps its interior.
func TestSecretServiceKeepsInteriorNewlines(t *testing.T) {
	ctx := context.Background()
	svc := NewSecretService(newFakeSecretRepo(), testCipher(t))

	if _, err := svc.Create(ctx, SecretInput{
		Scope: "global",
		Name:  "DEPLOY_KEY",
		Value: "-----BEGIN KEY-----\nabc\ndef\n-----END KEY-----\n",
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	value, err := svc.GetValueByName(ctx, "global", "", "DEPLOY_KEY")
	if err != nil {
		t.Fatalf("GetValueByName() error = %v", err)
	}
	if want := "-----BEGIN KEY-----\nabc\ndef\n-----END KEY-----"; value != want {
		t.Fatalf("value = %q, want %q", value, want)
	}
}
