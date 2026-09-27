package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

// fakeDialogImporter writes an import into a multiDialogRepo, so the service's
// own readers see the result exactly as they would after a real commit.
type fakeDialogImporter struct {
	repo *multiDialogRepo
	err  error
	got  []domain.DialogImport
}

func (f *fakeDialogImporter) ImportDialogs(ctx context.Context, dialogs []domain.DialogImport) error {
	if f.err != nil {
		return f.err
	}
	f.got = dialogs
	for _, d := range dialogs {
		f.repo.add(d.Dialog)
		for _, m := range d.Messages {
			if _, err := f.repo.AppendMessage(ctx, d.Dialog.ID, m); err != nil {
				return err
			}
		}
	}
	return nil
}

// stubAttachmentRepo serves canned attachment rows per dialog.
type stubAttachmentRepo struct {
	byDialog map[uuid.UUID][]domain.Attachment
}

func (r stubAttachmentRepo) CreateAttachment(_ context.Context, a domain.Attachment) (domain.Attachment, error) {
	return a, nil
}

func (r stubAttachmentRepo) ListAttachmentsByDialog(_ context.Context, dialogID uuid.UUID) ([]domain.Attachment, error) {
	return r.byDialog[dialogID], nil
}

func (r stubAttachmentRepo) GetAttachment(context.Context, uuid.UUID, uuid.UUID) (domain.Attachment, error) {
	return domain.Attachment{}, repository.ErrNotFound
}

func (r stubAttachmentRepo) DeleteAttachment(context.Context, uuid.UUID, uuid.UUID) error {
	return nil
}

func (r stubAttachmentRepo) LinkAttachmentsToMessage(context.Context, uuid.UUID, int64, []uuid.UUID) error {
	return nil
}

func (r stubAttachmentRepo) ClaimOrphanedAttachmentFiles(context.Context, int) ([]domain.OrphanedAttachmentFile, error) {
	return nil, nil
}

func (r stubAttachmentRepo) DropOrphanedAttachmentFiles(context.Context, []int64) error {
	return nil
}

func (r stubAttachmentRepo) DeleteUnsentAttachments(context.Context, string) (int64, error) {
	return 0, nil
}

func newBundleTestService(t *testing.T, repo *multiDialogRepo, attachments repository.AttachmentRepository) *ChatService {
	t.Helper()
	svc := NewChatService(nil, nil, nil, nil, repo, attachments, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "", t.TempDir(), "", 10, PlanFanoutConfig{}, ActionExecConfig{})
	svc.SetDialogImporter(&fakeDialogImporter{repo: repo})
	return svc
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// seedExportablePlan builds a plan with one of everything a bundle carries and
// one of everything it must leave behind.
func seedExportablePlan(t *testing.T, svc *ChatService, repo *multiDialogRepo) (root, decompose, execute uuid.UUID) {
	t.Helper()
	ctx := context.Background()

	root = repo.add(domain.Dialog{Title: "Upgrade Postgres", Mode: "main", Categories: []string{"database"}})
	decompose = repo.add(domain.Dialog{Title: decomposeSubagentTitle, Mode: decomposeSubagentMode, ParentID: &root})
	execute = repo.add(domain.Dialog{Title: "Run step", Mode: executeDialogMode, ParentID: &root})

	for _, m := range []domain.DialogMessage{
		{Role: "system", Content: "exporter's prompt: CompanyName=Acme"},
		{Role: "user", Content: "upgrade postgres to 17"},
		{Role: "assistant", ToolCalls: json.RawMessage(`[{"id":"c1","type":"function","function":{"name":"run_subagent","arguments":"{}"}}]`)},
		{Role: "tool", ToolCallID: "c1", Name: "run_subagent", Content: "DAG saved to dags/" + root.String() + ".md"},
	} {
		if _, err := repo.AppendMessage(ctx, root, m); err != nil {
			t.Fatal(err)
		}
	}
	for _, m := range []domain.DialogMessage{
		{Role: "system", Content: "exporter's decompose prompt"},
		{Role: "user", Content: "decompose it"},
	} {
		if _, err := repo.AppendMessage(ctx, decompose, m); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := repo.AppendMessage(ctx, execute, domain.DialogMessage{Role: "user", Content: "ran on prod"}); err != nil {
		t.Fatal(err)
	}

	if err := svc.WriteSummary(root, "Upgrade in place."); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, filepath.Join(svc.dagsDir, root.String()+".md"), "```mermaid\ngraph TD\nA-->B\n```")
	writeTestFile(t, filepath.Join(svc.planContractsDir, root.String()+".json"), `{"shared":[{"key":"db","value":"pg17"}],"stages":{},"future":"kept"}`)
	if _, err := svc.WriteActionPlan(root, json.RawMessage(`{"stages":[{"title":"Backup","steps":[{"type":"shell","action":"dump","categories":[]}],"checks":[]}],"rollback":[]}`)); err != nil {
		t.Fatal(err)
	}
	if err := svc.WriteActionPlanComments(root, map[string]string{"s0.step1": "check disk first"}); err != nil {
		t.Fatal(err)
	}
	// Execution state: none of it may travel.
	if err := svc.WriteActionPlanChecks(root, []string{"s0.step1"}); err != nil {
		t.Fatal(err)
	}
	if err := svc.WriteActionPlanRuns(root, map[string]string{"s0.step1": execute.String()}); err != nil {
		t.Fatal(err)
	}
	if err := svc.WritePlanState(root, PlanState{Status: ActionPlanStatusInProgress}); err != nil {
		t.Fatal(err)
	}
	if err := svc.WriteReport(root, "it went fine"); err != nil {
		t.Fatal(err)
	}
	if err := svc.recordSubagentPause(root, SubagentPause{
		Subagent: SubagentDecompose, DialogID: decompose.String(), ToolCallID: "ask1", MainAskID: "subagent_ask_x",
	}); err != nil {
		t.Fatal(err)
	}
	return root, decompose, execute
}

func TestExportPlanCarriesThePlanAndNotItsRun(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := newBundleTestService(t, repo, nil)
	root, decompose, _ := seedExportablePlan(t, svc, repo)

	b, err := svc.ExportPlan(context.Background(), root)
	if err != nil {
		t.Fatalf("ExportPlan() error = %v", err)
	}

	if b.Format != PlanBundleFormat || b.Version != PlanBundleVersion {
		t.Fatalf("format/version = %q/%d, want %q/%d", b.Format, b.Version, PlanBundleFormat, PlanBundleVersion)
	}
	if b.Plan.ID != root || b.Plan.Title != "Upgrade Postgres" || b.Plan.Mode != "main" {
		t.Fatalf("Plan = %+v, want the root dialog", b.Plan)
	}
	if b.Summary != "Upgrade in place." {
		t.Errorf("Summary = %q", b.Summary)
	}
	if !strings.Contains(b.DAG, "A-->B") {
		t.Errorf("DAG = %q, want the stored mermaid", b.DAG)
	}
	if !strings.Contains(string(b.Contract), `"future":"kept"`) {
		t.Errorf("Contract = %s, want the stored document verbatim", b.Contract)
	}
	if len(b.ActionPlan) == 0 {
		t.Error("ActionPlan is empty")
	}
	if got := b.Comments["s0.step1"]; got != "check disk first" {
		t.Errorf("Comments[s0.step1] = %q, want the operator's comment", got)
	}

	if len(b.Chat) != 3 {
		t.Fatalf("len(Chat) = %d, want 3 (system row dropped)", len(b.Chat))
	}
	for _, m := range b.Chat {
		if m.Role == "system" {
			t.Fatal("Chat carries a system row")
		}
	}
	if len(b.Subagents) != 1 || b.Subagents[0].ID != decompose {
		t.Fatalf("Subagents = %+v, want only the decompose transcript", b.Subagents)
	}
	if len(b.Pauses) != 1 || b.Pauses[0].DialogID != decompose.String() {
		t.Fatalf("Pauses = %+v, want the decompose pause", b.Pauses)
	}

	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	for _, leaked := range []string{"ran on prod", "it went fine", "in_progress", "CompanyName=Acme"} {
		if strings.Contains(string(raw), leaked) {
			t.Errorf("bundle contains %q, which belongs to the run or the exporting install", leaked)
		}
	}
}

func TestExportPlanRefusesAChildDialog(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := newBundleTestService(t, repo, nil)
	_, decompose, _ := seedExportablePlan(t, svc, repo)

	if _, err := svc.ExportPlan(context.Background(), decompose); !errors.Is(err, ErrNotAPlan) {
		t.Fatalf("ExportPlan(child) error = %v, want ErrNotAPlan", err)
	}
}

func TestImportPlanRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srcRepo := newMultiDialogRepo()
	src := newBundleTestService(t, srcRepo, nil)
	oldRoot, oldDecompose, _ := seedExportablePlan(t, src, srcRepo)
	b, err := src.ExportPlan(ctx, oldRoot)
	if err != nil {
		t.Fatal(err)
	}
	// Through JSON, as the file travels.
	raw, err := json.Marshal(b)
	if err != nil {
		t.Fatal(err)
	}
	var decoded PlanBundle
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}

	dstRepo := newMultiDialogRepo()
	dst := newBundleTestService(t, dstRepo, nil)
	d, err := dst.ImportPlan(ctx, decoded)
	if err != nil {
		t.Fatalf("ImportPlan() error = %v", err)
	}
	if d.ID == oldRoot {
		t.Fatal("imported plan kept the exporter's id")
	}
	if d.Title != "Upgrade Postgres" || d.Mode != "main" || d.ParentID != nil {
		t.Fatalf("imported root = %+v", d)
	}
	if len(d.Categories) != 1 || d.Categories[0] != "database" {
		t.Errorf("Categories = %v, want [database]", d.Categories)
	}

	msgs, _ := dstRepo.ListMessages(ctx, d.ID)
	if len(msgs) != 4 {
		t.Fatalf("len(messages) = %d, want 4", len(msgs))
	}
	if msgs[0].Role != "system" || strings.Contains(msgs[0].Content, "Acme") {
		t.Errorf("messages[0] = %+v, want this install's system prompt", msgs[0])
	}
	if want := "DAG saved to dags/" + d.ID.String() + ".md"; msgs[3].Content != want {
		t.Errorf("tool result = %q, want %q", msgs[3].Content, want)
	}

	children, _ := dstRepo.ListChildren(ctx, d.ID)
	if len(children) != 1 || children[0].Mode != decomposeSubagentMode {
		t.Fatalf("children = %+v, want the decompose transcript alone", children)
	}
	newDecompose := children[0].ID
	if newDecompose == oldDecompose {
		t.Fatal("imported sub-agent kept the exporter's id")
	}
	childMsgs, _ := dstRepo.ListMessages(ctx, newDecompose)
	if len(childMsgs) != 2 || childMsgs[0].Role != "system" {
		t.Fatalf("decompose messages = %+v, want a fresh system row plus the task", childMsgs)
	}

	if summary, found, _ := dst.ReadSummary(d.ID); !found || summary != "Upgrade in place." {
		t.Errorf("summary = %q (found %v)", summary, found)
	}
	if dag, found, _ := dst.ReadDAG(d.ID); !found || !strings.Contains(dag, "A-->B") {
		t.Errorf("dag = %q (found %v)", dag, found)
	}
	if contract, found, _ := dst.ReadPlanContract(d.ID); !found || len(contract.Shared) != 1 {
		t.Errorf("contract = %+v (found %v)", contract, found)
	}
	if _, found, _ := dst.ReadActionPlan(d.ID); !found {
		t.Error("action plan was not written")
	}
	if comments, _ := dst.ReadActionPlanComments(d.ID); comments["s0.step1"] != "check disk first" {
		t.Errorf("comments = %v", comments)
	}

	if checks, _ := dst.ReadActionPlanChecks(d.ID); len(checks) != 0 {
		t.Errorf("checks = %v, want none: an import has never run", checks)
	}
	if runs, _ := dst.ReadActionPlanRuns(d.ID); len(runs) != 0 {
		t.Errorf("runs = %v, want none", runs)
	}
	if state, _ := dst.ReadPlanState(d.ID); state.Status != ActionPlanStatusDraft {
		t.Errorf("status = %q, want draft", state.Status)
	}
	if _, found, _ := dst.ReadReport(d.ID); found {
		t.Error("report was imported")
	}

	state, found, err := dst.ReadSubagentState(d.ID)
	if err != nil || !found {
		t.Fatalf("ReadSubagentState() found=%v err=%v", found, err)
	}
	if len(state.Pauses) != 1 || state.Pauses[0].DialogID != newDecompose.String() || state.Pauses[0].MainAskID != "subagent_ask_x" {
		t.Fatalf("pauses = %+v, want the pause pointing at the new decompose dialog", state.Pauses)
	}
}

func TestImportPlanCarriesAttachments(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	srcRepo := newMultiDialogRepo()
	root := srcRepo.add(domain.Dialog{Title: "With file", Mode: "main"})
	msg, _ := srcRepo.AppendMessage(ctx, root, domain.DialogMessage{Role: "user", Content: "see attached"})
	attID := uuid.New()
	storedName := attID.String() + "_config.yaml"
	src := newBundleTestService(t, srcRepo, stubAttachmentRepo{byDialog: map[uuid.UUID][]domain.Attachment{
		root: {{ID: attID, DialogID: root, MessageID: &msg.ID, Filename: "config.yaml", ContentType: "text/yaml", Kind: domain.AttachmentKindText, Path: filepath.Join("attachments", root.String(), storedName)}},
	}})
	writeTestFile(t, filepath.Join(src.attachmentsDir, root.String(), storedName), "port: 5432\n")

	b, err := src.ExportPlan(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Chat) != 1 || len(b.Chat[0].Attachments) != 1 || string(b.Chat[0].Attachments[0].Data) != "port: 5432\n" {
		t.Fatalf("Chat = %+v, want the message with its file", b.Chat)
	}

	dstRepo := newMultiDialogRepo()
	dst := newBundleTestService(t, dstRepo, nil)
	importer := &fakeDialogImporter{repo: dstRepo}
	dst.SetDialogImporter(importer)
	d, err := dst.ImportPlan(ctx, b)
	if err != nil {
		t.Fatalf("ImportPlan() error = %v", err)
	}

	atts := importer.got[0].Messages[1].Attachments
	if len(atts) != 1 || atts[0].ID == attID || atts[0].DialogID != d.ID {
		t.Fatalf("attachments = %+v, want one with a fresh id on the new dialog", atts)
	}
	data, err := os.ReadFile(filepath.Join(dst.attachmentsDir, d.ID.String(), filepath.Base(atts[0].Path)))
	if err != nil || string(data) != "port: 5432\n" {
		t.Fatalf("attachment file = %q, err %v", data, err)
	}
}

func TestImportPlanRemovesFilesWhenTheCommitFails(t *testing.T) {
	t.Parallel()

	repo := newMultiDialogRepo()
	svc := newBundleTestService(t, repo, nil)
	svc.SetDialogImporter(&fakeDialogImporter{repo: repo, err: errors.New("db down")})

	b := validTestBundle()
	b.Chat = []PlanBundleMessage{{Role: "user", Content: "hi", Attachments: []PlanBundleAttachment{
		{ID: uuid.New(), Filename: "a.txt", ContentType: "text/plain", Data: []byte("x")},
	}}}
	if _, err := svc.ImportPlan(context.Background(), b); err == nil {
		t.Fatal("ImportPlan() error = nil, want the commit failure")
	}
	entries, _ := os.ReadDir(svc.attachmentsDir)
	if len(entries) != 0 {
		t.Fatalf("attachments dir holds %d entries after a failed import, want 0", len(entries))
	}
}

func validTestBundle() PlanBundle {
	return PlanBundle{
		Format:  PlanBundleFormat,
		Version: PlanBundleVersion,
		Plan:    PlanBundleMeta{ID: uuid.New(), Title: "t", Mode: "main", CreatedAt: time.Now()},
	}
}

func TestValidatePlanBundle(t *testing.T) {
	t.Parallel()

	dup := uuid.New()
	tests := []struct {
		name   string
		mutate func(*PlanBundle)
	}{
		{name: "wrong format", mutate: func(b *PlanBundle) { b.Format = "zip" }},
		{name: "newer version", mutate: func(b *PlanBundle) { b.Version = PlanBundleVersion + 1 }},
		{name: "no plan id", mutate: func(b *PlanBundle) { b.Plan.ID = uuid.Nil }},
		{name: "not a plan mode", mutate: func(b *PlanBundle) { b.Plan.Mode = "discuss" }},
		{name: "action plan not an object", mutate: func(b *PlanBundle) { b.ActionPlan = json.RawMessage(`[1]`) }},
		{name: "contract not an object", mutate: func(b *PlanBundle) { b.Contract = json.RawMessage(`"x"`) }},
		{name: "execute sub-agent", mutate: func(b *PlanBundle) {
			b.Subagents = []PlanBundleDialog{{ID: uuid.New(), Mode: executeDialogMode}}
		}},
		{name: "two decompose sub-agents", mutate: func(b *PlanBundle) {
			b.Subagents = []PlanBundleDialog{{ID: uuid.New(), Mode: "decompose"}, {ID: uuid.New(), Mode: "decompose"}}
		}},
		{name: "sub-agent reuses the plan id", mutate: func(b *PlanBundle) {
			b.Subagents = []PlanBundleDialog{{ID: b.Plan.ID, Mode: "plan"}}
		}},
		{name: "system row", mutate: func(b *PlanBundle) { b.Chat = []PlanBundleMessage{{Role: "system"}} }},
		{name: "tool calls not JSON", mutate: func(b *PlanBundle) {
			b.Chat = []PlanBundleMessage{{Role: "assistant", ToolCalls: json.RawMessage(`{`)}}
		}},
		{name: "empty attachment", mutate: func(b *PlanBundle) {
			b.Chat = []PlanBundleMessage{{Role: "user", Attachments: []PlanBundleAttachment{{ID: uuid.New(), Filename: "a.txt"}}}}
		}},
		{name: "attachment without a name", mutate: func(b *PlanBundle) {
			b.Chat = []PlanBundleMessage{{Role: "user", Attachments: []PlanBundleAttachment{{ID: uuid.New(), Data: []byte("x")}}}}
		}},
		{name: "attachment id used twice", mutate: func(b *PlanBundle) {
			a := PlanBundleAttachment{ID: dup, Filename: "a.txt", Data: []byte("x")}
			b.Chat = []PlanBundleMessage{{Role: "user", Attachments: []PlanBundleAttachment{a, a}}}
		}},
	}

	if err := validatePlanBundle(validTestBundle()); err != nil {
		t.Fatalf("validatePlanBundle(valid) error = %v", err)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b := validTestBundle()
			tt.mutate(&b)
			if err := validatePlanBundle(b); !errors.Is(err, ErrInvalidPlanBundle) {
				t.Fatalf("validatePlanBundle() error = %v, want ErrInvalidPlanBundle", err)
			}
		})
	}
}
