package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/service"
)

const dialogListLimit = 10

// DialogHandler exposes HTTP endpoints for persisted chat dialogs.
type DialogHandler struct {
	dialogSvc *service.DialogService
	chatSvc   *service.ChatService
}

func NewDialogHandler(dialogSvc *service.DialogService, chatSvc *service.ChatService) *DialogHandler {
	return &DialogHandler{dialogSvc: dialogSvc, chatSvc: chatSvc}
}

type createDialogPayload struct {
	Mode     string  `json:"mode"`
	Title    string  `json:"title"`
	ParentID *string `json:"parentId"`
}

type updateDialogTitlePayload struct {
	Title string `json:"title"`
}

type updateDialogCategoriesPayload struct {
	Categories []string `json:"categories"`
}

type updateDialogSummaryPayload struct {
	Summary string `json:"summary"`
}

type pinDialogPayload struct {
	Pinned bool `json:"pinned"`
}

type sendDialogMessagePayload struct {
	Message       string   `json:"message"`
	AttachmentIDs []string `json:"attachmentIds"`
}

type submitToolResultPayload struct {
	ToolCallID string   `json:"toolCallId"`
	Answers    []string `json:"answers"`
}

type dialogListItem struct {
	domain.Dialog
	PlanStatus *string `json:"planStatus,omitempty"`
}

func (h *DialogHandler) enrichDialogsWithPlanStatus(ctx context.Context, dialogs []domain.Dialog) ([]dialogListItem, error) {
	_ = ctx
	if len(dialogs) == 0 {
		return []dialogListItem{}, nil
	}

	planIDs := make([]uuid.UUID, 0, len(dialogs))
	for _, d := range dialogs {
		if !carriesPlan(d) {
			continue
		}
		planIDs = append(planIDs, d.ID)
	}

	states := h.chatSvc.ReadPlanStates(planIDs)

	items := make([]dialogListItem, len(dialogs))
	for i, d := range dialogs {
		item := dialogListItem{Dialog: d}
		if carriesPlan(d) {
			state := states[d.ID]
			statusStr := string(state.Status)
			item.PlanStatus = &statusStr
		}
		items[i] = item
	}
	return items, nil
}

// carriesPlan is the Go twin of postgres.planDialogPredicate and of the
// plan_dialogs view: a root dialog whose mode carries a plan. A whitelist, so a
// mode added later is not silently listed as a plan that has no plan.
func carriesPlan(d domain.Dialog) bool {
	if d.ParentID != nil {
		return false
	}
	return mode.CarriesPlan(d.Mode)
}

func (h *DialogHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		page := 1
		if p := r.URL.Query().Get("page"); p != "" {
			if v, err := strconv.Atoi(p); err == nil && v > 0 {
				page = v
			}
		}

		limit := dialogListLimit
		if l := r.URL.Query().Get("limit"); l != "" {
			if v, err := strconv.Atoi(l); err == nil && v > 0 {
				limit = v
			}
		}

		offset := (page - 1) * limit

		scopeAll := r.URL.Query().Get("scope") == "all"
		scopePinned := r.URL.Query().Get("scope") == "pinned"
		modeFilter := strings.TrimSpace(r.URL.Query().Get("mode"))
		searchQuery := strings.TrimSpace(r.URL.Query().Get("search"))

		var total int
		var dialogs []domain.Dialog
		var err error

		if searchQuery != "" {
			total, err = h.dialogSvc.CountSearch(r.Context(), modeFilter, searchQuery)
			if err != nil {
				handleServiceError(w, err)
				return
			}
			dialogs, err = h.dialogSvc.Search(r.Context(), modeFilter, searchQuery, limit, offset)
		} else if scopePinned {
			dialogs, err = h.dialogSvc.ListPinned(r.Context())
			if err != nil {
				handleServiceError(w, err)
				return
			}
			total = len(dialogs)
		} else if scopeAll {
			total, err = h.dialogSvc.CountAll(r.Context())
			if err != nil {
				handleServiceError(w, err)
				return
			}
			dialogs, err = h.dialogSvc.ListAll(r.Context(), limit, offset)
		} else if modeFilter != "" {
			total, err = h.dialogSvc.CountByMode(r.Context(), modeFilter)
			if err != nil {
				handleServiceError(w, err)
				return
			}
			dialogs, err = h.dialogSvc.ListByMode(r.Context(), modeFilter, limit, offset)
		} else {
			total, err = h.dialogSvc.Count(r.Context())
			if err != nil {
				handleServiceError(w, err)
				return
			}
			dialogs, err = h.dialogSvc.List(r.Context(), limit, offset)
		}
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if dialogs == nil {
			dialogs = []domain.Dialog{}
		}
		items, err := h.enrichDialogsWithPlanStatus(r.Context(), dialogs)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		w.Header().Set("X-Total-Count", strconv.Itoa(total))
		writeJSON(w, http.StatusOK, items)
	}
}

func (h *DialogHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createDialogPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		var parentID *uuid.UUID
		if req.ParentID != nil {
			id, err := uuid.Parse(strings.TrimSpace(*req.ParentID))
			if err != nil {
				writeError(w, http.StatusBadRequest, "invalid parentId")
				return
			}
			parentID = &id
		}

		d, err := h.dialogSvc.Create(r.Context(), req.Mode, req.Title, parentID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, d)
	}
}

func (h *DialogHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		d, err := h.dialogSvc.Get(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func (h *DialogHandler) UpdateTitle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req updateDialogTitlePayload
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := h.dialogSvc.UpdateTitle(r.Context(), id, req.Title); err != nil {
			handleServiceError(w, err)
			return
		}

		d, err := h.dialogSvc.Get(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func (h *DialogHandler) UpdateCategories() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req updateDialogCategoriesPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := h.dialogSvc.SetCategories(r.Context(), id, req.Categories); err != nil {
			handleServiceError(w, err)
			return
		}

		d, err := h.dialogSvc.Get(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func (h *DialogHandler) Pin() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req pinDialogPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		if err := h.dialogSvc.SetPinned(r.Context(), id, req.Pinned); err != nil {
			handleServiceError(w, err)
			return
		}

		d, err := h.dialogSvc.Get(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, d)
	}
}

func (h *DialogHandler) ListChildren() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		dialogs, err := h.dialogSvc.ListChildren(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if dialogs == nil {
			dialogs = []domain.Dialog{}
		}
		writeJSON(w, http.StatusOK, dialogs)
	}
}

func (h *DialogHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}
		if err := h.dialogSvc.Delete(r.Context(), id); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *DialogHandler) ListMessages() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		msgs, err := h.dialogSvc.GetMessages(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		msgs, err = h.chatSvc.EnrichMessagesWithAttachments(r.Context(), id, msgs)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if msgs == nil {
			msgs = []domain.DialogMessage{}
		}
		writeJSON(w, http.StatusOK, msgs)
	}
}

func (h *DialogHandler) SendMessage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req sendDialogMessagePayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Message == "" && len(req.AttachmentIDs) == 0 {
			writeError(w, http.StatusBadRequest, "message or attachmentIds is required")
			return
		}

		resp, err := h.chatSvc.SendInDialog(r.Context(), id, req.Message, req.AttachmentIDs)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *DialogHandler) SubmitToolResult() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req submitToolResultPayload
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.ToolCallID == "" {
			writeError(w, http.StatusBadRequest, "toolCallId is required")
			return
		}
		if len(req.Answers) == 0 {
			writeError(w, http.StatusBadRequest, "answers must not be empty")
			return
		}

		resp, err := h.chatSvc.SubmitToolResult(r.Context(), id, req.ToolCallID, req.Answers)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *DialogHandler) Retry() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		resp, err := h.chatSvc.RetryLastResponse(r.Context(), id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, resp)
	}
}

func (h *DialogHandler) DAG() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		content, found, err := h.chatSvc.ReadDAG(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "dag not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"content": content})
	}
}

func (h *DialogHandler) Summary() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		content, found, err := h.chatSvc.ReadSummary(id)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if !found {
			writeError(w, http.StatusNotFound, "summary not found")
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"content": content})
	}
}

func (h *DialogHandler) UpdateSummary() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, ok := parseUUIDParam(w, r, "id", "dialog id")
		if !ok {
			return
		}

		var req updateDialogSummaryPayload
		if !decodeJSON(w, r, &req) {
			return
		}

		summary := strings.TrimSpace(req.Summary)
		if summary == "" {
			writeError(w, http.StatusBadRequest, "summary is required")
			return
		}

		if err := h.chatSvc.WriteSummary(id, summary); err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"content": summary})
	}
}
