package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
)

// SubagentPause records a sub-agent suspended on a question the operator answers
// in the orchestrator's chat.
//
// It is persisted rather than held in memory for the same reason a fan-out run
// is: the operator may answer after a page reload or a restart, and the answer
// has to find its way back to the transcript waiting for it. Being a file, it
// needs no database migration.
type SubagentPause struct {
	// Subagent names the paused agent, so a later kind that can ask is routed to
	// its own resume path rather than to decompose's.
	Subagent SubagentName `json:"subagent"`
	// DialogID is the sub-agent's own dialog, whose transcript holds the
	// dangling ask_question.
	DialogID string `json:"dialogId"`
	// ToolCallID is that dangling call: where the answers are written.
	ToolCallID string `json:"toolCallId"`
	// MainAskID is the ask_question the orchestrator relayed the questions
	// under. Its call has an id of its own, so the two have to be mapped: the
	// operator answers the orchestrator's, and the answer belongs to the
	// sub-agent's. It is empty between the launch returning and the orchestrator
	// actually asking.
	MainAskID string `json:"mainAskId,omitempty"`
	// Questions is what was relayed, in order, so answers pair up even if the
	// orchestrator reworded them.
	Questions []domain.Question `json:"questions"`
	PausedAt  int64             `json:"pausedAt"`
}

// SubagentState is every pause outstanding on one plan.
type SubagentState struct {
	Pauses []SubagentPause `json:"pauses,omitempty"`
}

func (s *ChatService) subagentMutex(rootID uuid.UUID) *sync.Mutex {
	mu, _ := s.subagentClaims.LoadOrStore("state|"+rootID.String(), &sync.Mutex{})
	return mu.(*sync.Mutex)
}

func (s *ChatService) subagentStatePath(rootID uuid.UUID) string {
	return filepath.Join(s.subagentsDir, rootID.String()+".json")
}

// ReadSubagentState returns the pauses recorded for a plan. A missing file is
// not an error: most plans never suspend a sub-agent.
func (s *ChatService) ReadSubagentState(rootID uuid.UUID) (SubagentState, bool, error) {
	b, err := os.ReadFile(s.subagentStatePath(rootID))
	if errors.Is(err, os.ErrNotExist) {
		return SubagentState{}, false, nil
	}
	if err != nil {
		return SubagentState{}, false, fmt.Errorf("read subagent state: %w", err)
	}

	var state SubagentState
	if err := json.Unmarshal(b, &state); err != nil {
		return SubagentState{}, false, fmt.Errorf("unmarshal subagent state: %w", err)
	}
	return state, true, nil
}

func (s *ChatService) writeSubagentState(rootID uuid.UUID, state SubagentState) error {
	if err := os.MkdirAll(s.subagentsDir, 0o755); err != nil {
		return fmt.Errorf("create subagents directory: %w", err)
	}
	data, err := json.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshal subagent state: %w", err)
	}
	if err := atomicfile.Write(s.subagentStatePath(rootID), data); err != nil {
		return fmt.Errorf("write subagent state: %w", err)
	}
	return nil
}

// updateSubagentState applies mutate under the plan's lock, so a launch
// recording a pause cannot race the answer that clears it.
func (s *ChatService) updateSubagentState(rootID uuid.UUID, mutate func(*SubagentState)) (SubagentState, error) {
	mu := s.subagentMutex(rootID)
	mu.Lock()
	defer mu.Unlock()

	state, _, err := s.ReadSubagentState(rootID)
	if err != nil {
		return SubagentState{}, err
	}
	mutate(&state)
	if err := s.writeSubagentState(rootID, state); err != nil {
		return SubagentState{}, err
	}
	return state, nil
}

// recordSubagentPause stores a suspended sub-agent, replacing any pause of the
// same sub-agent the orchestrator never got round to relaying.
func (s *ChatService) recordSubagentPause(rootID uuid.UUID, p SubagentPause) error {
	_, err := s.updateSubagentState(rootID, func(state *SubagentState) {
		kept := state.Pauses[:0]
		for _, existing := range state.Pauses {
			// A bound pause is one the operator can still answer; an unbound one
			// of the same sub-agent has been superseded by this suspension.
			if existing.Subagent == p.Subagent && existing.MainAskID == "" {
				continue
			}
			kept = append(kept, existing)
		}
		state.Pauses = append(kept, p)
	})
	return err
}

// bindSubagentPause stamps the orchestrator's own ask_question id onto the pause
// it is relaying, and reports whether it found one to stamp.
//
// It is called at the point the orchestrator's loop suspends, which is the only
// place both ids exist: the sub-agent's dangling call was recorded when it
// suspended, and the orchestrator's own call id only exists now.
//
// A question count that does not match the pause is taken as the orchestrator
// asking something of its own rather than relaying, and binds nothing: the
// answers are paired positionally, so binding a question that is not the relay
// would send the operator's answer to the wrong place. The pause stays unbound
// and is picked up by the dangling-ask branch on the next launch.
func (s *ChatService) bindSubagentPause(rootID uuid.UUID, mainAskID string, questions []domain.Question) bool {
	bound := false
	if _, err := s.updateSubagentState(rootID, func(state *SubagentState) {
		for i := len(state.Pauses) - 1; i >= 0; i-- {
			p := &state.Pauses[i]
			if p.MainAskID != "" {
				continue
			}
			if len(questions) > 0 && len(p.Questions) > 0 && len(questions) != len(p.Questions) {
				return
			}
			p.MainAskID = mainAskID
			if len(questions) > 0 {
				p.Questions = questions
			}
			bound = true
			return
		}
	}); err != nil {
		return false
	}
	return bound
}

// takeSubagentPause removes and returns the pause relayed under mainAskID.
func (s *ChatService) takeSubagentPause(rootID uuid.UUID, mainAskID string) (SubagentPause, bool, error) {
	var found SubagentPause
	ok := false

	if _, err := s.updateSubagentState(rootID, func(state *SubagentState) {
		kept := make([]SubagentPause, 0, len(state.Pauses))
		for _, p := range state.Pauses {
			if !ok && p.MainAskID != "" && p.MainAskID == mainAskID {
				found = p
				ok = true
				continue
			}
			kept = append(kept, p)
		}
		state.Pauses = kept
	}); err != nil {
		return SubagentPause{}, false, err
	}
	return found, ok, nil
}
