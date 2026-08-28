package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
)

// ChatHandler exposes HTTP endpoints for LLM chat.
type ChatHandler struct {
	svc *service.ChatService
}

func NewChatHandler(svc *service.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc}
}

func (h *ChatHandler) Send() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req domain.ChatRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Message == "" {
			writeError(w, http.StatusBadRequest, "message is required")
			return
		}
		resp, err := h.svc.Send(r.Context(), req.Message, req.Mode)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}
