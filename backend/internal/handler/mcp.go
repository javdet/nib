package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
)

// MCPHandler exposes HTTP endpoints for managing MCP connections and tools.
type MCPHandler struct {
	svc *service.MCPService
}

func NewMCPHandler(svc *service.MCPService) *MCPHandler {
	return &MCPHandler{svc: svc}
}

type createMCPConnectionRequest struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	ServerURL string          `json:"serverUrl"`
	APIToken  string          `json:"apiToken"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type updateMCPConnectionRequest struct {
	Name      string          `json:"name"`
	ServerURL string          `json:"serverUrl"`
	APIToken  string          `json:"apiToken,omitempty"`
	Metadata  json.RawMessage `json:"metadata,omitempty"`
}

type callToolRequest struct {
	Arguments map[string]any `json:"arguments"`
}

func (h *MCPHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conns, err := h.svc.ListConnections(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if conns == nil {
			conns = []domain.MCPConnection{}
		}
		writeJSON(w, http.StatusOK, conns)
	}
}

func (h *MCPHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createMCPConnectionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Type == "" {
			writeError(w, http.StatusBadRequest, "type is required")
			return
		}
		if req.ServerURL == "" {
			writeError(w, http.StatusBadRequest, "serverUrl is required")
			return
		}
		if req.Name == "" {
			req.Name = req.Type
		}

		conn, err := h.svc.CreateConnectionWithToken(r.Context(), req.Type, req.Name, req.ServerURL, req.APIToken, req.Metadata)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, conn)
	}
}

func (h *MCPHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid connection id")
			return
		}

		var req updateMCPConnectionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		if req.ServerURL == "" {
			writeError(w, http.StatusBadRequest, "serverUrl is required")
			return
		}

		conn, err := h.svc.UpdateConnection(r.Context(), id, req.Name, req.ServerURL, req.APIToken, req.Metadata)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, conn)
	}
}

func (h *MCPHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid connection id")
			return
		}
		if err := h.svc.DeleteConnection(r.Context(), id); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *MCPHandler) ListTools() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid connection id")
			return
		}

		tools, err := h.svc.ListTools(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tools)
	}
}

func (h *MCPHandler) CallTool() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid connection id")
			return
		}
		toolName := chi.URLParam(r, "toolName")
		if toolName == "" {
			writeError(w, http.StatusBadRequest, "tool name is required")
			return
		}

		var req callToolRequest
		if r.Body != nil && r.ContentLength > 0 && !decodeJSON(w, r, &req) {
			return
		}
		if req.Arguments == nil {
			req.Arguments = make(map[string]any)
		}

		result, err := h.svc.CallTool(r.Context(), id, toolName, req.Arguments)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}
