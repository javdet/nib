package service

import (
	"testing"

	"github.com/javdet/nib/internal/llm"
)

func toolResultIDs(msgs []llm.Message) []string {
	var out []string
	for _, m := range msgs {
		if m.Role == "tool" {
			out = append(out, m.ToolCallID)
		}
	}
	return out
}

// assertWellFormedTranscript checks the invariant the completion API enforces:
// the rows answering a call sit directly under it, and each call is answered once.
func assertWellFormedTranscript(t *testing.T, msgs []llm.Message) {
	t.Helper()

	answered := make(map[string]int)
	for i, m := range msgs {
		if m.Role == "tool" {
			prev := i - 1
			for prev >= 0 && msgs[prev].Role == "tool" {
				prev--
			}
			if prev < 0 || msgs[prev].Role != "assistant" || len(msgs[prev].ToolCalls) == 0 {
				t.Errorf("tool row %d (%s) does not follow an assistant with tool_calls", i, m.ToolCallID)
			}
			continue
		}
		if m.Role != "assistant" || len(m.ToolCalls) == 0 {
			continue
		}
		for j, tc := range m.ToolCalls {
			at := i + 1 + j
			if at >= len(msgs) || msgs[at].Role != "tool" || msgs[at].ToolCallID != tc.ID {
				t.Errorf("call %s at %d is not answered at %d", tc.ID, i, at)
				continue
			}
			answered[tc.ID]++
		}
	}
	for id, n := range answered {
		if n != 1 {
			t.Errorf("call %s answered %d times, want 1", id, n)
		}
	}
}

// The regression the repair exists for. A round stores its assistant row and its
// tool rows as separate inserts; an action sub-agent finishing in that gap lands
// between them. Matching results by adjacency would answer both calls twice --
// once synthetically, once for real -- which the completion API rejects outright.
func TestRepairOrphanToolCalls_foreignAppendBetweenCallAndResults(t *testing.T) {
	t.Parallel()

	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1"}, {ID: "call_2"}}},
		{Role: "assistant", Content: "1.1 finished"},
		{Role: "tool", ToolCallID: "call_1", Content: "real 1"},
		{Role: "tool", ToolCallID: "call_2", Content: "real 2"},
	}

	got := repairOrphanToolCalls(msgs)
	assertWellFormedTranscript(t, got)

	if ids := toolResultIDs(got); len(ids) != 2 {
		t.Fatalf("tool rows = %v, want exactly one per call", ids)
	}
	for _, m := range got {
		if m.Role == "tool" && m.Content != "real 1" && m.Content != "real 2" {
			t.Errorf("a real result was replaced by a synthetic one: %q", m.Content)
		}
	}

	var kept bool
	for _, m := range got {
		if m.Role == "assistant" && m.Content == "1.1 finished" {
			kept = true
		}
	}
	if !kept {
		t.Error("the interleaved message was dropped; it must survive the repair")
	}
}

// A pending ask_question leaves its call deliberately unanswered, so a message
// appended behind it must not turn the real results of the same round into
// duplicates.
func TestRepairOrphanToolCalls_foreignAppendWithPendingAsk(t *testing.T) {
	t.Parallel()

	msgs := []llm.Message{
		{Role: "assistant", ToolCalls: []llm.ToolCall{{ID: "call_1"}, {ID: "ask_1"}}},
		{Role: "tool", ToolCallID: "call_1", Content: "real 1"},
		{Role: "assistant", Content: "1.2 finished"},
	}

	got := repairOrphanToolCalls(msgs)
	assertWellFormedTranscript(t, got)

	if ids := toolResultIDs(got); len(ids) != 2 {
		t.Fatalf("tool rows = %v, want one per call", ids)
	}
}

// A tool row answering no call cannot be sent anywhere, so the repair drops it
// rather than emitting it after an assistant that never made the call.
func TestRepairOrphanToolCalls_dropsUnmatchedToolRow(t *testing.T) {
	t.Parallel()

	msgs := []llm.Message{
		{Role: "user", Content: "go"},
		{Role: "tool", ToolCallID: "ghost", Content: "orphan"},
		{Role: "assistant", Content: "done"},
	}

	got := repairOrphanToolCalls(msgs)
	assertWellFormedTranscript(t, got)
	if ids := toolResultIDs(got); len(ids) != 0 {
		t.Fatalf("tool ids = %v, want none", ids)
	}
}
