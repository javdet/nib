package handler

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/javdet/nib/internal/service"
)

// CommandRunHandler serves the commands agents ran for a plan.
type CommandRunHandler struct {
	svc *service.CommandRunService
}

func NewCommandRunHandler(svc *service.CommandRunService) *CommandRunHandler {
	return &CommandRunHandler{svc: svc}
}

// List returns the plan's command runs, newest first; ?key= narrows them to
// one action row.
func (h *CommandRunHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		runs, err := h.svc.List(r.Context(), id, r.URL.Query().Get("key"))
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runs": runs})
	}
}

// Output returns a page of one run's output. A snapshot the client polls,
// like the container logs: a command can run for an hour.
func (h *CommandRunHandler) Output() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		runID, ok := parseUUIDParam(w, r, "runID", "command run id")
		if !ok {
			return
		}
		offset, ok := parseInt64Query(w, r, "offset")
		if !ok {
			return
		}
		limit, ok := parseInt64Query(w, r, "limit")
		if !ok {
			return
		}
		out, err := h.svc.Output(r.Context(), id, runID, offset, limit)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// Cancel stops a running command.
func (h *CommandRunHandler) Cancel() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		runID, ok := parseUUIDParam(w, r, "runID", "command run id")
		if !ok {
			return
		}
		if err := h.svc.Cancel(r.Context(), id, runID); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// parseInt64Query reads an optional non-negative integer query parameter.
func parseInt64Query(w http.ResponseWriter, r *http.Request, name string) (int64, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get(name))
	if raw == "" {
		return 0, true
	}
	v, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || v < 0 {
		writeError(w, http.StatusBadRequest, name+" must be a non-negative integer")
		return 0, false
	}
	return v, true
}
