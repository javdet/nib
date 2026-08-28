package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// EnvironmentHandler exposes HTTP endpoints for environments
// scoped under a parent project.
type EnvironmentHandler struct {
	svc *service.EnvironmentService
}

func NewEnvironmentHandler(svc *service.EnvironmentService) *EnvironmentHandler {
	return &EnvironmentHandler{svc: svc}
}

func (h *EnvironmentHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}

		envs, err := h.svc.ListByProject(r.Context(), projectID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if envs == nil {
			envs = []domain.Environment{}
		}
		writeJSON(w, http.StatusOK, envs)
	}
}

func (h *EnvironmentHandler) GetByID() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}

		envID, err := uuid.Parse(chi.URLParam(r, "envID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid environment id")
			return
		}

		env, err := h.svc.GetByID(r.Context(), projectID, envID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, env)
	}
}

func (h *EnvironmentHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		var req nameDescRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		env, err := h.svc.Create(r.Context(), projectID, req.Name, req.Description)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, env)
	}
}

func (h *EnvironmentHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		envID, err := uuid.Parse(chi.URLParam(r, "envID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid environment id")
			return
		}
		var req nameDescRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		env, err := h.svc.Update(r.Context(), projectID, envID, req.Name, req.Description)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, env)
	}
}

func (h *EnvironmentHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		envID, err := uuid.Parse(chi.URLParam(r, "envID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid environment id")
			return
		}
		if err := h.svc.Delete(r.Context(), projectID, envID); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
