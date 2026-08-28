package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/includedtools"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/service"
	"github.com/javdet/nib/internal/toolcatalog"
	"github.com/go-chi/chi/v5"
)

// IncludedToolsHandler exposes HTTP endpoints for per-mode MCP tool included lists.
type IncludedToolsHandler struct {
	includedToolsSvc *includedtools.Service
	catalogStore     *toolcatalog.Store
	chatSvc          *service.ChatService
}

// NewIncludedToolsHandler creates an IncludedToolsHandler.
func NewIncludedToolsHandler(
	includedToolsSvc *includedtools.Service,
	catalogStore *toolcatalog.Store,
	chatSvc *service.ChatService,
) *IncludedToolsHandler {
	return &IncludedToolsHandler{
		includedToolsSvc: includedToolsSvc,
		catalogStore:     catalogStore,
		chatSvc:          chatSvc,
	}
}

type systemToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AlwaysOn    bool   `json:"alwaysOn,omitempty"`
}

type includedToolsResponse struct {
	Mode          string              `json:"mode"`
	SystemTools   []systemToolSummary `json:"systemTools"`
	IncludedTools []string            `json:"includedTools"`
}

type setIncludedToolsPayload struct {
	Tools []string `json:"tools"`
}

func (h *IncludedToolsHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modeName := chi.URLParam(r, "mode")
		if !mode.IsValid(modeName) {
			writeError(w, http.StatusBadRequest, "invalid mode")
			return
		}

		resp, err := h.buildIncludedToolsResponse(modeName)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *IncludedToolsHandler) Set() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		modeName := chi.URLParam(r, "mode")
		if !mode.IsValid(modeName) {
			writeError(w, http.StatusBadRequest, "invalid mode")
			return
		}

		var req setIncludedToolsPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := h.includedToolsSvc.Set(modeName, req.Tools); err != nil {
			handleServiceError(w, err)
			return
		}

		resp, err := h.buildIncludedToolsResponse(modeName)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *IncludedToolsHandler) ListCatalogTools() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if h.catalogStore == nil {
			writeJSON(w, http.StatusOK, []toolcatalog.CatalogTool{})
			return
		}
		tools, err := h.catalogStore.ListTools(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tools)
	}
}

func (h *IncludedToolsHandler) buildIncludedToolsResponse(modeName string) (includedToolsResponse, error) {
	systemDefs, err := h.chatSvc.SystemToolsForMode(modeName)
	if err != nil {
		return includedToolsResponse{}, err
	}

	systemTools := make([]systemToolSummary, 0, len(systemDefs))
	for _, def := range systemDefs {
		systemTools = append(systemTools, systemToolSummary{
			Name:        def.Name,
			Description: def.Description,
			AlwaysOn:    def.AlwaysOn,
		})
	}

	included, err := h.includedToolsSvc.Get(modeName)
	if err != nil {
		return includedToolsResponse{}, err
	}
	if included == nil {
		included = []string{}
	}

	return includedToolsResponse{
		Mode:          modeName,
		SystemTools:   systemTools,
		IncludedTools: included,
	}, nil
}
