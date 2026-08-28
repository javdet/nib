package systemprompts

import (
	"errors"
	"testing"
)

func TestValidateName(t *testing.T) {
	t.Parallel()

	valid := []string{"default", "plan", "my-prompt", "a1", "A_b-2"}
	for _, name := range valid {
		name := name
		t.Run("valid_"+name, func(t *testing.T) {
			t.Parallel()
			if err := ValidateName(name); err != nil {
				t.Fatalf("ValidateName(%q) = %v, want nil", name, err)
			}
		})
	}

	invalid := []string{
		"",
		".",
		"..",
		"../x",
		"x/y",
		`x\y`,
		".hidden",
		"-bad",
		"has.dot",
	}

	for _, name := range invalid {
		name := name
		t.Run("invalid_"+name, func(t *testing.T) {
			t.Parallel()
			err := ValidateName(name)
			if err == nil {
				t.Fatalf("ValidateName(%q) = nil, want error", name)
			}
			if !errors.Is(err, ErrInvalidName) {
				t.Fatalf("ValidateName(%q) error = %v, want ErrInvalidName", name, err)
			}
		})
	}
}
