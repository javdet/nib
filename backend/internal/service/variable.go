package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

const defaultVariableScope = "global"

const (
	VariableScopeGlobal      = "global"
	VariableScopeProject     = "project"
	VariableScopeEnvironment = "environment"
	VariableScopeCloud       = "cloud"
	VariableScopeLocation    = "location"
)

const (
	VariableKindString = "string"
	VariableKindList   = "list"
)

var validVariableIdent = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ErrInvalidVariableName is returned when a variable name fails validation.
var ErrInvalidVariableName = errors.New("invalid variable name")

// ErrInvalidVariableScope is returned when a variable scope fails validation.
var ErrInvalidVariableScope = errors.New("invalid variable scope")

// ErrInvalidVariableValue is returned when a variable value fails validation.
var ErrInvalidVariableValue = errors.New("invalid variable value")

// ErrVariableProtected is returned when a protected variable cannot be deleted or renamed.
var ErrVariableProtected = errors.New("variable is protected")

var defaultToolCategories = []string{
	"issue-tracker",
	"knowledge-base",
	"cloud",
	"platform",
	"code-repository",
	"monitoring",
	"logging",
	"observability",
	"kubernetes",
	"search",
}

// VariableService implements business logic for prompt template variables.
type VariableService struct {
	repo repository.VariableRepository
}

func NewVariableService(repo repository.VariableRepository) *VariableService {
	return &VariableService{repo: repo}
}

func (s *VariableService) List(ctx context.Context) ([]domain.PromptVariable, error) {
	vars, err := s.repo.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("list variables: %w", err)
	}
	return vars, nil
}

func (s *VariableService) Get(ctx context.Context, id uuid.UUID) (domain.PromptVariable, error) {
	v, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.PromptVariable{}, fmt.Errorf("get variable: %w", err)
	}
	return v, nil
}

func (s *VariableService) Create(ctx context.Context, v domain.PromptVariable) (domain.PromptVariable, error) {
	v = normalizeVariable(v)
	if err := validateVariable(v); err != nil {
		return domain.PromptVariable{}, err
	}
	v.Deletable = true

	result, err := s.repo.Create(ctx, v)
	if err != nil {
		return domain.PromptVariable{}, fmt.Errorf("create variable: %w", err)
	}
	return result, nil
}

func (s *VariableService) Update(ctx context.Context, id uuid.UUID, v domain.PromptVariable) (domain.PromptVariable, error) {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return domain.PromptVariable{}, fmt.Errorf("get variable: %w", err)
	}

	v = normalizeVariable(v)
	v.Kind = existing.Kind
	v.Deletable = existing.Deletable

	if !existing.Deletable {
		if v.Scope != existing.Scope || v.ScopeName != existing.ScopeName || v.Name != existing.Name {
			return domain.PromptVariable{}, fmt.Errorf("%w: scope and name cannot be changed", ErrVariableProtected)
		}
	}

	if err := validateVariable(v); err != nil {
		return domain.PromptVariable{}, err
	}

	result, err := s.repo.Update(ctx, id, v)
	if err != nil {
		return domain.PromptVariable{}, fmt.Errorf("update variable: %w", err)
	}
	return result, nil
}

func (s *VariableService) Delete(ctx context.Context, id uuid.UUID) error {
	existing, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get variable: %w", err)
	}
	if !existing.Deletable {
		return fmt.Errorf("%w: built-in variables cannot be deleted", ErrVariableProtected)
	}

	if err := s.repo.Delete(ctx, id); err != nil {
		return fmt.Errorf("delete variable: %w", err)
	}
	return nil
}

// EnsureDefaults seeds built-in list variables when missing.
func (s *VariableService) EnsureDefaults(ctx context.Context) error {
	value, err := json.Marshal(defaultToolCategories)
	if err != nil {
		return fmt.Errorf("marshal tool categories: %w", err)
	}

	if err := s.repo.EnsureBuiltin(ctx, domain.PromptVariable{
		Scope:       defaultVariableScope,
		Name:        "toolCategories",
		Description: "MCP tool categories for search and filtering",
		Value:       string(value),
		Kind:        VariableKindList,
	}); err != nil {
		return fmt.Errorf("ensure toolCategories: %w", err)
	}
	return nil
}

// LoadAll returns variables grouped by scope for template rendering.
// scopeNames picks which entity's copy wins each non-global scope; see
// repository.VariableRepository.LoadAll.
func (s *VariableService) LoadAll(ctx context.Context, scopeNames map[string]string) (map[string]map[string]any, error) {
	vars, err := s.repo.LoadAll(ctx, scopeNames)
	if err != nil {
		return nil, fmt.Errorf("load variables: %w", err)
	}
	return vars, nil
}

func normalizeVariable(v domain.PromptVariable) domain.PromptVariable {
	v.Scope = strings.TrimSpace(v.Scope)
	if v.Scope == "" {
		v.Scope = defaultVariableScope
	}
	v.ScopeName = strings.TrimSpace(v.ScopeName)
	if v.Scope == defaultVariableScope {
		v.ScopeName = ""
	}
	v.Name = strings.TrimSpace(v.Name)
	v.Kind = strings.TrimSpace(v.Kind)
	if v.Kind == "" {
		v.Kind = VariableKindString
	}
	return v
}

func validateVariable(v domain.PromptVariable) error {
	if err := validateScope(v.Scope); err != nil {
		return err
	}
	if err := validateScopeName(v.Scope, v.ScopeName); err != nil {
		return err
	}
	if err := validateVariableName(v.Name); err != nil {
		return err
	}
	if err := validateVariableKind(v.Kind); err != nil {
		return err
	}
	return validateVariableValue(v.Kind, v.Value)
}

func validateVariableKind(kind string) error {
	switch kind {
	case VariableKindString, VariableKindList:
		return nil
	default:
		return fmt.Errorf("%w: kind must be string or list", ErrInvalidVariableValue)
	}
}

func validateVariableValue(kind, value string) error {
	if kind != VariableKindList {
		return nil
	}
	var items []string
	if err := json.Unmarshal([]byte(value), &items); err != nil {
		return fmt.Errorf("%w: list value must be a JSON array of strings", ErrInvalidVariableValue)
	}
	return nil
}

func validateScope(scope string) error {
	switch scope {
	case VariableScopeGlobal, VariableScopeProject, VariableScopeEnvironment,
		VariableScopeCloud, VariableScopeLocation:
		return nil
	default:
		return fmt.Errorf("%w: must be global, project, environment, cloud, or location", ErrInvalidVariableScope)
	}
}

func validateScopeName(scope, scopeName string) error {
	if scope == defaultVariableScope {
		return nil
	}
	if scopeName == "" {
		return fmt.Errorf("%w: scope name is required for non-global scope", ErrInvalidVariableScope)
	}
	return nil
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func validateVariableName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidVariableName)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: reserved name", ErrInvalidVariableName)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: path separators are not allowed", ErrInvalidVariableName)
	}
	if strings.Contains(name, ".") {
		return fmt.Errorf("%w: dots are not allowed", ErrInvalidVariableName)
	}
	if !validVariableIdent.MatchString(name) {
		return fmt.Errorf("%w: must match [a-zA-Z0-9][a-zA-Z0-9_-]*", ErrInvalidVariableName)
	}
	return nil
}
