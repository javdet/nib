package kb

import (
	"strings"
	"testing"
)

func TestChunk_basic(t *testing.T) {
	t.Parallel()
	got := Chunk("abcdefghij", 4, 1)
	want := []string{"abcd", "defg", "ghij"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d (%v)", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestChunk_overlapClamped(t *testing.T) {
	t.Parallel()
	got := Chunk("abc", 2, 99)
	if len(got) < 1 {
		t.Fatalf("expected at least one chunk, got %v", got)
	}
	if got[0] != "ab" {
		t.Errorf("first chunk = %q, want ab", got[0])
	}
}

func TestChunk_empty(t *testing.T) {
	t.Parallel()
	if got := Chunk("", 10, 2); got != nil {
		t.Fatalf("want nil, got %#v", got)
	}
}

func TestChunk_unicodeRunes(t *testing.T) {
	t.Parallel()
	s := strings.Repeat("é", 5)
	got := Chunk(s, 2, 0)
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3: %q", len(got), got)
	}
	if len([]rune(got[0])) != 2 || len([]rune(got[1])) != 2 || len([]rune(got[2])) != 1 {
		t.Fatalf("unexpected rune lengths: %#v", got)
	}
}
