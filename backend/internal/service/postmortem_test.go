package service

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func newPostmortemTestService(t *testing.T) (*ChatService, *multiDialogRepo) {
	t.Helper()
	repo := newMultiDialogRepo()
	return &ChatService{
		dialogRepo:     repo,
		postmortemsDir: t.TempDir(),
		activity:       NewActivityBroker(),
	}, repo
}

func TestReadPostmortem_missingFileIsEmptyOngoingDocument(t *testing.T) {
	t.Parallel()
	svc, _ := newPostmortemTestService(t)

	doc, err := svc.ReadPostmortem(uuid.New())
	if err != nil {
		t.Fatalf("ReadPostmortem() err = %v", err)
	}
	if doc.Status != IncidentStatusOngoing {
		t.Fatalf("Status = %q, want %q", doc.Status, IncidentStatusOngoing)
	}
	if doc.Version != 0 {
		t.Fatalf("Version = %d, want 0", doc.Version)
	}
	if doc.RootCauses == nil || doc.Timeline == nil || doc.ShortTerm == nil || doc.Findings == nil || doc.Locks == nil {
		t.Fatal("empty document carries nil slices; the page reads them as null")
	}
}

func TestApplyPostmortemPatch_roleMatrix(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		role    incidentRole
		field   string
		applied bool
	}{
		{"orchestrator writes summary", incidentRoleOrchestrator, pmFieldSummary, true},
		{"orchestrator cannot write impact", incidentRoleOrchestrator, pmFieldImpact, false},
		{"orchestrator cannot write closure", incidentRoleOrchestrator, pmFieldClosure, false},
		{"synthesis writes hypothesis", incidentRoleSynthesis, pmFieldHypothesis, true},
		{"synthesis cannot write status", incidentRoleSynthesis, pmFieldStatus, false},
		{"impact writes impact", incidentRoleImpact, pmFieldImpact, true},
		{"impact cannot write summary", incidentRoleImpact, pmFieldSummary, false},
		{"closure writes closure", incidentRoleClosure, pmFieldClosure, true},
		{"closure cannot write summary", incidentRoleClosure, pmFieldSummary, false},
		{"digger cannot write summary", incidentRoleDigger, pmFieldSummary, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := newPostmortem()
			value := "resolved"
			refusals, err := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{tt.field: value}}, tt.role)
			if err != nil {
				t.Fatalf("applyPostmortemPatch() err = %v", err)
			}
			got := *doc.scalar(tt.field) == value
			if got != tt.applied {
				t.Fatalf("%s applied = %v, want %v (refusals %v)", tt.field, got, tt.applied, refusals)
			}
			if !tt.applied && len(refusals) != 1 {
				t.Fatalf("refusals = %v, want one naming %s", refusals, tt.field)
			}
		})
	}
}

func TestApplyPostmortemPatch_operatorLocksAgentRefusedPerField(t *testing.T) {
	t.Parallel()
	doc := newPostmortem()

	if _, err := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{pmFieldSummary: "operator words"}}, incidentRoleOperator); err != nil {
		t.Fatalf("operator patch err = %v", err)
	}
	if !doc.locked(pmFieldSummary) {
		t.Fatal("operator edit did not lock summary")
	}

	refusals, err := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{
		pmFieldSummary:    "agent words",
		pmFieldHypothesis: "agent hypothesis",
	}}, incidentRoleOrchestrator)
	if err != nil {
		t.Fatalf("agent patch err = %v", err)
	}
	if doc.Summary != "operator words" {
		t.Fatalf("Summary = %q, want the operator's", doc.Summary)
	}
	if doc.Hypothesis != "agent hypothesis" {
		t.Fatalf("Hypothesis = %q, want the agent's: the rest of a refused patch still applies", doc.Hypothesis)
	}
	if len(refusals) != 1 || !strings.Contains(refusals[0], "owned by the operator") {
		t.Fatalf("refusals = %v, want one naming the operator's ownership", refusals)
	}

	if _, err := applyPostmortemPatch(&doc, PostmortemPatch{Unlock: []string{pmFieldSummary}}, incidentRoleOperator); err != nil {
		t.Fatalf("unlock err = %v", err)
	}
	if _, err := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{pmFieldSummary: "agent again"}}, incidentRoleOrchestrator); err != nil {
		t.Fatalf("agent patch after unlock err = %v", err)
	}
	if doc.Summary != "agent again" {
		t.Fatalf("Summary = %q after unlock, want the agent's", doc.Summary)
	}
}

func TestApplyPostmortemPatch_agentCannotClose(t *testing.T) {
	t.Parallel()
	doc := newPostmortem()
	refusals, _ := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{pmFieldStatus: "closed"}}, incidentRoleOrchestrator)
	if doc.Status != IncidentStatusOngoing {
		t.Fatalf("Status = %q, want ongoing", doc.Status)
	}
	if len(refusals) != 1 {
		t.Fatalf("refusals = %v, want one", refusals)
	}
}

func TestApplyPostmortemPatch_closedRefusesAgentsButNotOperatorOrClosure(t *testing.T) {
	t.Parallel()
	doc := newPostmortem()
	doc.Status = IncidentStatusClosed

	refusals, _ := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{pmFieldSummary: "late"}}, incidentRoleSynthesis)
	if doc.Summary != "" || len(refusals) != 1 {
		t.Fatalf("closed incident took an agent write: summary %q, refusals %v", doc.Summary, refusals)
	}
	if _, err := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{pmFieldClosure: "done"}}, incidentRoleClosure); err != nil || doc.Closure != "done" {
		t.Fatalf("closure write: closure %q, err %v", doc.Closure, err)
	}
	if _, err := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{pmFieldSummary: "op"}}, incidentRoleOperator); err != nil || doc.Summary != "op" {
		t.Fatalf("operator write: summary %q, err %v", doc.Summary, err)
	}
}

func TestApplyPostmortemPatch_validation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
		value string
		want  string
		ok    bool
	}{
		{"time normalised to UTC", pmFieldStartedAt, "2026-10-07T16:05:00+02:00", "2026-10-07T14:05:00Z", true},
		{"time without zone", pmFieldDetectedAt, "2026-10-07 14:05", "2026-10-07T14:05:00Z", true},
		{"time cleared", pmFieldResolvedAt, "", "", true},
		{"time garbage", pmFieldStartedAt, "yesterday", "", false},
		{"severity case folded", pmFieldSeverity, "SEV2", "sev2", true},
		{"severity unknown", pmFieldSeverity, "p1", "", false},
		{"status unknown", pmFieldStatus, "fixed", IncidentStatusOngoing, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			doc := newPostmortem()
			_, err := applyPostmortemPatch(&doc, PostmortemPatch{Set: map[string]string{tt.field: tt.value}}, incidentRoleOperator)
			if (err == nil) != tt.ok {
				t.Fatalf("err = %v, want ok %v", err, tt.ok)
			}
			if !tt.ok && !errors.Is(err, ErrInvalidPostmortemPatch) {
				t.Fatalf("err = %v, want ErrInvalidPostmortemPatch", err)
			}
			if got := *doc.scalar(tt.field); tt.ok && got != tt.want {
				t.Fatalf("%s = %q, want %q", tt.field, got, tt.want)
			}
		})
	}
}

func TestApplyPostmortemPatch_agentListKeepsOperatorAndPlannedItems(t *testing.T) {
	t.Parallel()
	doc := newPostmortem()

	agent := func(texts ...string) []string {
		actions := make([]PostmortemAction, 0, len(texts))
		for _, text := range texts {
			actions = append(actions, PostmortemAction{Text: text})
		}
		refusals, err := applyPostmortemPatch(&doc, PostmortemPatch{Lists: &PostmortemLists{ShortTerm: actions}}, incidentRoleOrchestrator)
		if err != nil {
			t.Fatalf("agent list err = %v", err)
		}
		return refusals
	}

	agent("Roll back api to v1.4", "Scale workers to 6")
	if len(doc.ShortTerm) != 2 {
		t.Fatalf("ShortTerm = %+v, want 2 items", doc.ShortTerm)
	}
	rollbackID := doc.ShortTerm[0].ID
	if doc.ShortTerm[0].Status != PostmortemActionTodo {
		t.Fatalf("Status = %q, want todo by default", doc.ShortTerm[0].Status)
	}

	// The operator adds one, and a plan takes the scaling action.
	if _, err := applyPostmortemPatch(&doc, PostmortemPatch{Ops: []PostmortemItemOp{
		{List: pmListShortTerm, Op: "add", Fields: PostmortemItemFields{Text: "Page the DB on-call"}},
	}}, incidentRoleOperator); err != nil {
		t.Fatalf("operator add err = %v", err)
	}
	doc.ShortTerm[1].Status = PostmortemActionInPlan
	doc.ShortTerm[1].PlanID = uuid.NewString()

	// The agent restates only the rollback, reworded in case and spacing.
	agent("roll back  API to v1.4")

	var texts []string
	for _, a := range doc.ShortTerm {
		texts = append(texts, a.Text)
	}
	if len(doc.ShortTerm) != 3 {
		t.Fatalf("ShortTerm = %v, want rollback, the planned item and the operator's", texts)
	}
	if doc.ShortTerm[0].ID != rollbackID {
		t.Fatalf("restated item id = %q, want %q kept", doc.ShortTerm[0].ID, rollbackID)
	}
	if !slices.Contains(texts, "Scale workers to 6") || !slices.Contains(texts, "Page the DB on-call") {
		t.Fatalf("ShortTerm = %v, want the planned and the operator's item kept", texts)
	}

	// Sending the list empty drops every item the agent owns, and only those.
	agent()
	if len(doc.ShortTerm) != 2 {
		t.Fatalf("ShortTerm after empty list = %+v, want the 2 pinned items", doc.ShortTerm)
	}
}

func TestApplyPostmortemPatch_agentRefusedListAndInPlanStatus(t *testing.T) {
	t.Parallel()
	doc := newPostmortem()

	refusals, err := applyPostmortemPatch(&doc, PostmortemPatch{Lists: &PostmortemLists{
		Impacted:  []PostmortemImpacted{{Name: "api"}},
		ShortTerm: []PostmortemAction{{Text: "x", Status: "in_plan"}},
	}}, incidentRoleSynthesis)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(doc.Impacted) != 0 {
		t.Fatalf("Impacted = %+v, want synthesis refused", doc.Impacted)
	}
	if len(doc.ShortTerm) != 1 || doc.ShortTerm[0].Status != PostmortemActionTodo {
		t.Fatalf("ShortTerm = %+v, want one todo item", doc.ShortTerm)
	}
	if len(refusals) != 2 {
		t.Fatalf("refusals = %v, want the list and the status", refusals)
	}
}

func TestApplyPostmortemPatch_operatorOps(t *testing.T) {
	t.Parallel()
	doc := newPostmortem()

	ops := func(op ...PostmortemItemOp) error {
		_, err := applyPostmortemPatch(&doc, PostmortemPatch{Ops: op}, incidentRoleOperator)
		return err
	}

	if err := ops(
		PostmortemItemOp{List: pmListTimeline, Op: "add", Fields: PostmortemItemFields{At: "2026-10-07T14:10:00Z", Event: "alert fired"}},
		PostmortemItemOp{List: pmListTimeline, Op: "add", Fields: PostmortemItemFields{At: "2026-10-07T14:00:00Z", Event: "deploy"}},
		PostmortemItemOp{List: pmListTimeline, Op: "add", Fields: PostmortemItemFields{At: "around lunch", Event: "first complaint"}},
	); err != nil {
		t.Fatalf("add err = %v", err)
	}
	if got := []string{doc.Timeline[0].Event, doc.Timeline[1].Event, doc.Timeline[2].Event}; !slices.Equal(got, []string{"deploy", "alert fired", "first complaint"}) {
		t.Fatalf("timeline order = %v, want chronological with the unparsed time last", got)
	}
	if !doc.Timeline[0].Operator {
		t.Fatal("operator item not marked as the operator's")
	}

	if err := ops(PostmortemItemOp{List: pmListTimeline, Op: "delete", ID: doc.Timeline[2].ID}); err != nil {
		t.Fatalf("delete err = %v", err)
	}
	if len(doc.Timeline) != 2 {
		t.Fatalf("Timeline = %+v, want 2 after delete", doc.Timeline)
	}

	tests := []struct {
		name string
		op   PostmortemItemOp
	}{
		{"unknown list", PostmortemItemOp{List: "notes", Op: "add"}},
		{"unknown op", PostmortemItemOp{List: pmListRootCauses, Op: "move"}},
		{"unknown id", PostmortemItemOp{List: pmListRootCauses, Op: "update", ID: "rc-99", Fields: PostmortemItemFields{Text: "x"}}},
		{"missing text", PostmortemItemOp{List: pmListRootCauses, Op: "add"}},
		{"in_plan by hand", PostmortemItemOp{List: pmListLongTerm, Op: "add", Fields: PostmortemItemFields{Text: "x", Status: "in_plan"}}},
	}
	for _, tt := range tests {
		if err := ops(tt.op); !errors.Is(err, ErrInvalidPostmortemPatch) {
			t.Errorf("%s: err = %v, want ErrInvalidPostmortemPatch", tt.name, err)
		}
	}
}

func TestApplyPostmortemPatch_agentCannotUseOps(t *testing.T) {
	t.Parallel()
	doc := newPostmortem()
	refusals, err := applyPostmortemPatch(&doc, PostmortemPatch{Ops: []PostmortemItemOp{
		{List: pmListRootCauses, Op: "add", Fields: PostmortemItemFields{Text: "x"}},
	}}, incidentRoleOrchestrator)
	if err != nil || len(refusals) != 1 || len(doc.RootCauses) != 0 {
		t.Fatalf("agent ops: err %v, refusals %v, root causes %+v", err, refusals, doc.RootCauses)
	}
}

func TestUpdatePostmortem_versionAndUnchangedWrite(t *testing.T) {
	t.Parallel()
	svc, _ := newPostmortemTestService(t)
	root := uuid.New()

	events, cancel := svc.SubscribeActivity(root)
	defer cancel()

	set := func(v string) Postmortem {
		doc, err := svc.updatePostmortem(root, func(doc *Postmortem) error {
			_, err := applyPostmortemPatch(doc, PostmortemPatch{Set: map[string]string{pmFieldSummary: v}}, incidentRoleOrchestrator)
			return err
		})
		if err != nil {
			t.Fatalf("updatePostmortem() err = %v", err)
		}
		return doc
	}

	if doc := set("a"); doc.Version != 1 {
		t.Fatalf("Version = %d, want 1", doc.Version)
	}
	if got := (<-events).Kind; got != domain.ActivityPostmortemUpdated {
		t.Fatalf("activity = %q, want %q", got, domain.ActivityPostmortemUpdated)
	}
	if doc := set("a"); doc.Version != 1 {
		t.Fatalf("Version = %d after an unchanged write, want 1", doc.Version)
	}
	select {
	case ev := <-events:
		t.Fatalf("unchanged write published %q", ev.Kind)
	default:
	}
	if doc := set("b"); doc.Version != 2 {
		t.Fatalf("Version = %d, want 2", doc.Version)
	}

	read, err := svc.ReadPostmortem(root)
	if err != nil || read.Summary != "b" || read.Version != 2 {
		t.Fatalf("ReadPostmortem() = %q v%d, err %v; want b v2", read.Summary, read.Version, err)
	}
}

func TestUpdatePostmortem_concurrentWritersAllLand(t *testing.T) {
	t.Parallel()
	svc, _ := newPostmortemTestService(t)
	root := uuid.New()

	const writers = 16
	var wg sync.WaitGroup
	for i := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := svc.updatePostmortem(root, func(doc *Postmortem) error {
				upsertFinding(doc, PostmortemFinding{System: "system-" + string(rune('a'+i)), Status: "healthy"})
				return nil
			})
			if err != nil {
				t.Errorf("updatePostmortem() err = %v", err)
			}
		}()
	}
	wg.Wait()

	doc, err := svc.ReadPostmortem(root)
	if err != nil {
		t.Fatalf("ReadPostmortem() err = %v", err)
	}
	if len(doc.Findings) != writers || doc.Version != writers {
		t.Fatalf("findings = %d, version = %d; want %d of each", len(doc.Findings), doc.Version, writers)
	}
}

func TestIncidentPostmortem_refusesDialogsThatAreNotIncidentRoots(t *testing.T) {
	t.Parallel()
	svc, repo := newPostmortemTestService(t)
	ctx := context.Background()

	incident := repo.add(domain.Dialog{Mode: incidentDialogMode})
	child, _ := repo.CreateDialog(ctx, incidentDialogMode, "digger", &incident)
	plan := repo.add(domain.Dialog{Mode: "main"})

	if _, err := svc.IncidentPostmortem(ctx, incident); err != nil {
		t.Fatalf("IncidentPostmortem(root) err = %v", err)
	}
	for name, id := range map[string]uuid.UUID{"child": child.ID, "plan": plan} {
		if _, err := svc.IncidentPostmortem(ctx, id); !errors.Is(err, ErrNotIncidentDialog) {
			t.Errorf("IncidentPostmortem(%s) err = %v, want ErrNotIncidentDialog", name, err)
		}
	}

	doc, err := svc.EditIncidentPostmortem(ctx, incident, PostmortemPatch{Set: map[string]string{pmFieldSeverity: "sev2"}})
	if err != nil || doc.Severity != "sev2" || !doc.locked(pmFieldSeverity) {
		t.Fatalf("EditIncidentPostmortem() = %+v, err %v; want sev2 locked", doc, err)
	}
}

func TestUpdatePostmortemHandler_reportsRefusalsAndUnknownFields(t *testing.T) {
	t.Parallel()
	svc, _ := newPostmortemTestService(t)
	root := uuid.New()

	out, err := svc.updatePostmortemHandler(root, incidentRoleImpact)(context.Background(), map[string]any{
		"set": map[string]any{"impact": "5% of checkouts failed", "summary": "x", "colour": "red"},
		"lists": map[string]any{
			"impacted": []any{map[string]any{"name": "checkout", "environment": "prod"}},
		},
	})
	if err != nil {
		t.Fatalf("handler err = %v", err)
	}
	if !strings.Contains(out, "version 1") || !strings.Contains(out, "summary is not yours") || !strings.Contains(out, "colour") {
		t.Fatalf("result = %q, want the version, the refused field and the unknown one", out)
	}

	doc, _ := svc.ReadPostmortem(root)
	if doc.Impact != "5% of checkouts failed" || len(doc.Impacted) != 1 || doc.Impacted[0].Name != "checkout" {
		t.Fatalf("doc = %+v, want impact and impacted written", doc)
	}
}
