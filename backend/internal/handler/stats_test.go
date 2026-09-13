package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/javdet/nib/internal/service"
)

// A bad range must reach the client as a 400. The sentinel needs a case in
// handleServiceError or it silently degrades to a 500, which is the regression
// this guards.
func TestStatsHandlerRejectsBadRangeWith400(t *testing.T) {
	h := NewStatsHandler(service.NewStatsService(nil, nil))

	tests := []struct {
		name  string
		query string
	}{
		{name: "unknown bucket", query: "?bucket=fortnight"},
		{name: "unparseable from", query: "?from=yesterday"},
		{name: "from after to", query: "?from=2026-09-10&to=2026-09-01"},
		{name: "hourly range over the bucket cap", query: "?from=2025-01-01&to=2026-01-01&bucket=hour"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/api/v1/stats"+tt.query, nil)

			h.Get()(rec, req)

			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
			}
			var body struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode error body: %v", err)
			}
			if body.Error == "" {
				t.Error("error body is empty; the message names the offending parameter")
			}
		})
	}
}
