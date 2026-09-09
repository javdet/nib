package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/javdet/nib/internal/service"
)

// SkillHandler exposes HTTP endpoints for managing skill files.
type SkillHandler struct {
	svc *service.SkillService
}

func NewSkillHandler(svc *service.SkillService) *SkillHandler {
	return &SkillHandler{svc: svc}
}

type skillPayload struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (h *SkillHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := h.svc.List()
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (h *SkillHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req skillPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := h.svc.Create(req.Name, req.Content); err != nil {
			handleServiceError(w, err)
			return
		}
		skill, err := h.svc.Get(req.Name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, skill)
	}
}

func (h *SkillHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		skill, err := h.svc.Get(name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skill)
	}
}

// GetRendered returns skill content with text/template actions resolved.
func (h *SkillHandler) GetRendered() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		skill, err := h.svc.GetRendered(r.Context(), chi.URLParam(r, "name"))
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, skill)
	}
}

func (h *SkillHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		var req skillPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		newName := req.Name
		if newName == "" {
			newName = name
		}
		if newName != name {
			if err := h.svc.Rename(name, newName, req.Content); err != nil {
				handleServiceError(w, err)
				return
			}
		} else if err := h.svc.Set(name, req.Content); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *SkillHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if err := h.svc.Delete(name); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
