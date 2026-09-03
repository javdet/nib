package mode

import "testing"

func TestIsValid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want bool
	}{
		{"decompose", "decompose", true},
		{"plan", "plan", true},
		{"execute", "execute", true},
		{"discuss", "discuss", true},
		{"incident", "incident", true},
		{"empty", "", false},
		{"unknown", "rollback", false},
		{"typo", "decompse", false},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsValid(tt.in); got != tt.want {
				t.Errorf("IsValid(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestModesPredefined(t *testing.T) {
	t.Parallel()
	if len(Modes) != 6 {
		t.Fatalf("len(Modes) = %d, want 6", len(Modes))
	}
	for _, want := range []string{"main", "decompose", "plan", "execute", "discuss", "incident"} {
		if !IsValid(want) {
			t.Errorf("expected predefined mode %q", want)
		}
	}
}
