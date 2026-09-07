package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/javdet/nib/internal/skills"
)

func newSkillRouter(t *testing.T) (chi.Router, *skills.Service) {
	t.Helper()

	svc := skills.NewService(t.TempDir(), "skills")
	h := NewSkillHandler(svc, nil, nil)

	r := chi.NewRouter()
	r.Route("/skills", func(r chi.Router) {
		r.Get("/", h.List())
		r.Post("/", h.Create())
		r.Route("/{name}", func(r chi.Router) {
			r.Get("/", h.Get())
			r.Get("/rendered", h.GetRendered())
			r.Put("/", h.Update())
			r.Delete("/", h.Delete())
		})
	})
	return r, svc
}

func skillRequest(t *testing.T, r chi.Router, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, strings.NewReader(body))
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	return rec
}

// The system skills carry nib's own documentation. They must not be reachable
// over HTTP: not listed, not readable, not writable.
func TestSkills_SystemSkillsAreNotListed(t *testing.T) {
	t.Parallel()

	r, svc := newSkillRouter(t)
	if err := svc.Create("ours", "---\nname: ours\ndescription: ours\n---\nbody"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	rec := skillRequest(t, r, http.MethodGet, "/skills", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var list skills.ListResult
	if err := json.NewDecoder(rec.Body).Decode(&list); err != nil {
		t.Fatalf("decode body: %v", err)
	}

	got := make([]string, 0, len(list.Skills))
	for _, meta := range list.Skills {
		got = append(got, meta.Name)
	}
	if !slices.Contains(got, "ours") {
		t.Fatalf("List = %v, want the operator's own skill", got)
	}
	for _, name := range skills.SystemNames() {
		if slices.Contains(got, name) {
			t.Fatalf("List = %v, want no system skill; %q leaked into the API", got, name)
		}
	}
}

func TestSkills_SystemSkillIsNotReadableOverHTTP(t *testing.T) {
	t.Parallel()

	r, _ := newSkillRouter(t)

	for _, name := range skills.SystemNames() {
		for _, path := range []string{"/skills/" + name, "/skills/" + name + "/rendered"} {
			rec := skillRequest(t, r, http.MethodGet, path, "")
			if rec.Code != http.StatusNotFound {
				t.Fatalf("GET %s status = %d, want 404", path, rec.Code)
			}
		}
	}
}

func TestSkills_WritingASystemSkillIsForbidden(t *testing.T) {
	t.Parallel()

	r, svc := newSkillRouter(t)
	if err := svc.Create("ours", "body"); err != nil {
		t.Fatalf("Create: %v", err)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{"create", http.MethodPost, "/skills", `{"name":"nib-configuration","content":"hijacked"}`},
		{"update", http.MethodPut, "/skills/nib-configuration", `{"content":"hijacked"}`},
		{"delete", http.MethodDelete, "/skills/nib-configuration", ""},
		// Renaming an ordinary skill onto a system name would shadow the
		// image's own answer in ListAll.
		{"rename onto a system name", http.MethodPut, "/skills/ours", `{"name":"nib-configuration","content":"hijacked"}`},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			rec := skillRequest(t, r, tt.method, tt.path, tt.body)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("%s %s status = %d, want 403; body: %s", tt.method, tt.path, rec.Code, rec.Body)
			}
		})
	}

	if _, err := os.Stat(filepath.Join(svc.Dir(), "ours.md")); err != nil {
		t.Fatalf("the refused rename moved the operator's skill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(svc.Dir(), "nib-configuration.md")); !os.IsNotExist(err) {
		t.Fatalf("a system skill was written to disk: %v", err)
	}
}
