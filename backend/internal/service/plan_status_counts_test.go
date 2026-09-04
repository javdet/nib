package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/metrics"
)

// planCountRepo is a dialog repo that only answers the plan-id query.
type planCountRepo struct {
	stubDialogRepo
	ids []uuid.UUID
	err error
}

func (r *planCountRepo) ListPlanDialogIDs(context.Context) ([]uuid.UUID, error) {
	if r.err != nil {
		return nil, r.err
	}
	return r.ids, nil
}

func writePlanStateFile(t *testing.T, dir string, id uuid.UUID, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, id.String()+".json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write plan state fixture: %v", err)
	}
}

func TestPlanStatusCounts(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		// files maps a plan to the raw plan_state body written for it; an empty
		// body means no file at all.
		files []string
		want  map[string]int
	}{
		{
			name:  "no plans",
			files: nil,
			want:  map[string]int{},
		},
		{
			name: "a plan with no state file counts as draft",
			// This is the common case: a plan only gets a file once its status
			// or schedule is set.
			files: []string{""},
			want:  map[string]int{"draft": 1},
		},
		{
			name: "each status is counted",
			files: []string{
				`{"status":"draft"}`,
				`{"status":"scheduled"}`,
				`{"status":"in_progress"}`,
				`{"status":"done"}`,
				`{"status":"reopened"}`,
				`{"status":"rolled_back"}`,
			},
			want: map[string]int{
				"draft": 1, "scheduled": 1, "in_progress": 1,
				"done": 1, "reopened": 1, "rolled_back": 1,
			},
		},
		{
			name:  "statuses are tallied",
			files: []string{`{"status":"done"}`, `{"status":"done"}`, `{"status":"in_progress"}`},
			want:  map[string]int{"done": 2, "in_progress": 1},
		},
		{
			name: "an unreadable or unknown status falls back to draft",
			// ReadPlanState normalizes both, so the count matches what the
			// dialog list shows rather than inventing a status.
			files: []string{`not json at all`, `{"status":"nonsense"}`},
			want:  map[string]int{"draft": 2},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			dir := t.TempDir()
			repo := &planCountRepo{}
			for _, body := range tt.files {
				id := uuid.New()
				repo.ids = append(repo.ids, id)
				if body != "" {
					writePlanStateFile(t, dir, id, body)
				}
			}

			svc := &ChatService{planStateDir: dir, dialogRepo: repo}
			got, err := svc.PlanStatusCounts(context.Background())
			if err != nil {
				t.Fatalf("PlanStatusCounts: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Fatalf("counts = %v, want %v", got, tt.want)
			}
			for status, want := range tt.want {
				if got[status] != want {
					t.Errorf("counts[%q] = %d, want %d", status, got[status], want)
				}
			}
		})
	}
}

func TestPlanStatusCounts_repositoryError(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		planStateDir: t.TempDir(),
		dialogRepo:   &planCountRepo{err: errors.New("connection refused")},
	}

	if _, err := svc.PlanStatusCounts(context.Background()); err == nil {
		t.Fatal("PlanStatusCounts returned no error when the query failed")
	}
}

// TestPlanStatusesMatchMetricLabels is the drift guard.
//
// internal/metrics is a leaf package and cannot import ActionPlanStatus, so it
// keeps its own copy of the status list. If the two ever disagree, a plan in the
// new status would be counted here and never published, or a label would be
// published that no plan can be in.
func TestPlanStatusesMatchMetricLabels(t *testing.T) {
	t.Parallel()

	labels := planStatusLabels()
	if len(labels) != len(actionPlanStatuses) {
		t.Fatalf("metrics has %d statuses, service has %d: %v vs %v",
			len(labels), len(actionPlanStatuses), labels, actionPlanStatuses)
	}

	inMetrics := make(map[string]bool, len(labels))
	for _, l := range labels {
		inMetrics[l] = true
	}
	for _, status := range actionPlanStatuses {
		if !inMetrics[string(status)] {
			t.Errorf("status %q has no metric label", status)
		}
		if !isValidActionPlanStatus(status) {
			t.Errorf("status %q is in the list but fails validation", status)
		}
	}

	inService := make(map[string]bool, len(actionPlanStatuses))
	for _, status := range actionPlanStatuses {
		inService[string(status)] = true
	}
	for _, l := range labels {
		if !inService[l] {
			t.Errorf("metric label %q is not a valid plan status", l)
		}
	}

	// Guard the guard: a helper that stopped reflecting the metrics package
	// would make this test vacuous.
	if len(metrics.PlanStatuses) != len(labels) {
		t.Errorf("planStatusLabels() does not mirror metrics.PlanStatuses")
	}
}

func TestNormalizeMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mode string
		want string
	}{
		{"orchestrator", "main", "main"},
		{"decompose", "decompose", "decompose"},
		{"incident", "incident", "incident"},
		// POST /chat takes the mode from the request and the agent loop never
		// validates it, so an arbitrary string can reach a label.
		{"unknown mode", "../../etc/passwd", "other"},
		{"empty", "", "other"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeMode(tt.mode); got != tt.want {
				t.Errorf("normalizeMode(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}
