package service

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func newHandoffTestService(t *testing.T) (*ChatService, *multiDialogRepo, uuid.UUID) {
	t.Helper()
	repo := newMultiDialogRepo()
	svc := &ChatService{
		dialogRepo:     repo,
		postmortemsDir: t.TempDir(),
		summariesDir:   t.TempDir(),
		planOriginDir:  t.TempDir(),
		activity:       NewActivityBroker(),
	}
	incident := repo.add(domain.Dialog{Mode: incidentDialogMode, Title: "Checkout 502s"})
	_, err := svc.updatePostmortem(incident, func(doc *Postmortem) error {
		_, err := applyPostmortemPatch(doc, PostmortemPatch{
			Set: map[string]string{pmFieldSummary: "Checkout returned 502 for 5% of requests.", pmFieldEnvironment: "prod"},
			Lists: &PostmortemLists{
				RootCauses: []PostmortemItem{{Text: "DB connection pool exhausted"}},
				ShortTerm:  []PostmortemAction{{Text: "Raise the pool to 200"}, {Text: "Restart stuck workers"}},
				LongTerm:   []PostmortemAction{{Text: "Alert on pool saturation"}},
			},
		}, incidentRoleOrchestrator)
		return err
	})
	if err != nil {
		t.Fatalf("seed post-mortem: %v", err)
	}
	return svc, repo, incident
}

func TestCreatePlanFromIncident_createsLinkedMainPlan(t *testing.T) {
	t.Parallel()
	svc, repo, incident := newHandoffTestService(t)
	ctx := context.Background()

	doc, _ := svc.ReadPostmortem(incident)
	raise, longTerm := doc.ShortTerm[0].ID, doc.LongTerm[0].ID

	plan, err := svc.CreatePlanFromIncident(ctx, incident, []string{raise, longTerm, raise})
	if err != nil {
		t.Fatalf("CreatePlanFromIncident() err = %v", err)
	}

	d, err := repo.GetDialog(ctx, plan.PlanID)
	if err != nil {
		t.Fatalf("plan dialog: %v", err)
	}
	if d.Mode != mainDialogMode || d.ParentID != nil {
		t.Fatalf("plan = mode %q parent %v, want a main root", d.Mode, d.ParentID)
	}
	if d.Title != "Remediate: Checkout 502s" {
		t.Fatalf("plan title = %q", d.Title)
	}

	for _, want := range []string{"Checkout 502s", "DB connection pool exhausted", "1. Raise the pool to 200", "Long-term", "1. Alert on pool saturation", "Environment: prod"} {
		if !strings.Contains(plan.Message, want) {
			t.Errorf("message lacks %q:\n%s", want, plan.Message)
		}
	}
	if strings.Contains(plan.Message, "Restart stuck workers") {
		t.Errorf("message carries an action that was not chosen:\n%s", plan.Message)
	}

	summary, err := os.ReadFile(filepath.Join(svc.summariesDir, plan.PlanID.String()+".txt"))
	if err != nil || !strings.Contains(string(summary), "Raise the pool to 200") {
		t.Fatalf("plan summary = %q, err %v", summary, err)
	}

	origin, found, err := svc.ReadPlanOrigin(plan.PlanID)
	if err != nil || !found || origin.IncidentID != incident.String() || !slices.Equal(origin.ActionIDs, []string{raise, longTerm}) {
		t.Fatalf("origin = %+v found %v err %v", origin, found, err)
	}

	doc, _ = svc.ReadPostmortem(incident)
	for _, id := range []string{raise, longTerm} {
		a := doc.findAction(id)
		if a.Status != PostmortemActionInPlan || a.PlanID != plan.PlanID.String() {
			t.Errorf("action %s = %+v, want in_plan on the new plan", id, a)
		}
	}
	if doc.ShortTerm[1].Status != PostmortemActionTodo {
		t.Errorf("unchosen action = %+v, want todo", doc.ShortTerm[1])
	}
	if len(doc.LinkedPlans) != 1 || doc.LinkedPlans[0].PlanID != plan.PlanID.String() {
		t.Fatalf("LinkedPlans = %+v", doc.LinkedPlans)
	}

	withTitle, found, err := svc.PlanOriginForDialog(ctx, plan.PlanID)
	if err != nil || !found || withTitle.IncidentTitle != "Checkout 502s" {
		t.Fatalf("PlanOriginForDialog() = %+v found %v err %v", withTitle, found, err)
	}

	// A second press for the same action is refused rather than planned twice.
	if _, err := svc.CreatePlanFromIncident(ctx, incident, []string{raise}); !errors.Is(err, ErrPostmortemActionPlanned) {
		t.Fatalf("second CreatePlanFromIncident() err = %v, want ErrPostmortemActionPlanned", err)
	}
}

func TestCreatePlanFromIncident_refusals(t *testing.T) {
	t.Parallel()
	svc, repo, incident := newHandoffTestService(t)
	ctx := context.Background()
	plan := repo.add(domain.Dialog{Mode: mainDialogMode})

	tests := []struct {
		name    string
		id      uuid.UUID
		actions []string
		want    error
	}{
		{"no actions", incident, nil, ErrNoActionsSelected},
		{"blank actions", incident, []string{" "}, ErrNoActionsSelected},
		{"unknown action", incident, []string{"st-99"}, ErrUnknownPostmortemItem},
		{"not an incident", plan, []string{"st-1"}, ErrNotIncidentDialog},
	}
	for _, tt := range tests {
		if _, err := svc.CreatePlanFromIncident(ctx, tt.id, tt.actions); !errors.Is(err, tt.want) {
			t.Errorf("%s: err = %v, want %v", tt.name, err, tt.want)
		}
	}

	doc, _ := svc.ReadPostmortem(incident)
	if len(doc.LinkedPlans) != 0 {
		t.Fatalf("a refused request linked a plan: %+v", doc.LinkedPlans)
	}
}

func TestReadPlanOrigin_absentForOrdinaryPlan(t *testing.T) {
	t.Parallel()
	svc := &ChatService{planOriginDir: t.TempDir()}
	if _, found, err := svc.ReadPlanOrigin(uuid.New()); err != nil || found {
		t.Fatalf("ReadPlanOrigin() found %v err %v, want neither", found, err)
	}
}
