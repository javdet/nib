package handler

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/javdet/nib/internal/mcpconfig"
)

// MCPConfigHandler exposes HTTP endpoints for managing mcp.json servers.
type MCPConfigHandler struct {
	svc *mcpconfig.Service
}

// NewMCPConfigHandler creates an MCPConfigHandler.
func NewMCPConfigHandler(svc *mcpconfig.Service) *MCPConfigHandler {
	return &MCPConfigHandler{svc: svc}
}

type mcpServerPayload struct {
	Name string `json:"name"`
	mcpconfig.ServerEntry
}

type mcpRawPayload struct {
	Content string `json:"content"`
}

type mcpRawResponse struct {
	Content  string   `json:"content"`
	Warnings []string `json:"warnings,omitempty"`
}

// mcpSaveResponse reports what saved fine but will not be acted on — a server
// entry written outside "mcpServers", say. Saving does not depend on it.
type mcpSaveResponse struct {
	Warnings []string `json:"warnings,omitempty"`
}

func (h *MCPConfigHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		servers, err := h.svc.ListServers()
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, servers)
	}
}

func (h *MCPConfigHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mcpServerPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if err := h.svc.AddServer(req.Name, req.ServerEntry); err != nil {
			handleServiceError(w, err)
			return
		}
		server, err := h.svc.GetServer(req.Name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, server)
	}
}

func (h *MCPConfigHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		server, err := h.svc.GetServer(name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, server)
	}
}

func (h *MCPConfigHandler) ListTools() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		tools, err := h.svc.ListServerTools(r.Context(), name)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tools)
	}
}

func (h *MCPConfigHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		var req mcpServerPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		newName := req.Name
		if newName == "" {
			newName = name
		}
		if err := h.svc.UpdateServer(name, newName, req.ServerEntry); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *MCPConfigHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := chi.URLParam(r, "name")
		if err := h.svc.DeleteServer(name); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *MCPConfigHandler) GetRaw() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		content, err := h.svc.GetRaw()
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpRawResponse{
			Content:  content,
			Warnings: mcpconfig.Warnings(content),
		})
	}
}

func (h *MCPConfigHandler) SetRaw() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mcpRawPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if err := h.svc.SetRaw(req.Content); err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, mcpSaveResponse{
			Warnings: mcpconfig.Warnings(req.Content),
		})
	}
}
