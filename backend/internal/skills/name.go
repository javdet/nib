package skills

import (
	"errors"

	"github.com/javdet/nib/internal/filestore"
)

// ErrInvalidName is returned when a skill name fails validation.
var ErrInvalidName = errors.New("invalid skill name")

// ValidateName checks that name is safe to use as a basename for a .md file.
func ValidateName(name string) error {
	return filestore.ValidateBasename(name, ErrInvalidName)
}
