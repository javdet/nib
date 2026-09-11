package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/textutil"
)

// maxActionNoteBytes caps one note. The notes of every action that has run are
// handed to every later sub-agent through get_action_list, so this bound is
// multiplied by the length of the plan rather than paid once.
const maxActionNoteBytes = 4000

// actionNoteTruncationMarker tells the reading agent it is looking at a cut
// result, so it opens the transcript instead of trusting a half sentence.
const actionNoteTruncationMarker = "\n…(truncated)"

func actionPlanNotesPath(dir string, dialogID uuid.UUID) string {
	return filepath.Join(dir, dialogID.String()+".notes.json")
}

// readActionPlanNotes returns the executor notes of a plan keyed by action row
// key.
//
// It and its writer are deliberately unexported: the notes are the sub-agents'
// working memory, not something the operator reads, and handler lives in another
// package -- so "the web interface never receives this" is a property the
// compiler holds rather than a convention someone drops later.
func (s *ChatService) readActionPlanNotes(dialogID uuid.UUID) (map[string]string, error) {
	path := actionPlanNotesPath(s.actionPlansDir, dialogID)
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}

	var notes map[string]string
	if err := json.Unmarshal(b, &notes); err != nil {
		return nil, fmt.Errorf("unmarshal action plan notes: %w", err)
	}
	if notes == nil {
		return map[string]string{}, nil
	}
	return notes, nil
}

// writeActionPlanNotes persists executor notes keyed by action row key.
func (s *ChatService) writeActionPlanNotes(dialogID uuid.UUID, notes map[string]string) error {
	if notes == nil {
		notes = map[string]string{}
	}

	if err := os.MkdirAll(s.actionPlansDir, 0o755); err != nil {
		return fmt.Errorf("create action_plans directory: %w", err)
	}

	data, err := json.Marshal(notes)
	if err != nil {
		return fmt.Errorf("marshal action plan notes: %w", err)
	}

	return writeActionPlanFile(actionPlanNotesPath(s.actionPlansDir, dialogID), data)
}

// recordActionNote stores what a run of one action reported, for the sub-agents
// that run the actions after it.
//
// The note is per row, not per attempt: a restart overwrites it, the same claim
// `run` makes in get_action_list. Nothing is returned and a failure is only
// logged -- every caller has already finished the work, and losing the note must
// not turn a successful run into a failed one.
//
// It takes the plan lock, which is a plain sync.Mutex, so it must be called
// after finishActionExecRun has returned rather than from inside its mutate
// closure.
func (s *ChatService) recordActionNote(planID uuid.UUID, key, text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	text = textutil.TruncateBytes(text, actionNoteTruncationMarker, maxActionNoteBytes)

	mu := s.planMutex(planID)
	mu.Lock()
	defer mu.Unlock()

	notes, err := s.readActionPlanNotes(planID)
	if err != nil {
		slog.Warn("record action note: read", "plan_id", planID, "key", key, "error", err)
		return
	}
	notes[key] = text
	if err := s.writeActionPlanNotes(planID, notes); err != nil {
		slog.Warn("record action note: write", "plan_id", planID, "key", key, "error", err)
	}
}
