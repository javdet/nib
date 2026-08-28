package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/service"
)

// SystemToolsHandler serves the built-in agent tool catalog for developers.
type SystemToolsHandler struct {
	chatSvc *service.ChatService
}

// NewSystemToolsHandler creates a handler for system tool definitions.
func NewSystemToolsHandler(chatSvc *service.ChatService) *SystemToolsHandler {
	return &SystemToolsHandler{chatSvc: chatSvc}
}

// List returns all built-in tool definitions with mode availability.
func (h *SystemToolsHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tools, err := h.chatSvc.SystemToolDefs()
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, tools)
	}
}
