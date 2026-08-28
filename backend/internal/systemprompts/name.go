package systemprompts

import (
	"errors"

	"github.com/javdet/nib/internal/filestore"
)

// ErrInvalidName is returned when a prompt name fails validation.
var ErrInvalidName = errors.New("invalid prompt name")

// ErrNotEditable is returned when a write targets a prompt other than EditableName.
// System prompts ship with the image; only discuss is editable at runtime.
var ErrNotEditable = errors.New("system prompt is not editable")

// ValidateName checks that name is safe to use as a basename for a .md file.
func ValidateName(name string) error {
	return filestore.ValidateBasename(name, ErrInvalidName)
}
