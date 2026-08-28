package mcpconfig

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
)

var validName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)

// ErrInvalidName is returned when an MCP server name fails validation.
var ErrInvalidName = errors.New("invalid mcp server name")

// ErrInvalidJSON is returned when mcp.json content cannot be parsed or validated.
var ErrInvalidJSON = errors.New("invalid mcp json")

// ValidateName checks that name is safe to use as an mcpServers map key.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("%w: name is required", ErrInvalidName)
	}
	if name == "." || name == ".." {
		return fmt.Errorf("%w: reserved name", ErrInvalidName)
	}
	if strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("%w: path separators are not allowed", ErrInvalidName)
	}
	if !validName.MatchString(name) {
		return fmt.Errorf("%w: must match [a-zA-Z0-9][a-zA-Z0-9_-]*", ErrInvalidName)
	}
	return nil
}
