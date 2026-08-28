package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
)

// SelectionHandler exposes HTTP endpoints for the global UI selection.
type SelectionHandler struct {
	store *service.SelectionStore
}

func NewSelectionHandler(store *service.SelectionStore) *SelectionHandler {
	return &SelectionHandler{store: store}
}

func (h *SelectionHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, h.store.Get())
	}
}

func (h *SelectionHandler) Set() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req domain.Selection
		if !decodeJSON(w, r, &req) {
			return
		}
		writeJSON(w, http.StatusOK, h.store.Set(req))
	}
}
