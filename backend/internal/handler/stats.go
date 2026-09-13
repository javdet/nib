package handler

import (
	"net/http"
	"time"

	"github.com/javdet/nib/internal/service"
)

// StatsHandler serves the statistics dashboard.
type StatsHandler struct {
	svc *service.StatsService
}

func NewStatsHandler(svc *service.StatsService) *StatsHandler {
	return &StatsHandler{svc: svc}
}

// Get answers GET /api/v1/stats?from=&to=&bucket=.
//
// One composite response rather than an endpoint per section: the page loads as
// a unit, and the frontend has no query cache that would make several requests
// cheap.
func (h *StatsHandler) Get() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		rg, err := service.ParseStatsRange(
			q.Get("from"), q.Get("to"), q.Get("bucket"), time.Now())
		if err != nil {
			handleServiceError(w, err)
			return
		}

		report, err := h.svc.Report(r.Context(), rg)
		if err != nil {
			handleServiceError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, report)
	}
}
