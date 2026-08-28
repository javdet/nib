package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/service"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

// LocationHandler exposes HTTP endpoints for locations
// scoped under a cloud within a project.
type LocationHandler struct {
	svc *service.LocationService
}

func NewLocationHandler(svc *service.LocationService) *LocationHandler {
	return &LocationHandler{svc: svc}
}

func (h *LocationHandler) List() http.HandlerFunc {
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

		locations, err := h.svc.ListByCloud(r.Context(), projectID, cloudID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		if locations == nil {
			locations = []domain.Location{}
		}
		writeJSON(w, http.StatusOK, locations)
	}
}

func (h *LocationHandler) GetByID() http.HandlerFunc {
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

		locationID, err := uuid.Parse(chi.URLParam(r, "locationID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid location id")
			return
		}

		location, err := h.svc.GetByID(r.Context(), projectID, cloudID, locationID)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, location)
	}
}

func (h *LocationHandler) Create() http.HandlerFunc {
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
		location, err := h.svc.Create(r.Context(), projectID, cloudID, req.Name, req.Description)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, location)
	}
}

func (h *LocationHandler) Update() http.HandlerFunc {
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
		locationID, err := uuid.Parse(chi.URLParam(r, "locationID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid location id")
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
		location, err := h.svc.Update(r.Context(), projectID, cloudID, locationID, req.Name, req.Description)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, location)
	}
}

func (h *LocationHandler) Delete() http.HandlerFunc {
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
		locationID, err := uuid.Parse(chi.URLParam(r, "locationID"))
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid location id")
			return
		}
		if err := h.svc.Delete(r.Context(), projectID, cloudID, locationID); err != nil {
			handleServiceError(w, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
