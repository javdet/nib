package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/kbsettings"
	"github.com/javdet/nib/internal/mode"
)

func TestKBAgentAllowSet(t *testing.T) {
	t.Parallel()
	allow := kbAgentAllowSet()

	if len(allow) != 3 {
		t.Fatalf("len(allow) = %d, want 3: the kb agent reads the plan, reads one document and writes it back", len(allow))
	}
	for _, name := range []string{GetActionListToolName, GetKBDocumentToolName, UpdateKBToolName} {
		if _, ok := allow[name]; !ok {
			t.Fatalf("missing %q", name)
		}
	}
	// Naming the ones that would let a knowledge-base merge touch live
	// infrastructure, so a later widening of this set is deliberate.
	for _, name := range []string{
		ExecuteCommandToolName,
		APICallToolName,
		GetSecretsToolName,
		RunExecutorToolName,
		AskQuestionToolName,
		SetReportToolName,
		UpdateActionPlanToolName,
	} {
		if _, ok := allow[name]; ok {
			t.Fatalf("kb agent must not carry %q", name)
		}
	}
}

// TestActionAgentAllowSetWithholdsUpdateKB mirrors the set_report guard: both
// agents run in execute mode, so the mode allow list cannot tell a per-action
// executor from the subagent launched when the plan ends.
func TestActionAgentAllowSetWithholdsUpdateKB(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := mode.SeedAllowLists(dir); err != nil {
		t.Fatalf("seed allow lists: %v", err)
	}
	execList := filepath.Join(dir, "execute.json")
	if err := os.WriteFile(execList, []byte(`{"allow_tools":["get_action_list","update_kb"]}`), 0o644); err != nil {
		t.Fatalf("write execute allow list: %v", err)
	}

	svc := &ChatService{allowToolsDir: dir}
	allow, err := svc.actionAgentAllowSet(context.Background(), nil)
	if err != nil {
		t.Fatalf("actionAgentAllowSet err = %v", err)
	}

	if _, ok := allow[UpdateKBToolName]; ok {
		t.Fatal("action agent must never carry update_kb, even when the execute allow list names it")
	}
	if _, ok := allow[GetActionListToolName]; !ok {
		t.Fatal("action agent lost get_action_list")
	}
}

func TestPlanKBCollection(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		project string
		want    string
		wantOK  bool
	}{
		{name: "project name is the collection", project: "kiss", want: "kiss", wantOK: true},
		{name: "dots and dashes are allowed", project: "kiss-2.infra", want: "kiss-2.infra", wantOK: true},
		{name: "any means no project was chosen", project: selectionAny, wantOK: false},
		{name: "empty", project: "", wantOK: false},
		{name: "whitespace only", project: "   ", wantOK: false},
		{name: "a name a collection cannot take", project: "../escape", wantOK: false},
		{name: "leading dot is not a collection", project: ".hidden", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := planKBCollection(domain.Selection{Project: tt.project})
			if ok != tt.wantOK {
				t.Fatalf("planKBCollection(%q) ok = %v, want %v", tt.project, ok, tt.wantOK)
			}
			if got != tt.want {
				t.Fatalf("planKBCollection(%q) = %q, want %q", tt.project, got, tt.want)
			}
		})
	}
}

// selectionStoreFor is the global header selection a plan without a recorded
// snapshot falls back to.
func selectionStoreFor(project string) *SelectionStore {
	store := NewSelectionStore()
	store.Set(domain.Selection{Project: project})
	return store
}

// kbSettingsStore returns a store over a temp dir with AutoUpdate set.
func kbSettingsStore(t *testing.T, autoUpdate bool) *kbsettings.Store {
	t.Helper()
	store := kbsettings.NewStore(t.TempDir(), "")
	if err := store.Set(kbsettings.Settings{AutoUpdate: autoUpdate}); err != nil {
		t.Fatalf("set knowledge settings: %v", err)
	}
	return store
}

// A nil llm provider and a nil knowledge store would panic if a run started, so
// StartKBUpdateAgent returning cleanly is itself the assertion that it did not.
func TestStartKBUpdateAgentSkips(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		build   func(t *testing.T, planID uuid.UUID) *ChatService
		wantErr bool
	}{
		{
			name: "auto-update is off",
			build: func(t *testing.T, _ uuid.UUID) *ChatService {
				return &ChatService{
					dialogRepo:       &actionListDialogRepo{},
					knowledgeSvc:     &KnowledgeService{},
					kbSettings:       kbSettingsStore(t, false),
					kbUpdatesDir:     t.TempDir(),
					planSelectionDir: t.TempDir(),
					selection:        selectionStoreFor("kiss"),
				}
			},
		},
		{
			name: "the settings store is not wired",
			build: func(t *testing.T, _ uuid.UUID) *ChatService {
				return &ChatService{
					dialogRepo:       &actionListDialogRepo{},
					knowledgeSvc:     &KnowledgeService{},
					kbUpdatesDir:     t.TempDir(),
					planSelectionDir: t.TempDir(),
					selection:        selectionStoreFor("kiss"),
				}
			},
		},
		{
			name: "the plan has already been folded in",
			build: func(t *testing.T, planID uuid.UUID) *ChatService {
				svc := &ChatService{
					dialogRepo:       &actionListDialogRepo{},
					knowledgeSvc:     &KnowledgeService{},
					kbSettings:       kbSettingsStore(t, true),
					kbUpdatesDir:     t.TempDir(),
					planSelectionDir: t.TempDir(),
					selection:        selectionStoreFor("kiss"),
				}
				if err := svc.writeKBUpdate(planID, kbUpdate{Collection: "kiss", Changed: true}); err != nil {
					t.Fatalf("writeKBUpdate err = %v", err)
				}
				return svc
			},
		},
		{
			name: "no project was selected",
			build: func(t *testing.T, _ uuid.UUID) *ChatService {
				return &ChatService{
					dialogRepo:       &actionListDialogRepo{},
					knowledgeSvc:     &KnowledgeService{},
					kbSettings:       kbSettingsStore(t, true),
					kbUpdatesDir:     t.TempDir(),
					planSelectionDir: t.TempDir(),
					selection:        selectionStoreFor(selectionAny),
				}
			},
		},
		{
			name: "the knowledge service is not wired",
			build: func(t *testing.T, _ uuid.UUID) *ChatService {
				return &ChatService{
					dialogRepo:       &actionListDialogRepo{},
					kbSettings:       kbSettingsStore(t, true),
					kbUpdatesDir:     t.TempDir(),
					planSelectionDir: t.TempDir(),
					selection:        selectionStoreFor("kiss"),
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			planID := uuid.New()
			svc := tt.build(t, planID)
			if err := svc.StartKBUpdateAgent(context.Background(), planID); err != nil {
				t.Fatalf("StartKBUpdateAgent err = %v", err)
			}
		})
	}
}

func TestStartKBUpdateAgentWithoutADialogRepo(t *testing.T) {
	t.Parallel()

	svc := &ChatService{kbUpdatesDir: t.TempDir()}
	if err := svc.StartKBUpdateAgent(context.Background(), uuid.New()); err == nil {
		t.Fatal("expected an error when the dialog repository is not configured")
	}
}

// TestKBUpdateRoundTrip covers the marker that stops a second press of Finish
// re-running the merge.
func TestKBUpdateRoundTrip(t *testing.T) {
	t.Parallel()

	svc := &ChatService{kbUpdatesDir: t.TempDir()}
	planID := uuid.New()

	if _, found, err := svc.ReadKBUpdate(planID); err != nil || found {
		t.Fatalf("ReadKBUpdate before write = found %v, err %v", found, err)
	}

	want := kbUpdate{Collection: "kiss", Changed: true, UpdatedAt: "2026-09-22T00:00:00Z"}
	if err := svc.writeKBUpdate(planID, want); err != nil {
		t.Fatalf("writeKBUpdate err = %v", err)
	}

	got, found, err := svc.ReadKBUpdate(planID)
	if err != nil {
		t.Fatalf("ReadKBUpdate err = %v", err)
	}
	if !found {
		t.Fatal("ReadKBUpdate found = false after a write")
	}
	if got != want {
		t.Fatalf("ReadKBUpdate = %+v, want %+v", got, want)
	}
}

func TestBuildKBUpdateSeed(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	svc := &ChatService{
		summariesDir:     t.TempDir(),
		dagsDir:          t.TempDir(),
		reportsDir:       t.TempDir(),
		planContractsDir: t.TempDir(),
	}
	if err := svc.WriteSummary(rootID, "Move the API behind the new ingress."); err != nil {
		t.Fatalf("WriteSummary err = %v", err)
	}
	if err := svc.WriteReport(rootID, "## Outcome\n\nThe ingress now serves api.example.com."); err != nil {
		t.Fatalf("WriteReport err = %v", err)
	}

	seed, err := svc.buildKBUpdateSeed(rootID, "kiss")
	if err != nil {
		t.Fatalf("buildKBUpdateSeed err = %v", err)
	}

	for _, want := range []string{
		"Move the API behind the new ingress.",
		"## Plan report",
		"The ingress now serves api.example.com.",
		"## Your task",
		"`kiss`",
	} {
		if !strings.Contains(seed, want) {
			t.Fatalf("seed is missing %q:\n%s", want, seed)
		}
	}
}

// TestBuildKBUpdateSeedWithoutAReport covers the plan whose report agent failed:
// the merge still has the summary and get_action_list to work from.
func TestBuildKBUpdateSeedWithoutAReport(t *testing.T) {
	t.Parallel()

	rootID := uuid.New()
	svc := &ChatService{
		summariesDir:     t.TempDir(),
		dagsDir:          t.TempDir(),
		reportsDir:       t.TempDir(),
		planContractsDir: t.TempDir(),
	}

	seed, err := svc.buildKBUpdateSeed(rootID, "kiss")
	if err != nil {
		t.Fatalf("buildKBUpdateSeed err = %v", err)
	}
	if strings.Contains(seed, "## Plan report") {
		t.Fatalf("seed claims a report that was never written:\n%s", seed)
	}
	if !strings.Contains(seed, "## Your task") {
		t.Fatalf("seed is missing the task section:\n%s", seed)
	}
}

// TestKBCollectionLockSerialises is the guard on update_kb being a full
// overwrite: two plans finishing against one collection must not read the same
// document and then each write their own merge of it.
func TestKBCollectionLockSerialises(t *testing.T) {
	t.Parallel()

	svc := &ChatService{}
	sem := svc.kbCollectionLock("kiss")
	sem <- struct{}{}

	acquired := make(chan struct{})
	go func() {
		second := svc.kbCollectionLock("kiss")
		second <- struct{}{}
		close(acquired)
	}()

	select {
	case <-acquired:
		t.Fatal("a second holder took the collection while the first still had it")
	case <-time.After(50 * time.Millisecond):
	}

	<-sem
	select {
	case <-acquired:
	case <-time.After(time.Second):
		t.Fatal("the waiter never took the collection after it was released")
	}

	// A different collection is a different lock, so unrelated plans do not
	// queue behind each other.
	other := svc.kbCollectionLock("therapylog")
	select {
	case other <- struct{}{}:
	default:
		t.Fatal("a different collection blocked on an unrelated one")
	}
}

func TestKBCollectionLockIsStablePerCollection(t *testing.T) {
	t.Parallel()

	svc := &ChatService{}
	var wg sync.WaitGroup
	locks := make([]chan struct{}, 8)
	for i := range locks {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			locks[i] = svc.kbCollectionLock("kiss")
		}(i)
	}
	wg.Wait()

	for i, l := range locks {
		if l != locks[0] {
			t.Fatalf("lock %d differs from lock 0: concurrent callers got different semaphores", i)
		}
	}
}
