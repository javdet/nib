package filestore

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const mdSuffix = ".md"

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ResolveDir returns the directory for file-backed resources.
// When configDir is empty, defaultRel is joined under dataDir.
// Absolute paths are used as-is; relative paths are joined under dataDir.
func ResolveDir(dataDir, configDir, defaultRel string) string {
	configDir = strings.TrimSpace(configDir)
	if configDir == "" {
		return filepath.Join(dataDir, defaultRel)
	}
	if filepath.IsAbs(configDir) {
		return configDir
	}
	return filepath.Join(dataDir, configDir)
}

// ValidateBasename checks that name is safe to use as a basename for a .md file.
func ValidateBasename(name string, errInvalid error) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", errInvalid)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: reserved name", errInvalid)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: path separators are not allowed", errInvalid)
	}
	if strings.Contains(name, ".") {
		return fmt.Errorf("%w: dots are not allowed", errInvalid)
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("%w: must match [a-zA-Z0-9][a-zA-Z0-9_-]*", errInvalid)
	}
	return nil
}

// MDSuffix returns the markdown file suffix.
func MDSuffix() string {
	return mdSuffix
}

// IsNotExist reports whether err is os.ErrNotExist.
func IsNotExist(err error) bool {
	return errors.Is(err, os.ErrNotExist)
}
