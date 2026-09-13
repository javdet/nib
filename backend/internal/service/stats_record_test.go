package service

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

// recordingStatsWriter captures what the fire-and-forget writers send.
type recordingStatsWriter struct {
	mu          sync.Mutex
	transitions [][3]string
	llm         []domain.LLMUsageRecord
	runs        []domain.AgentRunUsageRecord
}

func (w *recordingStatsWriter) InsertLLMUsage(_ context.Context, rec domain.LLMUsageRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.llm = append(w.llm, rec)
	return nil
}

func (w *recordingStatsWriter) InsertAgentRunUsage(_ context.Context, rec domain.AgentRunUsageRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.runs = append(w.runs, rec)
	return nil
}

func (w *recordingStatsWriter) InsertPlanStatusTransition(_ context.Context, planID, from, to string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.transitions = append(w.transitions, [3]string{planID, from, to})
	return nil
}

func (w *recordingStatsWriter) transitionCount() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.transitions)
}

// The writes are detached goroutines, so the assertions have to wait for them
// rather than read immediately.
func waitFor(t *testing.T, cond func() bool) bool {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(5 * time.Millisecond)
	}
	return cond()
}

func newPlanStateService(t *testing.T, w *recordingStatsWriter) *ChatService {
	t.Helper()
	s := &ChatService{planStateDir: t.TempDir()}
	s.SetStatsWriter(w, "test-model")
	return s
}

func TestWritePlanStateRecordsTransition(t *testing.T) {
	w := &recordingStatsWriter{}
	s := newPlanStateService(t, w)
	id := uuid.New()

	// Absent file defaults to draft, so this is draft -> in_progress.
	if err := s.WritePlanState(id, PlanState{Status: ActionPlanStatusInProgress}); err != nil {
		t.Fatalf("WritePlanState() error = %v", err)
	}

	if !waitFor(t, func() bool { return w.transitionCount() == 1 }) {
		t.Fatalf("transitions recorded = %d, want 1", w.transitionCount())
	}
	got := w.transitions[0]
	if got[0] != id.String() || got[1] != string(ActionPlanStatusDraft) || got[2] != string(ActionPlanStatusInProgress) {
		t.Errorf("transition = %v, want [%s draft in_progress]", got, id)
	}
}

// A write that does not move the status is not a transition. Recording one
// would inflate every status-over-time chart with non-events, since plan state
// is rewritten whenever the schedule changes too.
func TestWritePlanStateSkipsNoOpTransition(t *testing.T) {
	w := &recordingStatsWriter{}
	s := newPlanStateService(t, w)
	id := uuid.New()

	if err := s.WritePlanState(id, PlanState{Status: ActionPlanStatusDone}); err != nil {
		t.Fatalf("WritePlanState() error = %v", err)
	}
	if !waitFor(t, func() bool { return w.transitionCount() == 1 }) {
		t.Fatalf("first write recorded %d transitions, want 1", w.transitionCount())
	}

	// Same status again, only the schedule changes.
	if err := s.WritePlanState(id, PlanState{Status: ActionPlanStatusDone, ScheduledAt: 42}); err != nil {
		t.Fatalf("WritePlanState() error = %v", err)
	}

	if waitFor(t, func() bool { return w.transitionCount() > 1 }) {
		t.Errorf("transitions recorded = %d, want 1: a no-op status write is not a transition", w.transitionCount())
	}
}

// Statistics must never be a condition of the work: with no writer wired the
// recording calls are no-ops, which is what keeps every other service test green.
func TestWritePlanStateWithoutStatsWriter(t *testing.T) {
	s := &ChatService{planStateDir: t.TempDir()}
	if err := s.WritePlanState(uuid.New(), PlanState{Status: ActionPlanStatusDone}); err != nil {
		t.Fatalf("WritePlanState() without a stats writer error = %v", err)
	}
}

// A reported zero cost is "the agent said nothing", not "the run was free":
// agent-entrypoint.sh fills the field with jq '... // 0'.
func TestPositiveCost(t *testing.T) {
	zero, priced := 0.0, 1.25
	tests := []struct {
		name string
		in   *float64
		want *float64
	}{
		{name: "absent", in: nil},
		{name: "reported zero is treated as unreported", in: &zero},
		{name: "a real price is kept", in: &priced, want: &priced},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := positiveCost(tt.in)
			switch {
			case tt.want == nil && got != nil:
				t.Fatalf("positiveCost() = %v, want nil", *got)
			case tt.want != nil && got == nil:
				t.Fatalf("positiveCost() = nil, want %v", *tt.want)
			case tt.want != nil && *got != *tt.want:
				t.Fatalf("positiveCost() = %v, want %v", *got, *tt.want)
			}
		})
	}
}
