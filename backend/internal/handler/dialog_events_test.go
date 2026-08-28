package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/domain"
)

type flushRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flushRecorder) Flush() {}

func TestWriteSSEEvent(t *testing.T) {
	rec := httptest.NewRecorder()
	w := &flushRecorder{ResponseRecorder: rec}

	ev := domain.AgentActivity{
		Kind:  domain.ActivityToolsStart,
		Round: 1,
		Count: 2,
	}
	if err := writeSSEEvent(w, ev); err != nil {
		t.Fatalf("writeSSEEvent: %v", err)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "data: ") {
		t.Fatalf("body missing data prefix: %q", body)
	}
	if !strings.Contains(body, `"kind":"tools_start"`) {
		t.Fatalf("body = %q", body)
	}
	if !strings.HasSuffix(body, "\n\n") {
		t.Fatalf("body should end with blank line: %q", body)
	}
}
