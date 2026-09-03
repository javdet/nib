package handler

import (
	"errors"
	"net/http"

	"github.com/javdet/nib/internal/service"
)

// ExecutionHandler exposes the one execution running at a time.
//
// The routes are global rather than nested under a dialog because the lease is
// global: a /dialogs/{id}/execution route would invite "stop my execution"
// semantics that the single-slot policy does not have.
type ExecutionHandler struct {
	chat *service.ChatService
}

func NewExecutionHandler(chat *service.ChatService) *ExecutionHandler {
	return &ExecutionHandler{chat: chat}
}

type stopExecutionResponse struct {
	Stopped bool                    `json:"stopped"`
	Lease   *service.ExecutionLease `json:"lease"`
}

// Get answers with the execution running right now, or null.
func (h *ExecutionHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lease, running := h.chat.ExecutionInProgress()
		if !running {
			writeJSON(w, http.StatusOK, nil)
			return
		}
		writeJSON(w, http.StatusOK, lease)
	}
}

// Stop force-stops the running execution.
//
// It reports the stop as done even when the container could not be reached: the
// lease is released either way, which is the whole point of the button, and the
// error is what the operator needs to know about the leftovers.
func (h *ExecutionHandler) Stop() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		lease, err := h.chat.CancelExecution(r.Context())
		if errors.Is(err, service.ErrNoExecutionRunning) {
			writeError(w, http.StatusConflict, "no execution is running")
			return
		}
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, stopExecutionResponse{Stopped: true, Lease: &lease})
	}
}
