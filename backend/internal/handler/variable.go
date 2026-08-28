package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// VariableHandler exposes HTTP endpoints for managing prompt template variables.
type VariableHandler struct {
	svc *service.VariableService
}

func NewVariableHandler(svc *service.VariableService) *VariableHandler {
	return &VariableHandler{svc: svc}
}

type variablePayload struct {
	Scope       string `json:"scope"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`
	Kind        string `json:"kind"`
}

func (h *VariableHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		vars, err := h.svc.List(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if vars == nil {
			vars = []domain.PromptVariable{}
		}
		writeJSON(w, http.StatusOK, vars)
	}
}

func (h *VariableHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req variablePayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		v, err := h.svc.Create(r.Context(), domain.PromptVariable{
			Scope:       req.Scope,
			Name:        req.Name,
			Description: req.Description,
			Value:       req.Value,
			Kind:        req.Kind,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, v)
	}
}

func (h *VariableHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid variable id")
			return
		}

		v, err := h.svc.Get(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (h *VariableHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid variable id")
			return
		}

		var req variablePayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		v, err := h.svc.Update(r.Context(), id, domain.PromptVariable{
			Scope:       req.Scope,
			Name:        req.Name,
			Description: req.Description,
			Value:       req.Value,
			Kind:        req.Kind,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, v)
	}
}

func (h *VariableHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid variable id")
			return
		}
		if err := h.svc.Delete(r.Context(), id); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
