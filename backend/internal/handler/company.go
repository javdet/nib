package handler

import (
	"net/http"

	"github.com/javdet/nib/internal/service"
)

// CompanyHandler exposes HTTP endpoints for company metadata.
type CompanyHandler struct {
	svc *service.CompanyService
}

func NewCompanyHandler(svc *service.CompanyService) *CompanyHandler {
	return &CompanyHandler{svc: svc}
}

func (h *CompanyHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		info, err := h.svc.Get(r.Context())
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}

func (h *CompanyHandler) Save() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req service.CompanyInfo
		if !decodeJSON(w, r, &req) {
			return
		}

		info, err := h.svc.Save(r.Context(), req)
		if err != nil {
			handleServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, info)
	}
}
