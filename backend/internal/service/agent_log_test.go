package service

import (
	"strings"
	"testing"

	"github.com/javdet/nib/internal/llm"
)

func TestToolNamesFromCalls(t *testing.T) {
	t.Parallel()

	if got := toolNamesFromCalls(nil); got != nil {
		t.Fatalf("nil calls: got %v want nil", got)
	}

	calls := []llm.ToolCall{
		{Name: "knowledge_search", ID: "1"},
		{Name: "tool_search", ID: "2"},
	}
	got := toolNamesFromCalls(calls)
	want := []string{"knowledge_search", "tool_search"}
	if len(got) != len(want) {
		t.Fatalf("len: got %d want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names[%d]: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestTruncateForLog(t *testing.T) {
	t.Parallel()

	short := "hello"
	if got := truncateForLog(short, logTruncateRunes); got != short {
		t.Fatalf("short string: got %q want %q", got, short)
	}

	long := strings.Repeat("x", logTruncateRunes+100)
	got := truncateForLog(long, logTruncateRunes)
	if len([]rune(got)) <= logTruncateRunes {
		t.Fatalf("expected truncation suffix, got len %d", len([]rune(got)))
	}
	if !strings.Contains(got, "runes truncated") {
		t.Fatalf("expected truncation marker in %q", got)
	}
}
