package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/crypto"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

// ErrSecretsEncryptionNotConfigured is returned when secret write operations
// are attempted without SECRETS_ENCRYPTION_KEY.
var ErrSecretsEncryptionNotConfigured = errors.New("secrets encryption not configured")

// SecretInput carries metadata and a plaintext value for create/update.
type SecretInput struct {
	Scope       string
	Name        string
	Description string
	Value       string
}

// SecretService implements business logic for encrypted prompt secrets.
type SecretService struct {
	repo   repository.SecretRepository
	cipher *crypto.Cipher

	mu         sync.RWMutex
	changeHook func()
}

func NewSecretService(repo repository.SecretRepository, cipher *crypto.Cipher) *SecretService {
	return &SecretService{repo: repo, cipher: cipher}
}

// SetChangeHook registers a callback invoked after a secret is created,
// updated, or deleted. mcp.json entries reference secrets by name through
// ${NAME}, so anything caching a resolved server has to be refreshed when a
// secret is rotated.
func (s *SecretService) SetChangeHook(hook func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.changeHook = hook
}

func (s *SecretService) notifyChange() {
	s.mu.RLock()
	hook := s.changeHook
	s.mu.RUnlock()
	if hook != nil {
		hook()
	}
}

func (s *SecretService) List(ctx context.Context) ([]domain.PromptSecret, error) {
	secrets, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	return secrets, nil
}

func (s *SecretService) Create(ctx context.Context, input SecretInput) (domain.PromptSecret, error) {
	if s.cipher == nil {
		return domain.PromptSecret{}, ErrSecretsEncryptionNotConfigured
	}

	meta := normalizeSecretMeta(domain.PromptSecret{
		Scope:       input.Scope,
		Name:        input.Name,
		Description: input.Description,
	})
	if err := validateSecretMeta(meta); err != nil {
		return domain.PromptSecret{}, err
	}
	if strings.TrimSpace(input.Value) == "" {
		return domain.PromptSecret{}, fmt.Errorf("%w: value is required", ErrInvalidVariableName)
	}

	encrypted, err := s.cipher.Encrypt(input.Value)
	if err != nil {
		return domain.PromptSecret{}, fmt.Errorf("encrypt secret: %w", err)
	}

	result, err := s.repo.Create(ctx, meta, encrypted)
	if err != nil {
		return domain.PromptSecret{}, fmt.Errorf("create secret: %w", err)
	}
	s.notifyChange()
	return result, nil
}

func (s *SecretService) Update(ctx context.Context, id uuid.UUID, input SecretInput) (domain.PromptSecret, error) {
	meta := normalizeSecretMeta(domain.PromptSecret{
		Scope:       input.Scope,
		Name:        input.Name,
		Description: input.Description,
	})
	if err := validateSecretMeta(meta); err != nil {
		return domain.PromptSecret{}, err
	}

	var encrypted []byte
	if strings.TrimSpace(input.Value) == "" {
		var err error
		encrypted, err = s.repo.GetEncrypted(ctx, id)
		if err != nil {
			return domain.PromptSecret{}, fmt.Errorf("get encrypted secret: %w", err)
		}
	} else {
		if s.cipher == nil {
			return domain.PromptSecret{}, ErrSecretsEncryptionNotConfigured
		}
		var err error
		encrypted, err = s.cipher.Encrypt(input.Value)
		if err != nil {
			return domain.PromptSecret{}, fmt.Errorf("encrypt secret: %w", err)
		}
	}

	result, err := s.repo.Update(ctx, id, meta, encrypted)
	if err != nil {
		return domain.PromptSecret{}, fmt.Errorf("update secret: %w", err)
	}
	s.notifyChange()
	return result, nil
}

func (s *SecretService) Delete(ctx context.Context, id uuid.UUID) error {
	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete secret: %w", err)
	}
	s.notifyChange()
	return nil
}

// GetValueByName decrypts a secret by scope and name. Values are never exposed to LLM tools.
func (s *SecretService) GetValueByName(ctx context.Context, scope, name string) (string, error) {
	if s.cipher == nil {
		return "", ErrSecretsEncryptionNotConfigured
	}
	scope = strings.TrimSpace(scope)
	if scope == "" {
		scope = defaultVariableScope
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("%w: secret name is required", ErrInvalidVariableName)
	}

	encrypted, err := s.repo.GetEncryptedByName(ctx, scope, name)
	if err != nil {
		return "", fmt.Errorf("get secret %q: %w", name, err)
	}
	plaintext, err := s.cipher.Decrypt(encrypted)
	if err != nil {
		return "", fmt.Errorf("decrypt secret %q: %w", name, err)
	}
	return plaintext, nil
}

// ExistsByName reports whether a secret with the given scope and name is stored.
// It does not need the cipher, so it works without SECRETS_ENCRYPTION_KEY.
func (s *SecretService) ExistsByName(ctx context.Context, scope, name string) (bool, error) {
	if _, err := s.getByName(ctx, scope, name); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// SetValueByName creates or updates a secret identified by scope and name.
// A blank value keeps the currently stored value, matching Update's semantics.
func (s *SecretService) SetValueByName(ctx context.Context, scope, name, description, value string) error {
	existing, err := s.getByName(ctx, scope, name)
	if err != nil && !errors.Is(err, repository.ErrNotFound) {
		return err
	}

	input := SecretInput{
		Scope:       scope,
		Name:        name,
		Description: description,
		Value:       value,
	}
	if errors.Is(err, repository.ErrNotFound) {
		if strings.TrimSpace(value) == "" {
			return nil
		}
		_, err = s.Create(ctx, input)
		return err
	}

	if strings.TrimSpace(description) == "" {
		input.Description = existing.Description
	}
	_, err = s.Update(ctx, existing.ID, input)
	return err
}

func (s *SecretService) getByName(ctx context.Context, scope, name string) (domain.PromptSecret, error) {
	meta := normalizeSecretMeta(domain.PromptSecret{Scope: scope, Name: name})
	if err := validateSecretMeta(meta); err != nil {
		return domain.PromptSecret{}, err
	}
	return s.repo.GetByName(ctx, meta.Scope, meta.Name)
}

func normalizeSecretMeta(s domain.PromptSecret) domain.PromptSecret {
	s.Scope = strings.TrimSpace(s.Scope)
	if s.Scope == "" {
		s.Scope = defaultVariableScope
	}
	s.Name = strings.TrimSpace(s.Name)
	return s
}

func validateSecretMeta(s domain.PromptSecret) error {
	if err := validateScope(s.Scope); err != nil {
		return err
	}
	return validateVariableName(s.Name)
}
