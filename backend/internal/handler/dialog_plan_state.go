package handler

import (
	"net/http"
	"strings"

	"github.com/javdet/nib/internal/service"
)

type planSchedulePayload struct {
	ScheduledAt int64 `json:"scheduledAt"`
}

type planStatusPayload struct {
	Status string `json:"status"`
}

func (h *DialogHandler) PlanState() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		state, err := h.chatSvc.ReadPlanState(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status":      state.Status,
			"scheduledAt": state.ScheduledAt,
		})
	}
}

func (h *DialogHandler) SetPlanSchedule() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req planSchedulePayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.ScheduledAt < 0 {
			writeError(w, http.StatusBadRequest, "scheduledAt must be >= 0")
			return
		}

		state, err := h.chatSvc.ReadPlanState(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		state.ScheduledAt = req.ScheduledAt
		if req.ScheduledAt > 0 {
			if state.Status == service.ActionPlanStatusDraft {
				state.Status = service.ActionPlanStatusScheduled
			}
		} else if state.Status == service.ActionPlanStatusScheduled {
			state.Status = service.ActionPlanStatusDraft
		}

		if err := h.chatSvc.WritePlanState(id, state); err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"scheduledAt": state.ScheduledAt,
			"status":      state.Status,
		})
	}
}

func (h *DialogHandler) SetPlanStatus() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req planStatusPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Status == "" {
			writeError(w, http.StatusBadRequest, "status is required")
			return
		}

		status := service.ActionPlanStatus(req.Status)
		state, err := h.chatSvc.ReadPlanState(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		state.Status = status
		if err := h.chatSvc.WritePlanState(id, state); err != nil {
			if strings.Contains(err.Error(), "invalid action plan status") {
				writeError(w, http.StatusBadRequest, err.Error())
				return
			}
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"status": state.Status,
		})
	}
}
