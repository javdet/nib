package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/kbdoc"
	"github.com/javdet/nib/internal/kbsettings"
)

const (
	// kbAgentMaxIterations is a little above the report agent's: this one reads
	// two documents and writes one, and a large knowledge base can take a couple
	// of rounds to merge.
	kbAgentMaxIterations = 12
	kbAgentTimeout       = 15 * time.Minute
	// kbPromptName is the overlay appended to execute.md for the subagent that
	// folds a finished plan back into its project's knowledge base.
	kbPromptName = "execute_kb"
	// kbSubagentName keys the launch claim. Like the report agent's it is not an
	// entry in subagentSpecs: no orchestrator launches this one.
	kbSubagentName SubagentName = "kb"
	kbDialogTitle               = "Knowledge base update"
)

// SetKBSettings wires the knowledge-base switches. Optional the same way the
// stats writer is: a nil store leaves auto-update off.
func (s *ChatService) SetKBSettings(store *kbsettings.Store) {
	s.kbSettings = store
}

// kbUpdate records that a finished plan has been folded into its collection.
type kbUpdate struct {
	Collection string `json:"collection"`
	// Changed is false when the agent read the document and found nothing the
	// plan had changed, which is a real outcome rather than a failure.
	Changed   bool   `json:"changed"`
	UpdatedAt string `json:"updatedAt"`
}

func kbUpdatePath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".json")
}

// ReadKBUpdate returns the record of a plan's knowledge-base update.
func (s *ChatService) ReadKBUpdate(dialogID uuid.UUID) (kbUpdate, bool, error) {
	b, err := os.ReadFile(kbUpdatePath(s.kbUpdatesDir, dialogID))
	if errors.Is(err, os.ErrNotExist) {
		return kbUpdate{}, false, nil
	}
	if err != nil {
		return kbUpdate{}, false, fmt.Errorf("read kb update: %w", err)
	}

	var rec kbUpdate
	if err := json.Unmarshal(b, &rec); err != nil {
		return kbUpdate{}, false, fmt.Errorf("unmarshal kb update: %w", err)
	}
	return rec, true, nil
}

func (s *ChatService) writeKBUpdate(dialogID uuid.UUID, rec kbUpdate) error {
	if err := os.MkdirAll(s.kbUpdatesDir, 0o755); err != nil {
		return fmt.Errorf("create kb updates directory: %w", err)
	}

	b, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal kb update: %w", err)
	}
	if err := atomicfile.Write(kbUpdatePath(s.kbUpdatesDir, dialogID), b); err != nil {
		return fmt.Errorf("write kb update: %w", err)
	}
	return nil
}

// planKBCollection maps a plan's selection to the collection its knowledge lives
// in. The collection name is the project name, which is the convention the
// Knowledge Base page already follows.
//
// There is deliberately no fallback to the default collection: update_kb is a
// full overwrite, and overwriting "default" because the plan did not name a
// project is the same mistake ExecuteUpdateKB refuses to make.
func planKBCollection(sel domain.Selection) (string, bool) {
	name := strings.TrimSpace(sel.Project)
	if name == "" || name == selectionAny {
		return "", false
	}
	if !kbdoc.ValidName(name) {
		return "", false
	}
	return name, true
}

// StartKBUpdateAgent folds a finished plan's outcome back into its project's
// knowledge base, in a subagent of its own, and returns as soon as that run is
// on its way.
//
// Like the report agent it does not take the ExecutionLease: it reads a finished
// plan and rewrites nib's own knowledge base, and changes no managed system. It
// does take a lock on the collection, which the report agent has no equivalent
// of -- see runKBUpdateAgent.
func (s *ChatService) StartKBUpdateAgent(ctx context.Context, planID uuid.UUID) error {
	if s.dialogRepo == nil {
		return fmt.Errorf("start kb update agent: dialog repository is not configured")
	}
	if s.knowledgeSvc == nil || s.kbSettings == nil {
		return nil
	}

	settings, err := s.kbSettings.Get()
	if err != nil {
		return fmt.Errorf("start kb update agent: read settings: %w", err)
	}
	if !settings.AutoUpdate {
		return nil
	}

	rootID, err := s.resolveRootDialogID(ctx, planID)
	if err != nil {
		return fmt.Errorf("start kb update agent: %w", err)
	}

	// Written once per plan. A failed run writes no record, so it stays
	// retryable; a run that decided nothing needed changing writes one, so it
	// does not repeat that decision on every press of Finish.
	if _, found, err := s.ReadKBUpdate(rootID); err != nil {
		return fmt.Errorf("start kb update agent: %w", err)
	} else if found {
		return nil
	}

	collection, ok := planKBCollection(s.planSelection(rootID))
	if !ok {
		slog.Info("kb update agent: plan has no knowledge base collection", "plan_id", rootID)
		return nil
	}

	claim := s.subagentClaim(rootID, kbSubagentName)
	if !claim.tryLock() {
		return nil
	}

	// The run outlives whatever started it -- an HTTP request, or the report
	// agent's own context, which is cancelled the moment that run returns.
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), kbAgentTimeout)
	go func() {
		defer cancel()
		defer claim.unlock()
		s.runKBUpdateAgent(runCtx, rootID, collection)
	}()

	return nil
}

// kbCollectionLock returns the one-slot semaphore guarding a collection.
func (s *ChatService) kbCollectionLock(collection string) chan struct{} {
	v, _ := s.kbCollectionLocks.LoadOrStore(collection, make(chan struct{}, 1))
	return v.(chan struct{})
}

// runKBUpdateAgent merges a finished plan into its collection in a dialog of its
// own. Every failure is logged and goes no further: the operator pressed Finish,
// that succeeded, and nobody is waiting on this.
func (s *ChatService) runKBUpdateAgent(ctx context.Context, rootID uuid.UUID, collection string) {
	// update_kb replaces a collection wholesale, so the read, the merge and the
	// write have to be one critical section: two plans finishing against one
	// project would otherwise each write a document built from the text before
	// the other started. Waiting is free here -- no request is held open -- so
	// this blocks rather than dropping the second update.
	sem := s.kbCollectionLock(collection)
	select {
	case sem <- struct{}{}:
		defer func() { <-sem }()
	case <-ctx.Done():
		slog.Warn("kb update agent: gave up waiting for the collection",
			"plan_id", rootID, "collection", collection)
		return
	}

	before, err := s.kbDocumentFingerprint(ctx, collection)
	if err != nil {
		slog.Error("kb update agent: read document", "plan_id", rootID, "collection", collection, "error", err)
		return
	}

	dialog, err := s.dialogRepo.CreateDialog(ctx, executeDialogMode, kbDialogTitle, &rootID)
	if err != nil {
		slog.Error("kb update agent: create dialog", "plan_id", rootID, "error", err)
		return
	}

	sysPrompt, err := s.subagentSystemPrompt(ctx, executeDialogMode, kbPromptName)
	if err != nil {
		slog.Error("kb update agent: resolve prompt", "plan_id", rootID, "error", err)
		return
	}
	seed, err := s.buildKBUpdateSeed(rootID, collection)
	if err != nil {
		slog.Error("kb update agent: build seed", "plan_id", rootID, "error", err)
		return
	}

	if _, err := s.dialogRepo.AppendMessage(ctx, dialog.ID, domain.DialogMessage{
		Role:    "system",
		Content: sysPrompt,
	}); err != nil {
		slog.Error("kb update agent: append system", "plan_id", rootID, "error", err)
		return
	}
	if _, err := s.dialogRepo.AppendMessage(ctx, dialog.ID, domain.DialogMessage{
		Role:    "user",
		Content: seed,
	}); err != nil {
		slog.Error("kb update agent: append seed", "plan_id", rootID, "error", err)
		return
	}

	catalog, err := s.buildToolCatalog(ctx, kbAgentAllowSet(), toolBinding{
		dialogID: dialog.ID,
		planID:   rootID,
		mode:     executeDialogMode,
	})
	if err != nil {
		slog.Error("kb update agent: build catalog", "plan_id", rootID, "error", err)
		return
	}

	if _, err := s.runPersistingAgentLoop(ctx, dialog.ID, executeDialogMode, catalog, loopConfig{
		planID:        rootID,
		maxIterations: kbAgentMaxIterations,
	}); err != nil {
		slog.Error("kb update agent failed", "plan_id", rootID, "dialog_id", dialog.ID, "error", err)
		return
	}

	after, err := s.kbDocumentFingerprint(ctx, collection)
	if err != nil {
		slog.Error("kb update agent: reread document", "plan_id", rootID, "collection", collection, "error", err)
		return
	}
	changed := before != after
	if !changed {
		// Either the agent judged the plan taught the knowledge base nothing, or
		// it ended its turn without making the call. Only the log can tell them
		// apart, and neither is worth retrying.
		slog.Warn("kb update agent left the document unchanged",
			"plan_id", rootID, "dialog_id", dialog.ID, "collection", collection)
	}

	if err := s.writeKBUpdate(rootID, kbUpdate{
		Collection: collection,
		Changed:    changed,
		UpdatedAt:  time.Now().UTC().Format(time.RFC3339),
	}); err != nil {
		slog.Error("kb update agent: record update", "plan_id", rootID, "error", err)
	}
}

// kbDocumentFingerprint hashes a collection's current source document, so a run
// can be told apart from one that wrote nothing. The content is hashed rather
// than kept, because a knowledge base document is large and only the comparison
// matters.
func (s *ChatService) kbDocumentFingerprint(ctx context.Context, collection string) (string, error) {
	doc, err := s.knowledgeSvc.GetDocument(ctx, collection)
	if err != nil {
		return "", fmt.Errorf("get knowledge document: %w", err)
	}
	sum := sha256.Sum256([]byte(doc.Content))
	return string(doc.Source) + ":" + hex.EncodeToString(sum[:]), nil
}

// kbAgentAllowSet is written out here rather than resolved from the execute mode
// list, for the reason reportAgentAllowSet gives: this agent reads a finished
// plan and rewrites one document, and every other tool the execute list carries
// is a way for that to have side effects on live infrastructure. Keeping the set
// literal also means an operator's edit to data/tools/execute.json cannot widen
// it.
func kbAgentAllowSet() map[string]struct{} {
	return map[string]struct{}{
		GetActionListToolName: {},
		GetKBDocumentToolName: {},
		UpdateKBToolName:      {},
	}
}

// buildKBUpdateSeed is the first user message of the knowledge-base subagent.
//
// The plan report is inlined rather than pulled with a tool: it is already the
// deduplicated account of what the plan did, it is bounded, and there is no tool
// that serves it. The action list stays behind get_action_list, the same way the
// report agent takes it, for the runs the report glossed over.
func (s *ChatService) buildKBUpdateSeed(rootID uuid.UUID, collection string) (string, error) {
	parts, _, err := s.sharedSeedSections(rootID)
	if err != nil {
		return "", err
	}

	report, found, err := s.ReadReport(rootID)
	if err != nil {
		return "", fmt.Errorf("read report: %w", err)
	}
	if found && strings.TrimSpace(report) != "" {
		parts = append(parts, "## Plan report\n\n"+strings.TrimSpace(report))
	}

	parts = append(parts, strings.TrimRight(`## Your task

This plan is finished. Fold what it established into the knowledge base collection
`+"`"+collection+"`"+`, and change nothing else.`, "\n"))

	return strings.Join(parts, "\n\n"), nil
}
