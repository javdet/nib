package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/javdet/nib/internal/service"
)

// IncludedToolsHandler exposes HTTP endpoints for per-mode MCP tool included lists.
type IncludedToolsHandler struct {
	svc *service.IncludedToolsService
}

// NewIncludedToolsHandler creates an IncludedToolsHandler.
func NewIncludedToolsHandler(svc *service.IncludedToolsService) *IncludedToolsHandler {
	return &IncludedToolsHandler{svc: svc}
}

type setIncludedToolsPayload struct {
	Tools []string `json:"tools"`
}

func (h *IncludedToolsHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		resp, err := h.svc.ForMode(chi.URLParam(r, "mode"))
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *IncludedToolsHandler) Set() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req setIncludedToolsPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		resp, err := h.svc.SetForMode(chi.URLParam(r, "mode"), req.Tools)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *IncludedToolsHandler) ListCatalogTools() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tools, err := h.svc.ListCatalogTools(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tools)
	}
}
