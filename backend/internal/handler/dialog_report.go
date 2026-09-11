package handler

import (
	"net/http"
)

// Report serves a finished plan's closing report. Like the summary it is an
// optional artifact, so a plan that has none answers 404 and the web interface
// reads that as "no report card".
func (h *DialogHandler) Report() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		content, found, err := h.chatSvc.ReadReport(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "report not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"content": content})
	}
}

// StartReport launches the subagent that writes the report and answers at once:
// the run takes minutes and reports itself over the dialog's activity stream.
func (h *DialogHandler) StartReport() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		if err := h.chatSvc.StartReportAgent(r.Context(), id); err != nil {
			handleServiceError(w, err)
			return
		}

		// A body rather than a bare 202: the frontend's fetch wrapper only
		// special-cases 204, so an empty 202 would reject on res.json().
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "started"})
	}
}
