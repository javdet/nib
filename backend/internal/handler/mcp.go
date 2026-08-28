package handler

import (
	"encoding/json"
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// MCPHandler exposes HTTP endpoints for managing MCP connections and tools.
type MCPHandler struct {
	svc             *service.MCPService
	frontendBaseURL string
}

func NewMCPHandler(svc *service.MCPService, frontendBaseURL string) *MCPHandler {
	return &MCPHandler{svc: svc, frontendBaseURL: frontendBaseURL}
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

// InitiateOAuth redirects the user to the OAuth provider's consent page.
func (h *MCPHandler) InitiateOAuth() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerType := chi.URLParam(r, "type")
		if providerType == "" {
			writeError(w, http.StatusBadRequest, "provider type is required")
			return
		}

		authURL, err := h.svc.InitiateOAuth(providerType)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}

		http.Redirect(w, r, authURL, http.StatusTemporaryRedirect)
	}
}

// OAuthCallback handles the OAuth redirect callback from the provider.
func (h *MCPHandler) OAuthCallback() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		providerType := chi.URLParam(r, "type")
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")

		if code == "" || state == "" {
			oauthErr := r.URL.Query().Get("error")
			desc := r.URL.Query().Get("error_description")
			if oauthErr != "" {
				writeError(w, http.StatusBadRequest, oauthErr+": "+desc)
				return
			}
			writeError(w, http.StatusBadRequest, "missing code or state parameter")
			return
		}

		_, err := h.svc.HandleOAuthCallback(r.Context(), providerType, state, code)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}

		redirectURL := h.frontendBaseURL + "/tools?oauth=success"
		http.Redirect(w, r, redirectURL, http.StatusTemporaryRedirect)
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
		if r.Body != nil && r.ContentLength > 0 {
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid request body")
				return
			}
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
