package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/service"
)

// Postmortem serves an incident's post-mortem. An incident nothing has been
// written for yet answers its empty document rather than 404, so the page
// renders the sections it is about to fill in.
func (h *DialogHandler) Postmortem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		doc, err := h.chatSvc.IncidentPostmortem(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	}
}

// EditPostmortem applies an operator's edit. The body is a patch, not the whole
// document: agents write it while the operator edits, and a whole-document PUT
// would undo whatever they wrote since the page last loaded.
func (h *DialogHandler) EditPostmortem() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var patch service.PostmortemPatch
		if !decodeJSON(w, r, &patch) {
			return
		}

		doc, err := h.chatSvc.EditIncidentPostmortem(r.Context(), id, patch)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, doc)
	}
}

type createIncidentPlanPayload struct {
	ActionIDs []string `json:"actionIds"`
}

// CreatePlanFromIncident turns the chosen corrective actions into a new plan.
// It does not start the plan's first turn: the answer carries the message, and
// the web interface sends it from the plan's chat.
func (h *DialogHandler) CreatePlanFromIncident() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req createIncidentPlanPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		plan, err := h.chatSvc.CreatePlanFromIncident(r.Context(), id, req.ActionIDs)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, plan)
	}
}

// PlanOrigin serves the incident a plan was created from, or 404 for a plan
// created any other way.
func (h *DialogHandler) PlanOrigin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		origin, found, err := h.chatSvc.PlanOriginForDialog(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "plan has no origin")
			return
		}
		writeJSON(w, http.StatusOK, origin)
	}
}

// Investigation serves an incident's latest run, or 404 before its first.
func (h *DialogHandler) Investigation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		run, found, err := h.chatSvc.IncidentRunForDialog(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "no investigation yet")
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// StopInvestigation cancels a running investigation or closure. What the
// agents already wrote into the post-mortem stays.
func (h *DialogHandler) StopInvestigation() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		if err := h.chatSvc.StopInvestigation(r.Context(), id); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

type closeIncidentPayload struct {
	ResolvedAt string `json:"resolvedAt"`
}

// CloseIncident starts the closure run and answers at once: the final impact
// and the closure summary take minutes and report over the activity stream.
func (h *DialogHandler) CloseIncident() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req closeIncidentPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		run, err := h.chatSvc.StartIncidentClosure(r.Context(), id, req.ResolvedAt)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusAccepted, run)
	}
}
