package kbdoc

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// ErrInvalidName is returned for a collection name that cannot be used as a
// document basename.
var ErrInvalidName = errors.New("invalid collection name")

// collectionNamePattern is the single source of truth for what a knowledge base
// collection may be called. Documents are stored as {name}.md, so the same
// pattern has to be safe as a file basename: the leading character is
// alphanumeric, which rules out "." and "..", and neither separator is allowed.
var collectionNamePattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$`)

// ValidName reports whether name is a usable collection name.
func ValidName(name string) bool {
	return collectionNamePattern.MatchString(name)
}

// ValidateName checks that name can be used as a knowledge document basename.
func ValidateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidName)
	}
	if !ValidName(name) {
		return fmt.Errorf("%w: must match [a-zA-Z0-9][a-zA-Z0-9._-]{0,63}", ErrInvalidName)
	}
	return nil
}
