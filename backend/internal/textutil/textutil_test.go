package textutil

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "ascii", in: "hello", want: "hello"},
		{name: "cyrillic", in: "Адаптация", want: "Адаптация"},
		{name: "nul", in: "a\x00b", want: "ab"},
		{name: "invalid utf8", in: "a\xc0b", want: "ab"},
		{name: "empty", in: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := Sanitize(tt.in)
			if got != tt.want {
				t.Fatalf("Sanitize() = %q, want %q", got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("Sanitize() returned invalid UTF-8")
			}
		})
	}
}

func TestTruncateRunes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		in       string
		maxRunes int
		want     string
	}{
		{name: "ascii short", in: "hello", maxRunes: 10, want: "hello"},
		{name: "ascii exact", in: "hello", maxRunes: 5, want: "hello"},
		{name: "ascii truncate", in: "hello", maxRunes: 3, want: "hel"},
		{name: "cyrillic", in: "Адаптация", maxRunes: 4, want: "Адап"},
		{name: "emoji", in: "🙂🙂🙂", maxRunes: 2, want: "🙂🙂"},
		{name: "zero", in: "hello", maxRunes: 0, want: ""},
		{name: "empty", in: "", maxRunes: 5, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := TruncateRunes(tt.in, tt.maxRunes)
			if got != tt.want {
				t.Fatalf("TruncateRunes() = %q, want %q", got, tt.want)
			}
			if !utf8.ValidString(got) {
				t.Fatalf("TruncateRunes() returned invalid UTF-8")
			}
		})
	}
}

func TestTruncateBytes(t *testing.T) {
	t.Parallel()

	suffix := "..."

	t.Run("short ascii unchanged", func(t *testing.T) {
		t.Parallel()
		short := strings.Repeat("a", 100)
		if got := TruncateBytes(short, suffix, 200); got != short {
			t.Fatalf("TruncateBytes(short) = %q, want unchanged", got)
		}
	})

	t.Run("long ascii with suffix", func(t *testing.T) {
		t.Parallel()
		long := strings.Repeat("b", 200)
		got := TruncateBytes(long, suffix, 100)
		if len(got) > 100 {
			t.Fatalf("len = %d, want <= 100", len(got))
		}
		if !strings.HasSuffix(got, suffix) {
			t.Fatalf("missing suffix: %q", got)
		}
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8")
		}
	})

	t.Run("cyrillic at byte boundary", func(t *testing.T) {
		t.Parallel()
		// Each Cyrillic letter is 2 bytes; cut mid-character without rune safety would fail.
		s := strings.Repeat("А", 50) // 100 bytes
		got := TruncateBytes(s, suffix, 81)
		if len(got) > 81 {
			t.Fatalf("len = %d, want <= 81", len(got))
		}
		if !utf8.ValidString(got) {
			t.Fatalf("invalid UTF-8 at byte boundary")
		}
	})

	t.Run("max smaller than suffix", func(t *testing.T) {
		t.Parallel()
		got := TruncateBytes("hello", suffix, 2)
		if got != suffix {
			t.Fatalf("TruncateBytes() = %q, want %q", got, suffix)
		}
	})
}
