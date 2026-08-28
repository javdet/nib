package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/version"
)

func VersionInfo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": version.Version()})
	}
}
