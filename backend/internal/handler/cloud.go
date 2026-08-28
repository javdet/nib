package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// CloudHandler exposes HTTP endpoints for clouds
// scoped under a parent project.
type CloudHandler struct {
	svc *service.CloudService
}

func NewCloudHandler(svc *service.CloudService) *CloudHandler {
	return &CloudHandler{svc: svc}
}

func (h *CloudHandler) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}

		clouds, err := h.svc.ListByProject(r.Context(), projectID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if clouds == nil {
			clouds = []domain.Cloud{}
		}
		writeJSON(w, http.StatusOK, clouds)
	}
}

func (h *CloudHandler) GetByID() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}

		cloudID, err := uuid.Parse(chi.URLParam(r, "cloudID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cloud id")
			return
		}

		cloud, err := h.svc.GetByID(r.Context(), projectID, cloudID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cloud)
	}
}

func (h *CloudHandler) Create() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		var req nameDescRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		cloud, err := h.svc.Create(r.Context(), projectID, req.Name, req.Description)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, cloud)
	}
}

func (h *CloudHandler) Update() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		cloudID, err := uuid.Parse(chi.URLParam(r, "cloudID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cloud id")
			return
		}
		var req nameDescRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		cloud, err := h.svc.Update(r.Context(), projectID, cloudID, req.Name, req.Description)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, cloud)
	}
}

func (h *CloudHandler) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectID, err := uuid.Parse(chi.URLParam(r, "id"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid project id")
			return
		}
		cloudID, err := uuid.Parse(chi.URLParam(r, "cloudID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid cloud id")
			return
		}
		if err := h.svc.Delete(r.Context(), projectID, cloudID); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
