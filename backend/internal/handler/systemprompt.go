package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/systemprompts"
	"github.com/go-chi/chi/v5"
)

// SystemPromptsHandler exposes HTTP endpoints for reading system prompts and for
// editing the one prompt that is editable at runtime.
type SystemPromptsHandler struct {
	svc *systemprompts.Service
}

func NewSystemPromptsHandler(svc *systemprompts.Service) *SystemPromptsHandler {
	return &SystemPromptsHandler{svc: svc}
}

// promptUpdateRequest carries the new body. The name comes from the URL only:
// prompts are a fixed set shipped with the image, so there is nothing to rename.
type promptUpdateRequest struct {
	Content string `json:"content"`
}

func (h *SystemPromptsHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := h.svc.Get(chi.URLParam(r, "name"))
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, p)
	}
}

func (h *SystemPromptsHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req promptUpdateRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := h.svc.Set(chi.URLParam(r, "name"), req.Content); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// Reset drops the runtime override so the prompt baked into the image applies again.
func (h *SystemPromptsHandler) Reset() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := h.svc.Reset(chi.URLParam(r, "name")); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
