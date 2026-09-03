package handler

import (
	"errors"
	"net/http"

	"github.com/javdet/nib/internal/service"
)

type startPlanFanoutPayload struct {
	// Stages, when set, restricts the run to those DAG stages. Empty plans all.
	Stages []string `json:"stages"`
	// Rollback runs the agent that writes the plan's rollback list. Unset means
	// "as usual": with the stages, and on its own when only stages were named.
	Rollback *bool `json:"rollback"`
}

// targets defers to the service so this route and the orchestrator's plan
// sub-agent cannot drift on what an empty stage list means.
func (p startPlanFanoutPayload) targets() service.FanoutTargets {
	return service.PlanFanoutTargetsFor(p.Stages, p.Rollback)
}

// StartPlanFanout kicks off a plan fan-out and answers 202 immediately: the run
// outlives this request and reports over the dialog's SSE stream.
func (h *DialogHandler) StartPlanFanout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req startPlanFanoutPayload
		if r.ContentLength > 0 && !decodeJSON(w, r, &req) {
			return
		}

		run, err := h.chatSvc.StartPlanFanout(r.Context(), id, req.targets())
		switch {
		case errors.Is(err, service.ErrFanoutInProgress):
			writeError(w, http.StatusConflict, err.Error())
			return
		case errors.Is(err, service.ErrNoDAGStages):
			writeError(w, http.StatusBadRequest, err.Error())
			return
		case err != nil:
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusAccepted, run)
	}
}

// PlanFanout returns the current run so a reloaded page can re-attach to a fan-out
// that is still going.
func (h *DialogHandler) PlanFanout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		run, found, err := h.chatSvc.ReadFanoutRun(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !found {
			writeJSON(w, http.StatusOK, nil)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// CancelPlanFanout stops a running fan-out. Stages already written stay on the plan.
func (h *DialogHandler) CancelPlanFanout() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		if !h.chatSvc.CancelPlanFanout(id) {
			writeError(w, http.StatusConflict, "no plan fan-out is running for this dialog")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
