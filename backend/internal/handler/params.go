package handler

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// parseUUIDParam parses a URL path parameter as a UUID.
// On failure it writes a 400 response and returns false.
func parseUUIDParam(w http.ResponseWriter, r *http.Request, param, label string) (uuid.UUID, bool) {
	raw := chi.URLParam(r, param)
	id, err := uuid.Parse(raw)
	if err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", label))
		return uuid.Nil, false
	}
	return id, true
}
