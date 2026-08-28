package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/rules"
	"github.com/go-chi/chi/v5"
)

// RulesHandler exposes HTTP endpoints for managing rule files.
type RulesHandler struct {
	svc *rules.Service
}

func NewRulesHandler(svc *rules.Service) *RulesHandler {
	return &RulesHandler{svc: svc}
}

type rulePayload struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func (h *RulesHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result, err := h.svc.List()
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func (h *RulesHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req rulePayload
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
		rule, err := h.svc.Get(req.Name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	}
}

func (h *RulesHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		rule, err := h.svc.Get(name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, rule)
	}
}

func (h *RulesHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		var req rulePayload
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

func (h *RulesHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if err := h.svc.Delete(name); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
