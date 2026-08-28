package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/systemprompts"
	"github.com/go-chi/chi/v5"
)

func newPromptRouter(t *testing.T) chi.Router {
	t.Helper()

	h := NewSystemPromptsHandler(systemprompts.NewService(t.TempDir(), ""))
	r := chi.NewRouter()
	r.Route("/system-prompts/{name}", func(r chi.Router) {
		r.Get("/", h.Get())
		r.Put("/", h.Update())
		r.Delete("/", h.Reset())
	})
	return r
}

func promptRequest(t *testing.T, r chi.Router, method, name, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, "/system-prompts/"+name, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

func decodePrompt(t *testing.T, rec *httptest.ResponseRecorder) systemprompts.Prompt {
	t.Helper()

	var p systemprompts.Prompt
	if err := json.NewDecoder(rec.Body).Decode(&p); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return p
}

func TestSystemPrompts_GetBakedInPrompt(t *testing.T) {
	t.Parallel()

	rec := promptRequest(t, newPromptRouter(t), http.MethodGet, "plan", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	p := decodePrompt(t, rec)
	if p.Source != systemprompts.SourceDefault {
		t.Errorf("source = %q, want %q", p.Source, systemprompts.SourceDefault)
	}
	if strings.TrimSpace(p.Content) == "" {
		t.Error("content is blank")
	}
}

func TestSystemPrompts_WritesRejectedForNonEditablePrompts(t *testing.T) {
	t.Parallel()

	r := newPromptRouter(t)

	if rec := promptRequest(t, r, http.MethodPut, "plan", `{"content":"nope"}`); rec.Code != http.StatusForbidden {
		t.Errorf("PUT /plan status = %d, want 403", rec.Code)
	}
	if rec := promptRequest(t, r, http.MethodDelete, "plan", ""); rec.Code != http.StatusForbidden {
		t.Errorf("DELETE /plan status = %d, want 403", rec.Code)
	}
	if rec := promptRequest(t, r, http.MethodGet, "plan", ""); decodePrompt(t, rec).Source != systemprompts.SourceDefault {
		t.Error("plan prompt changed despite the rejected writes")
	}
}

func TestSystemPrompts_DiscussUpdateAndReset(t *testing.T) {
	t.Parallel()

	r := newPromptRouter(t)

	if rec := promptRequest(t, r, http.MethodPut, "discuss", `{"content":"MARKER-1"}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT status = %d, want 204", rec.Code)
	}

	p := decodePrompt(t, promptRequest(t, r, http.MethodGet, "discuss", ""))
	if p.Source != systemprompts.SourceOverride || p.Content != "MARKER-1" {
		t.Fatalf("after PUT: %+v, want MARKER-1 from override", p)
	}

	if rec := promptRequest(t, r, http.MethodDelete, "discuss", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("DELETE status = %d, want 204", rec.Code)
	}
	if p := decodePrompt(t, promptRequest(t, r, http.MethodGet, "discuss", "")); p.Source != systemprompts.SourceDefault {
		t.Fatalf("after DELETE: source = %q, want %q", p.Source, systemprompts.SourceDefault)
	}
	// Reset is idempotent.
	if rec := promptRequest(t, r, http.MethodDelete, "discuss", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("second DELETE status = %d, want 204", rec.Code)
	}
}

func TestSystemPrompts_GetErrors(t *testing.T) {
	t.Parallel()

	r := newPromptRouter(t)

	if rec := promptRequest(t, r, http.MethodGet, "nosuchprompt", ""); rec.Code != http.StatusNotFound {
		t.Errorf("unknown prompt status = %d, want 404", rec.Code)
	}
	if rec := promptRequest(t, r, http.MethodGet, "has.dot", ""); rec.Code != http.StatusBadRequest {
		t.Errorf("invalid name status = %d, want 400", rec.Code)
	}
}
