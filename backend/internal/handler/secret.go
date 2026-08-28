package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// SecretHandler exposes HTTP endpoints for managing encrypted prompt secrets.
type SecretHandler struct {
	svc *service.SecretService
}

func NewSecretHandler(svc *service.SecretService) *SecretHandler {
	return &SecretHandler{svc: svc}
}

type secretPayload struct {
	Scope       string `json:"scope"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Value       string `json:"value"`
}

func (h *SecretHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		secrets, err := h.svc.List(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if secrets == nil {
			secrets = []domain.PromptSecret{}
		}
		writeJSON(w, http.StatusOK, secrets)
	}
}

func (h *SecretHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req secretPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		s, err := h.svc.Create(r.Context(), service.SecretInput{
			Scope:       req.Scope,
			Name:        req.Name,
			Description: req.Description,
			Value:       req.Value,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, s)
	}
}

func (h *SecretHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret id")
			return
		}

		var req secretPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}

		s, err := h.svc.Update(r.Context(), id, service.SecretInput{
			Scope:       req.Scope,
			Name:        req.Name,
			Description: req.Description,
			Value:       req.Value,
		})
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s)
	}
}

func (h *SecretHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid secret id")
			return
		}
		if err := h.svc.Delete(r.Context(), id); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
