package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/javdet/nib/internal/service"
	"github.com/google/uuid"
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

		if err := h.chatSvc.WriteActionPlan(id, req.Plan); err != nil {
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
		if err := json.Unmarshal(req.Plan, &planObj); err != nil {
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

// ExecutorChat returns or creates the shared execute chat for non-code actions
// of the plan stored on planDialogID.
func (h *DialogHandler) ExecutorChat() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		dialog, created, err := h.chatSvc.EnsureExecutorDialog(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"dialog":  dialog,
			"created": created,
		})
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
