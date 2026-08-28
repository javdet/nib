package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/mode"
)

// ListModes returns predefined chat modes for the frontend mode selector.
func ListModes() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, mode.Modes)
	}
}
