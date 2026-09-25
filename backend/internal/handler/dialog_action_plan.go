package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/service"
)

type actionPlanChecksPayload struct {
	Checked []string `json:"checked"`
}

type actionPlanCommentsPayload struct {
	Comments map[string]string `json:"comments"`
}

type actionPlanUpdatePayload struct {
	Plan json.RawMessage `json:"plan"`
}

type executeActionPayload struct {
	Key string `json:"key"`
}

type stageRunPayload struct {
	Scope string `json:"scope"`
	Stage int    `json:"stage"`
}

type actionPlanReorderPayload struct {
	Scope string `json:"scope"`
	Stage int    `json:"stage"`
	From  int    `json:"from"`
	To    int    `json:"to"`
}

func (h *DialogHandler) requireActionPlan(w http.ResponseWriter, id uuid.UUID) bool {
	_, found, err := h.chatSvc.ReadActionPlan(id)
	if err != nil {
		handleServiceError(w, err)
		return false
	}
	if !found {
		writeError(w, http.StatusNotFound, "action plan not found")
		return false
	}
	return true
}

func (h *DialogHandler) ActionPlan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		plan, found, err := h.chatSvc.ReadActionPlan(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "action plan not found")
			return
		}

		checked, err := h.chatSvc.ReadActionPlanChecks(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		comments, err := h.chatSvc.ReadActionPlanComments(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		var planObj any
		if err := json.Unmarshal(plan, &planObj); err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"plan":     planObj,
			"checked":  checked,
			"comments": comments,
		})
	}
}

func (h *DialogHandler) UpdateActionPlan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		if !h.requireActionPlan(w, id) {
			return
		}

		var req actionPlanUpdatePayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if len(req.Plan) == 0 {
			writeError(w, http.StatusBadRequest, "plan is required")
			return
		}
		if !json.Valid(req.Plan) {
			writeError(w, http.StatusBadRequest, "plan must be valid JSON")
			return
		}

		// The write stamps the derived item numbers, so the response echoes what
		// was stored rather than the payload that came in.
		stored, err := h.chatSvc.WriteActionPlan(id, req.Plan)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		// Editing the plan can add unchecked items to a finished plan, so the
		// status has to be re-evaluated against the new item set.
		checked, err := h.chatSvc.ReadActionPlanChecks(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		status, err := h.chatSvc.SyncPlanStatusForChecks(r.Context(), id, checked)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		var planObj any
		if err := json.Unmarshal(stored, &planObj); err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"plan":       planObj,
			"planStatus": status,
		})
	}
}

func (h *DialogHandler) SetActionPlanChecks() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		if !h.requireActionPlan(w, id) {
			return
		}

		var req actionPlanChecksPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := h.chatSvc.WriteActionPlanChecks(id, req.Checked); err != nil {
			handleServiceError(w, err)
			return
		}

		status, err := h.chatSvc.SyncPlanStatusForChecks(r.Context(), id, req.Checked)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"checked":    req.Checked,
			"planStatus": status,
		})
	}
}

func (h *DialogHandler) ReorderActionPlan() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		if !h.requireActionPlan(w, id) {
			return
		}

		var req actionPlanReorderPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		scope := service.ActionPlanScope(req.Scope)
		plan, checked, comments, err := h.chatSvc.ReorderActionPlanItems(
			id, scope, req.Stage, req.From, req.To,
		)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		status, err := h.chatSvc.SyncPlanStatusForChecks(r.Context(), id, checked)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		var planObj any
		if err := json.Unmarshal(plan, &planObj); err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"plan":       planObj,
			"checked":    checked,
			"comments":   comments,
			"planStatus": status,
		})
	}
}

// ExecuteActionPlanAction launches the coding agent for a single "code" action
// of the plan and returns the execute chat created for it.
func (h *DialogHandler) ExecuteActionPlanAction() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req executeActionPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if strings.TrimSpace(req.Key) == "" {
			writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		dialog, run, err := h.chatSvc.ExecuteCodeAction(r.Context(), id, req.Key)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"dialog": dialog,
			"run":    run,
		})
	}
}

// StartStageRun runs the unticked items of one stage, or of the rollback, one
// after another. It answers once the first item has started.
func (h *DialogHandler) StartStageRun() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		if !h.requireActionPlan(w, id) {
			return
		}

		var req stageRunPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		run, err := h.chatSvc.StartStageRun(r.Context(), id, service.StageRunScope(req.Scope), req.Stage)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// StageRun reports the plan's latest stage run, or null when it never had one.
func (h *DialogHandler) StageRun() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		run, err := h.chatSvc.ReadStageRun(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// StopStageRun stops the plan's stage run and the item it is on.
func (h *DialogHandler) StopStageRun() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		run, err := h.chatSvc.StopStageRun(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, run)
	}
}

// ActionPlanExecRuns reports the latest sub-agent run per action row, which is
// what puts a status beside each row in the plan view.
func (h *DialogHandler) ActionPlanExecRuns() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		runs, err := h.chatSvc.ReadActionPlanExecRuns(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, runs)
	}
}

// ActionPlanActionLogs returns what the agent-runner container of one running
// code action has written so far.
//
// A snapshot rather than a stream: an action runs for tens of minutes, and
// holding a response open for that long would tie it to the life of the
// container. The client polls instead.
func (h *DialogHandler) ActionPlanActionLogs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		key := strings.TrimSpace(r.URL.Query().Get("key"))
		if key == "" {
			writeError(w, http.StatusBadRequest, "key is required")
			return
		}

		logs, err := h.chatSvc.ReadActionContainerLogs(r.Context(), id, key)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, logs)
	}
}

func (h *DialogHandler) SetActionPlanComments() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		if !h.requireActionPlan(w, id) {
			return
		}

		var req actionPlanCommentsPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := h.chatSvc.WriteActionPlanComments(id, req.Comments); err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"comments": req.Comments,
		})
	}
}
