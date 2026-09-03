package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/javdet/nib/internal/service"
)

// A force stop is the only way out of a wedged execution, so its route has to
// answer honestly whether there was anything to stop.
func TestExecutionGetWithNothingRunning(t *testing.T) {
	t.Parallel()

	// A zero ChatService holds no lease, which is the state these routes are
	// about.
	h := NewExecutionHandler(&service.ChatService{})
	rec := httptest.NewRecorder()
	h.Get()(rec, httptest.NewRequest(http.MethodGet, "/api/v1/execution", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("body is not JSON: %v", err)
	}
	if body != nil {
		t.Errorf("body = %v, want null", body)
	}
}

func TestExecutionStopWithNothingRunning(t *testing.T) {
	t.Parallel()

	// A zero ChatService holds no lease, which is the state these routes are
	// about.
	h := NewExecutionHandler(&service.ChatService{})
	rec := httptest.NewRecorder()
	h.Stop()(rec, httptest.NewRequest(http.MethodDelete, "/api/v1/execution", nil))

	// Conflict, not 404: the resource exists, there is just nothing holding it.
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
}
