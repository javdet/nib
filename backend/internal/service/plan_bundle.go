package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/repository"
	"github.com/javdet/nib/internal/version"
)

const (
	// PlanBundleFormat and PlanBundleVersion identify a .nib file. The version
	// is bumped only when an older importer could not read a newer file
	// correctly; a field it would merely ignore does not need a bump.
	PlanBundleFormat  = "nib-plan"
	PlanBundleVersion = 1
)

var (
	// ErrNotAPlan is returned when a dialog that is not the root of a plan is
	// asked to be exported.
	ErrNotAPlan = errors.New("dialog is not a plan")
	// ErrInvalidPlanBundle is returned when an imported .nib file is not one
	// this version can read.
	ErrInvalidPlanBundle = errors.New("invalid plan file")
)

// PlanBundle is a plan as a single portable document: what it is, how it was
// decomposed and planned, and every conversation that produced it.
//
// What it leaves out is everything that describes a run rather than the plan:
// checkboxes, execution records and notes, stage runs, the fan-out record,
// plan status and schedule, the report, knowledge-base updates, code fixes, and
// the execute sub-agents' transcripts. An imported plan starts as a draft
// nobody has run. The system prompts are left out too: they are rendered from
// the exporting install's variables and host, so the importer renders its own.
type PlanBundle struct {
	Format     string    `json:"format"`
	Version    int       `json:"version"`
	ExportedAt time.Time `json:"exportedAt"`
	NibVersion string    `json:"nibVersion,omitempty"`

	Plan PlanBundleMeta `json:"plan"`

	Summary    string            `json:"summary,omitempty"`
	DAG        string            `json:"dag,omitempty"`
	Contract   json.RawMessage   `json:"contract,omitempty"`
	ActionPlan json.RawMessage   `json:"actionPlan,omitempty"`
	Comments   map[string]string `json:"comments,omitempty"`

	// Chat is the plan's own transcript, the one the operator talks to.
	Chat []PlanBundleMessage `json:"chat"`
	// Subagents are the decompose and plan sub-agents' transcripts.
	Subagents []PlanBundleDialog `json:"subagents,omitempty"`
	// Pauses are the decompose questions still waiting for an answer, so one
	// asked before the export can be answered after the import.
	Pauses []SubagentPause `json:"pauses,omitempty"`
}

// PlanBundleMeta describes the plan's root dialog. ID is the exporting
// install's id; an import mints a new one and rewrites every reference to it.
type PlanBundleMeta struct {
	ID         uuid.UUID `json:"id"`
	Title      string    `json:"title"`
	Mode       string    `json:"mode"`
	Categories []string  `json:"categories,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	// Selection is the project/environment/cloud/location the plan was made
	// under, kept for whoever reads the file. An import does not apply it:
	// those names belong to the exporting install.
	Selection *domain.Selection `json:"selection,omitempty"`
}

// PlanBundleDialog is one sub-agent transcript.
type PlanBundleDialog struct {
	ID        uuid.UUID           `json:"id"`
	Mode      string              `json:"mode"`
	Title     string              `json:"title"`
	CreatedAt time.Time           `json:"createdAt"`
	Messages  []PlanBundleMessage `json:"messages"`
}

// PlanBundleMessage is one transcript row, system rows excluded.
type PlanBundleMessage struct {
	Role        string                 `json:"role"`
	Content     string                 `json:"content"`
	ToolCalls   json.RawMessage        `json:"toolCalls,omitempty"`
	ToolCallID  string                 `json:"toolCallId,omitempty"`
	Name        string                 `json:"name,omitempty"`
	CreatedAt   time.Time              `json:"createdAt"`
	Attachments []PlanBundleAttachment `json:"attachments,omitempty"`
}

// PlanBundleAttachment is a file attached to a message. Data is base64 in the
// JSON, which is what encoding/json does with a byte slice.
type PlanBundleAttachment struct {
	ID          uuid.UUID `json:"id"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"contentType"`
	Data        []byte    `json:"data"`
}

// SetDialogImporter wires the transactional writer ImportPlan needs. Without it
// an import is refused rather than written row by row.
func (s *ChatService) SetDialogImporter(imp repository.DialogImporter) {
	s.dialogImporter = imp
}

// isBundledSubagentMode reports whether a child dialog in this mode belongs in
// a .nib file. decompose and plan built the plan; execute ran it.
func isBundledSubagentMode(m string) bool {
	return m == decomposeSubagentMode || m == stagePlanMode
}

// ExportPlan collects a plan into a PlanBundle.
func (s *ChatService) ExportPlan(ctx context.Context, rootID uuid.UUID) (PlanBundle, error) {
	root, err := s.dialogRepo.GetDialog(ctx, rootID)
	if err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: %w", err)
	}
	if root.ParentID != nil || !mode.CarriesPlan(root.Mode) {
		return PlanBundle{}, fmt.Errorf("export plan %s: %w", rootID, ErrNotAPlan)
	}

	b := PlanBundle{
		Format:     PlanBundleFormat,
		Version:    PlanBundleVersion,
		ExportedAt: time.Now().UTC(),
		NibVersion: version.Version(),
		Plan: PlanBundleMeta{
			ID:         root.ID,
			Title:      root.Title,
			Mode:       root.Mode,
			Categories: root.Categories,
			CreatedAt:  root.CreatedAt,
		},
	}
	if sel, found, err := s.ReadPlanSelection(rootID); err == nil && found {
		b.Plan.Selection = &sel
	}

	if b.Summary, _, err = s.ReadSummary(rootID); err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: read summary: %w", err)
	}
	if b.DAG, _, err = s.ReadDAG(rootID); err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: read dag: %w", err)
	}
	if b.Contract, err = s.readPlanContractRaw(rootID); err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: read contract: %w", err)
	}
	plan, found, err := s.ReadActionPlan(rootID)
	if err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: read action plan: %w", err)
	}
	if found {
		b.ActionPlan = plan
		comments, err := s.ReadActionPlanComments(rootID)
		if err != nil {
			return PlanBundle{}, fmt.Errorf("export plan: read comments: %w", err)
		}
		if len(comments) > 0 {
			b.Comments = comments
		}
	}

	if b.Chat, err = s.exportTranscript(ctx, rootID); err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: %w", err)
	}

	children, err := s.dialogRepo.ListChildren(ctx, rootID)
	if err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: list sub-agents: %w", err)
	}
	bundled := make(map[string]struct{}, len(children))
	for _, child := range children {
		if !isBundledSubagentMode(child.Mode) {
			continue
		}
		msgs, err := s.exportTranscript(ctx, child.ID)
		if err != nil {
			return PlanBundle{}, fmt.Errorf("export plan: %w", err)
		}
		b.Subagents = append(b.Subagents, PlanBundleDialog{
			ID:        child.ID,
			Mode:      child.Mode,
			Title:     child.Title,
			CreatedAt: child.CreatedAt,
			Messages:  msgs,
		})
		bundled[child.ID.String()] = struct{}{}
	}

	state, _, err := s.ReadSubagentState(rootID)
	if err != nil {
		return PlanBundle{}, fmt.Errorf("export plan: %w", err)
	}
	for _, p := range state.Pauses {
		if _, ok := bundled[p.DialogID]; ok {
			b.Pauses = append(b.Pauses, p)
		}
	}

	return b, nil
}

// readPlanContractRaw returns the stored contract as written, so fields this
// version does not model survive the trip.
func (s *ChatService) readPlanContractRaw(rootID uuid.UUID) (json.RawMessage, error) {
	b, err := os.ReadFile(filepath.Join(s.planContractsDir, rootID.String()+".json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return json.RawMessage(b), nil
}

func (s *ChatService) exportTranscript(ctx context.Context, dialogID uuid.UUID) ([]PlanBundleMessage, error) {
	msgs, err := s.dialogRepo.ListMessages(ctx, dialogID)
	if err != nil {
		return nil, fmt.Errorf("list messages of %s: %w", dialogID, err)
	}

	var byMessage map[int64][]domain.Attachment
	if s.attachmentRepo != nil {
		attachments, err := s.attachmentRepo.ListAttachmentsByDialog(ctx, dialogID)
		if err != nil {
			return nil, fmt.Errorf("list attachments of %s: %w", dialogID, err)
		}
		byMessage = groupAttachmentsByMessage(attachments)
	}

	out := make([]PlanBundleMessage, 0, len(msgs))
	for _, m := range msgs {
		if m.Role == "system" {
			continue
		}
		pm := PlanBundleMessage{
			Role:       m.Role,
			Content:    m.Content,
			ToolCalls:  m.ToolCalls,
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
			CreatedAt:  m.CreatedAt,
		}
		for _, a := range byMessage[m.ID] {
			data, err := os.ReadFile(filepath.Join(s.attachmentsDir, dialogID.String(), filepath.Base(a.Path)))
			if err != nil {
				// A file the sweeper already took cannot be exported; the message
				// still can, and losing the whole export over it would be worse.
				slog.Warn("export plan: skip unreadable attachment", "dialog_id", dialogID, "attachment_id", a.ID, "error", err)
				continue
			}
			pm.Attachments = append(pm.Attachments, PlanBundleAttachment{
				ID:          a.ID,
				Filename:    a.Filename,
				ContentType: a.ContentType,
				Data:        data,
			})
		}
		out = append(out, pm)
	}
	return out, nil
}

// ImportPlan creates a new plan from a bundle and returns its root dialog.
//
// Every dialog and attachment gets a fresh id, and every occurrence of an old
// id in the imported text is rewritten to the new one. A transcript refers to
// its plan by id (tool results name artifact paths, pauses name the sub-agent's
// dialog), and a plain textual rewrite is safe because a UUID never occurs by
// accident.
func (s *ChatService) ImportPlan(ctx context.Context, b PlanBundle) (domain.Dialog, error) {
	if s.dialogImporter == nil {
		return domain.Dialog{}, fmt.Errorf("import plan: dialog importer is not configured")
	}
	if err := validatePlanBundle(b); err != nil {
		return domain.Dialog{}, err
	}

	rootID := uuid.New()
	pairs := []string{b.Plan.ID.String(), rootID.String()}
	childIDs := make([]uuid.UUID, len(b.Subagents))
	for i, sub := range b.Subagents {
		childIDs[i] = uuid.New()
		pairs = append(pairs, sub.ID.String(), childIDs[i].String())
	}
	// Attachment ids are rewritten as well: their URLs embed them, and a URL
	// that survived with the old id would name another install's file.
	attachmentIDs := make(map[uuid.UUID]uuid.UUID)
	for _, msgs := range bundleTranscripts(b) {
		for _, m := range msgs {
			for _, a := range m.Attachments {
				if _, seen := attachmentIDs[a.ID]; !seen {
					attachmentIDs[a.ID] = uuid.New()
					pairs = append(pairs, a.ID.String(), attachmentIDs[a.ID].String())
				}
			}
		}
	}
	rewrite := strings.NewReplacer(pairs...).Replace

	categories, err := normalizeTagList(b.Plan.Categories)
	if err != nil {
		return domain.Dialog{}, fmt.Errorf("%w: categories: %w", ErrInvalidPlanBundle, err)
	}

	var written []string
	cleanup := func() {
		for _, dir := range written {
			_ = os.RemoveAll(dir)
		}
	}

	rootMode := strings.TrimSpace(b.Plan.Mode)
	rootMsgs, dirs, err := s.importTranscript(ctx, rootID, b.Chat, attachmentIDs, rewrite, func(ctx context.Context) (string, error) {
		return s.resolveSystemPrompt(ctx, rootMode)
	})
	written = append(written, dirs...)
	if err != nil {
		cleanup()
		return domain.Dialog{}, err
	}
	dialogs := []domain.DialogImport{{
		Dialog: domain.Dialog{
			ID:         rootID,
			Title:      strings.TrimSpace(b.Plan.Title),
			Mode:       rootMode,
			Categories: categories,
			CreatedAt:  b.Plan.CreatedAt,
		},
		Messages: rootMsgs,
	}}

	for i, sub := range b.Subagents {
		msgs, dirs, err := s.importTranscript(ctx, childIDs[i], sub.Messages, attachmentIDs, rewrite, func(ctx context.Context) (string, error) {
			return s.subagentImportPrompt(ctx, sub)
		})
		written = append(written, dirs...)
		if err != nil {
			cleanup()
			return domain.Dialog{}, err
		}
		parent := rootID
		dialogs = append(dialogs, domain.DialogImport{
			Dialog: domain.Dialog{
				ID:        childIDs[i],
				Title:     rewrite(sub.Title),
				Mode:      sub.Mode,
				ParentID:  &parent,
				CreatedAt: sub.CreatedAt,
			},
			Messages: msgs,
		})
	}

	if err := s.dialogImporter.ImportDialogs(ctx, dialogs); err != nil {
		cleanup()
		return domain.Dialog{}, fmt.Errorf("import plan: %w", err)
	}

	if err := s.writeImportedArtifacts(rootID, b, rewrite, len(rootMsgs) > 0); err != nil {
		// The rows are committed, so the plan is deleted rather than left
		// looking complete without its DAG or action list. The cascade queues
		// the attachment files for the sweeper; removing them here is the fast
		// path.
		if delErr := s.dialogRepo.DeleteDialog(ctx, rootID); delErr != nil {
			slog.Error("import plan: remove partial plan", "dialog_id", rootID, "error", delErr)
		}
		s.removeImportedArtifacts(rootID)
		cleanup()
		return domain.Dialog{}, fmt.Errorf("import plan: %w", err)
	}

	d, err := s.dialogRepo.GetDialog(ctx, rootID)
	if err != nil {
		return domain.Dialog{}, fmt.Errorf("import plan: %w", err)
	}
	slog.Info("plan imported", "dialog_id", rootID, "source_id", b.Plan.ID, "subagents", len(b.Subagents))
	return d, nil
}

func bundleTranscripts(b PlanBundle) [][]PlanBundleMessage {
	out := [][]PlanBundleMessage{b.Chat}
	for _, sub := range b.Subagents {
		out = append(out, sub.Messages)
	}
	return out
}

// subagentImportPrompt renders the system prompt the sub-agent would have been
// started with on this install. Only the decompose transcript is ever resumed;
// the planners' are rendered all the same so no transcript carries the
// exporting install's variables.
func (s *ChatService) subagentImportPrompt(ctx context.Context, sub PlanBundleDialog) (string, error) {
	switch {
	case sub.Mode == decomposeSubagentMode:
		return s.subagentSystemPrompt(ctx, decomposeSubagentMode, decomposeSubagentPromptName)
	case sub.Title == rollbackStageTitle:
		return s.fanoutSystemPrompt(ctx, rollbackPromptName)
	default:
		return s.fanoutSystemPrompt(ctx, stagePromptName)
	}
}

// importTranscript turns bundle messages into rows for dialogID, prefixed with a
// freshly rendered system prompt, and writes their attachment files. It
// returns the directories it wrote so a failed import can remove them.
func (s *ChatService) importTranscript(
	ctx context.Context,
	dialogID uuid.UUID,
	msgs []PlanBundleMessage,
	attachmentIDs map[uuid.UUID]uuid.UUID,
	rewrite func(string) string,
	systemPrompt func(context.Context) (string, error),
) ([]domain.DialogMessage, []string, error) {
	// An empty transcript stays empty: SendInDialog renders the prompt on the
	// first message, exactly as for a plan created here.
	if len(msgs) == 0 {
		return nil, nil, nil
	}

	sys, err := systemPrompt(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("import plan: render system prompt: %w", err)
	}
	out := make([]domain.DialogMessage, 0, len(msgs)+1)
	out = append(out, domain.DialogMessage{Role: "system", Content: sys})

	var dirs []string
	for _, m := range msgs {
		row := domain.DialogMessage{
			Role:       m.Role,
			Content:    rewrite(m.Content),
			ToolCallID: m.ToolCallID,
			Name:       m.Name,
			CreatedAt:  m.CreatedAt,
		}
		if len(m.ToolCalls) > 0 {
			row.ToolCalls = json.RawMessage(rewrite(string(m.ToolCalls)))
		}

		for _, a := range m.Attachments {
			filename := sanitizeAttachmentFilename(a.Filename)
			kind, ct, err := classifyAttachment(filename, a.ContentType)
			if err != nil {
				return nil, dirs, fmt.Errorf("%w: attachment %q: %w", ErrInvalidPlanBundle, a.Filename, err)
			}
			id := attachmentIDs[a.ID]
			dir := filepath.Join(s.attachmentsDir, dialogID.String())
			if len(dirs) == 0 {
				if err := os.MkdirAll(dir, 0o755); err != nil {
					return nil, dirs, fmt.Errorf("import plan: create attachment directory: %w", err)
				}
				dirs = append(dirs, dir)
			}
			storedName := id.String() + "_" + filename
			if err := writeAttachmentFile(filepath.Join(dir, storedName), a.Data); err != nil {
				return nil, dirs, fmt.Errorf("import plan: write attachment: %w", err)
			}
			row.Attachments = append(row.Attachments, domain.Attachment{
				ID:          id,
				DialogID:    dialogID,
				Filename:    filename,
				ContentType: ct,
				Kind:        kind,
				SizeBytes:   int64(len(a.Data)),
				Path:        filepath.Join("attachments", dialogID.String(), storedName),
			})
		}
		out = append(out, row)
	}
	return out, dirs, nil
}

// writeImportedArtifacts writes the plan's files under its new id. The plan
// selection is this install's current one, recorded the same moment the root
// prompt that bakes it in was rendered -- as SendInDialog does for a new plan.
func (s *ChatService) writeImportedArtifacts(rootID uuid.UUID, b PlanBundle, rewrite func(string) string, promptRendered bool) error {
	if b.Summary != "" {
		if err := s.WriteSummary(rootID, rewrite(b.Summary)); err != nil {
			return fmt.Errorf("write summary: %w", err)
		}
	}
	if b.DAG != "" {
		if err := os.MkdirAll(s.dagsDir, 0o755); err != nil {
			return fmt.Errorf("create dags directory: %w", err)
		}
		if err := writeDAGFile(filepath.Join(s.dagsDir, rootID.String()+".md"), rewrite(b.DAG)); err != nil {
			return fmt.Errorf("write dag: %w", err)
		}
	}
	if len(b.Contract) > 0 {
		if err := os.MkdirAll(s.planContractsDir, 0o755); err != nil {
			return fmt.Errorf("create plan_contracts directory: %w", err)
		}
		if err := writeActionPlanFile(filepath.Join(s.planContractsDir, rootID.String()+".json"), []byte(rewrite(string(b.Contract)))); err != nil {
			return fmt.Errorf("write contract: %w", err)
		}
	}
	if len(b.ActionPlan) > 0 {
		if _, err := s.WriteActionPlan(rootID, json.RawMessage(rewrite(string(b.ActionPlan)))); err != nil {
			return fmt.Errorf("write action plan: %w", err)
		}
		if len(b.Comments) > 0 {
			comments := make(map[string]string, len(b.Comments))
			for k, v := range b.Comments {
				comments[k] = rewrite(v)
			}
			if err := s.WriteActionPlanComments(rootID, comments); err != nil {
				return fmt.Errorf("write comments: %w", err)
			}
		}
	}
	bundled := make(map[string]struct{}, len(b.Subagents))
	for _, sub := range b.Subagents {
		bundled[sub.ID.String()] = struct{}{}
	}
	var pauses []SubagentPause
	for _, p := range b.Pauses {
		// A pause naming a transcript the file does not carry has nothing to
		// resume, and would route the operator's answer into nothing.
		if _, ok := bundled[p.DialogID]; !ok {
			continue
		}
		p.DialogID = rewrite(p.DialogID)
		pauses = append(pauses, p)
	}
	if len(pauses) > 0 {
		if err := s.writeSubagentState(rootID, SubagentState{Pauses: pauses}); err != nil {
			return err
		}
	}
	if promptRendered && s.selection != nil {
		if err := s.WritePlanSelection(rootID, s.selection.Get()); err != nil {
			return err
		}
	}
	return nil
}

// removeImportedArtifacts removes every file writeImportedArtifacts may have
// written, for an import that failed after the rows were committed.
func (s *ChatService) removeImportedArtifacts(rootID uuid.UUID) {
	id := rootID.String()
	for _, path := range []string{
		filepath.Join(s.summariesDir, id+".txt"),
		filepath.Join(s.dagsDir, id+".md"),
		filepath.Join(s.planContractsDir, id+".json"),
		filepath.Join(s.actionPlansDir, id+".json"),
		actionPlanCommentsPath(s.actionPlansDir, rootID),
		s.subagentStatePath(rootID),
		planSelectionPath(s.planSelectionDir, rootID),
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			slog.Warn("import plan: remove partial artifact", "path", path, "error", err)
		}
	}
}

// validatePlanBundle checks everything ImportPlan relies on before anything is
// written, so a bad file fails as a whole and names what is wrong with it.
func validatePlanBundle(b PlanBundle) error {
	invalid := func(format string, args ...any) error {
		return fmt.Errorf("%w: %s", ErrInvalidPlanBundle, fmt.Sprintf(format, args...))
	}

	if b.Format != PlanBundleFormat {
		return invalid("format is %q, want %q", b.Format, PlanBundleFormat)
	}
	if b.Version < 1 || b.Version > PlanBundleVersion {
		return invalid("version %d is not supported (this nib reads up to %d)", b.Version, PlanBundleVersion)
	}
	if b.Plan.ID == uuid.Nil {
		return invalid("plan.id is required")
	}
	if m := strings.TrimSpace(b.Plan.Mode); !mode.IsValid(m) || !mode.CarriesPlan(m) {
		return invalid("plan.mode %q is not a plan mode", b.Plan.Mode)
	}
	if len(b.ActionPlan) > 0 {
		var doc map[string]any
		if err := json.Unmarshal(b.ActionPlan, &doc); err != nil {
			return invalid("actionPlan must be a JSON object")
		}
	}
	if len(b.Contract) > 0 {
		var doc map[string]any
		if err := json.Unmarshal(b.Contract, &doc); err != nil {
			return invalid("contract must be a JSON object")
		}
	}

	ids := map[uuid.UUID]struct{}{b.Plan.ID: {}}
	decompose := 0
	for i, sub := range b.Subagents {
		if sub.ID == uuid.Nil {
			return invalid("subagents[%d].id is required", i)
		}
		if _, dup := ids[sub.ID]; dup {
			return invalid("subagents[%d].id %s is used twice", i, sub.ID)
		}
		ids[sub.ID] = struct{}{}
		if !isBundledSubagentMode(sub.Mode) {
			return invalid("subagents[%d].mode %q cannot be imported", i, sub.Mode)
		}
		if sub.Mode == decomposeSubagentMode {
			decompose++
		}
	}
	// A plan resumes the first decompose transcript it finds, so a second one
	// would be a transcript nothing can ever reach.
	if decompose > 1 {
		return invalid("a plan has at most one decompose sub-agent, found %d", decompose)
	}

	attachments := map[uuid.UUID]struct{}{}
	for t, msgs := range bundleTranscripts(b) {
		where := "chat"
		if t > 0 {
			where = fmt.Sprintf("subagents[%d].messages", t-1)
		}
		for i, m := range msgs {
			switch m.Role {
			case "user", "assistant", "tool":
			default:
				return invalid("%s[%d].role %q is not importable", where, i, m.Role)
			}
			if len(m.ToolCalls) > 0 && !json.Valid(m.ToolCalls) {
				return invalid("%s[%d].toolCalls is not valid JSON", where, i)
			}
			for j, a := range m.Attachments {
				if a.ID == uuid.Nil {
					return invalid("%s[%d].attachments[%d].id is required", where, i, j)
				}
				if _, dup := attachments[a.ID]; dup {
					return invalid("%s[%d].attachments[%d].id %s is used twice", where, i, j, a.ID)
				}
				if _, clash := ids[a.ID]; clash {
					return invalid("%s[%d].attachments[%d].id %s is also a dialog id", where, i, j, a.ID)
				}
				attachments[a.ID] = struct{}{}
				if name := sanitizeAttachmentFilename(a.Filename); name == "" || name == "." {
					return invalid("%s[%d].attachments[%d].filename is required", where, i, j)
				}
				if len(a.Data) == 0 {
					return invalid("%s[%d].attachments[%d] is empty", where, i, j)
				}
				if int64(len(a.Data)) > attachmentMaxBytes {
					return invalid("%s[%d].attachments[%d] exceeds %d bytes", where, i, j, attachmentMaxBytes)
				}
			}
		}
	}
	return nil
}
