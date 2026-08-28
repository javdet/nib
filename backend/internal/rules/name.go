package rules

import (
	"errors"

	"github.com/javdet/nib/internal/filestore"
)

// ErrInvalidName is returned when a rule name fails validation.
var ErrInvalidName = errors.New("invalid rule name")

// ValidateName checks that name is safe to use as a basename for a .md file.
func ValidateName(name string) error {
	return filestore.ValidateBasename(name, ErrInvalidName)
}
