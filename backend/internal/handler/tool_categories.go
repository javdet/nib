package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/service"
	"github.com/go-chi/chi/v5"
)

// ToolCategoriesHandler exposes HTTP endpoints for tool category patterns.
type ToolCategoriesHandler struct {
	svc *service.ToolCategoryService
}

// NewToolCategoriesHandler creates a ToolCategoriesHandler.
func NewToolCategoriesHandler(svc *service.ToolCategoryService) *ToolCategoriesHandler {
	return &ToolCategoriesHandler{svc: svc}
}

type setCategoryPatternsPayload struct {
	Patterns []string `json:"patterns"`
}

// List returns all categories with patterns and tool counts.
func (h *ToolCategoriesHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.svc == nil {
			writeJSON(w, http.StatusOK, []interface{}{})
			return
		}
		cats, err := h.svc.List(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cats)
	}
}

// SetPatterns replaces patterns for a category.
func (h *ToolCategoriesHandler) SetPatterns() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if name == "" {
			writeError(w, http.StatusBadRequest, "category name is required")
			return
		}

		var req setCategoryPatternsPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		cat, err := h.svc.SetPatterns(r.Context(), name, req.Patterns)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cat)
	}
}

// ListTools returns tools assigned to a category.
func (h *ToolCategoriesHandler) ListTools() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if name == "" {
			writeError(w, http.StatusBadRequest, "category name is required")
			return
		}

		tools, err := h.svc.ListToolsByCategory(r.Context(), name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tools)
	}
}

// ListUncategorizedTools returns tools with no category assignment.
func (h *ToolCategoriesHandler) ListUncategorizedTools() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tools, err := h.svc.ListUncategorizedTools(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tools)
	}
}
