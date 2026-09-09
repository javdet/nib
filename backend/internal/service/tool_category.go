package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"unicode"

	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/toolcatalog"
)

const toolCategoriesVariableName = "toolCategories"

// ToolCategoryService manages tool category patterns and syncs categories from the toolCategories variable.
type ToolCategoryService struct {
	repo  repository.VariableRepository
	store *toolcatalog.Store
}

// NewToolCategoryService wires variable repo and catalog store.
func NewToolCategoryService(repo repository.VariableRepository, store *toolcatalog.Store) *ToolCategoryService {
	return &ToolCategoryService{repo: repo, store: store}
}

// EnsureCategories syncs tool_categories from the global toolCategories variable.
func (s *ToolCategoryService) EnsureCategories(ctx context.Context) error {
	if s == nil || s.repo == nil || s.store == nil {
		return fmt.Errorf("tool category service not configured")
	}

	vars, err := s.repo.LoadAll(ctx, nil)
	if err != nil {
		return fmt.Errorf("load variables for tool categories: %w", err)
	}

	global := vars["global"]
	if global == nil {
		return fmt.Errorf("global variables scope missing")
	}

	raw, ok := global[toolCategoriesVariableName]
	if !ok {
		return fmt.Errorf("toolCategories variable not found")
	}

	items, ok := raw.([]string)
	if !ok {
		return fmt.Errorf("toolCategories must be a list variable")
	}

	keep := make([]string, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, name := range items {
		// Canonical here too, so the keep set below and the pruning compare the
		// same spelling the store writes.
		name = toolcatalog.CanonicalCategoryName(name)
		if name == "" {
			continue
		}
		if _, dup := seen[name]; dup {
			continue
		}
		seen[name] = struct{}{}
		keep = append(keep, name)
		if _, err := s.store.UpsertCategory(ctx, name, ""); err != nil {
			return fmt.Errorf("upsert category %q: %w", name, err)
		}
	}

	existing, err := s.store.ListCategoriesWithPatterns(ctx)
	if err != nil {
		return fmt.Errorf("list categories: %w", err)
	}
	keepSet := make(map[string]struct{}, len(keep))
	for _, name := range keep {
		keepSet[name] = struct{}{}
	}
	for _, cat := range existing {
		if _, ok := keepSet[cat.Name]; !ok {
			slog.Info("tool categories: pruning category not in toolCategories variable", "name", cat.Name)
		}
	}

	if err := s.store.DeleteCategoriesNotIn(ctx, keep); err != nil {
		return fmt.Errorf("delete stale categories: %w", err)
	}

	return s.store.RecomputeToolCategories(ctx)
}

// List returns all categories with patterns and tool counts.
func (s *ToolCategoryService) List(ctx context.Context) ([]toolcatalog.CategoryWithPatterns, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("tool category service not configured")
	}
	return s.store.ListCategoriesWithPatterns(ctx)
}

// SetPatterns replaces patterns for a category and recomputes tool assignments.
func (s *ToolCategoryService) SetPatterns(ctx context.Context, name string, patterns []string) (toolcatalog.CategoryWithPatterns, error) {
	if s == nil || s.store == nil {
		return toolcatalog.CategoryWithPatterns{}, fmt.Errorf("tool category service not configured")
	}

	// Canonical before the store call and before the lookup below: the store
	// writes the lowercased name, so comparing the caller's spelling to what
	// comes back would report "not found after update" for "Kubernetes".
	name = toolcatalog.CanonicalCategoryName(name)
	if name == "" {
		return toolcatalog.CategoryWithPatterns{}, fmt.Errorf("category name is required")
	}

	normalized, err := normalizePatterns(patterns)
	if err != nil {
		return toolcatalog.CategoryWithPatterns{}, err
	}

	if err := s.store.ReplaceCategoryPatterns(ctx, name, normalized); err != nil {
		return toolcatalog.CategoryWithPatterns{}, err
	}

	cats, err := s.store.ListCategoriesWithPatterns(ctx)
	if err != nil {
		return toolcatalog.CategoryWithPatterns{}, err
	}
	for _, cat := range cats {
		if cat.Name == name {
			return cat, nil
		}
	}
	return toolcatalog.CategoryWithPatterns{}, fmt.Errorf("category %q not found after update", name)
}

// ListToolsByCategory returns tools assigned to a category.
func (s *ToolCategoryService) ListToolsByCategory(ctx context.Context, name string) ([]toolcatalog.CatalogTool, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("tool category service not configured")
	}
	return s.store.ListToolsByCategory(ctx, name)
}

// ListUncategorizedTools returns tools with no category assignment.
func (s *ToolCategoryService) ListUncategorizedTools(ctx context.Context) ([]toolcatalog.CatalogTool, error) {
	if s == nil || s.store == nil {
		return nil, fmt.Errorf("tool category service not configured")
	}
	return s.store.ListUncategorizedTools(ctx)
}

func normalizePatterns(patterns []string) ([]string, error) {
	out := make([]string, 0, len(patterns))
	seen := make(map[string]struct{}, len(patterns))
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if strings.ContainsFunc(p, unicode.IsSpace) {
			return nil, fmt.Errorf("pattern %q must not contain whitespace", p)
		}
		starCount := strings.Count(p, "*")
		if starCount > 1 {
			return nil, fmt.Errorf("pattern %q must contain at most one wildcard", p)
		}
		if starCount == 1 && !strings.HasSuffix(p, "*") {
			return nil, fmt.Errorf("pattern %q wildcard must be a trailing *", p)
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	return out, nil
}
