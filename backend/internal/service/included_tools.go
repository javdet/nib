package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/javdet/nib/internal/includedtools"
	"github.com/javdet/nib/internal/mode"
	"github.com/javdet/nib/internal/toolcatalog"
)

// ErrInvalidMode is returned for a mode name outside [mode.Modes].
var ErrInvalidMode = errors.New("invalid mode")

// SystemToolSummary is one built-in tool as the settings UI lists it.
type SystemToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	AlwaysOn    bool   `json:"alwaysOn,omitempty"`
}

// ModeTools is everything the settings UI shows for one mode: the built-in Go
// tools that mode always carries, and the MCP tools an operator has switched on.
type ModeTools struct {
	Mode          string              `json:"mode"`
	SystemTools   []SystemToolSummary `json:"systemTools"`
	IncludedTools []string            `json:"includedTools"`
}

// systemToolLister reports the built-in Go tools a mode always carries.
// [ChatService] implements it; taking the interface rather than the whole agent
// keeps this service constructible in a test.
type systemToolLister interface {
	SystemToolsForMode(modeName string) ([]SystemToolDef, error)
}

// IncludedToolsService answers "which tools does this mode see" by joining the
// two halves of that answer: the built-in registry the agent loop owns, and the
// per-mode MCP include list on disk. Both used to be joined in the HTTP handler,
// which put the rule in the transport layer and left no place to test it.
type IncludedToolsService struct {
	included *includedtools.Service
	catalog  *toolcatalog.Store
	system   systemToolLister
}

// NewIncludedToolsService wires the include-list store, the MCP tool catalog and
// the source of the built-in tool registry.
func NewIncludedToolsService(included *includedtools.Service, catalog *toolcatalog.Store, system systemToolLister) *IncludedToolsService {
	return &IncludedToolsService{included: included, catalog: catalog, system: system}
}

// ForMode returns the built-in and included MCP tools for one mode.
func (s *IncludedToolsService) ForMode(modeName string) (ModeTools, error) {
	if !mode.IsValid(modeName) {
		return ModeTools{}, fmt.Errorf("%w: %q", ErrInvalidMode, modeName)
	}
	return s.forMode(modeName)
}

// SetForMode replaces the mode's MCP include list and returns the mode's tools as
// they stand afterwards, so a caller needs one round trip rather than two.
func (s *IncludedToolsService) SetForMode(modeName string, tools []string) (ModeTools, error) {
	if !mode.IsValid(modeName) {
		return ModeTools{}, fmt.Errorf("%w: %q", ErrInvalidMode, modeName)
	}
	if err := s.included.Set(modeName, tools); err != nil {
		return ModeTools{}, err
	}
	return s.forMode(modeName)
}

// ListCatalogTools returns every MCP tool the catalog currently carries. An
// unconfigured catalog is not an error: the backend runs without one and the UI
// shows an empty picker.
func (s *IncludedToolsService) ListCatalogTools(ctx context.Context) ([]toolcatalog.CatalogTool, error) {
	if s.catalog == nil {
		return []toolcatalog.CatalogTool{}, nil
	}
	return s.catalog.ListTools(ctx)
}

func (s *IncludedToolsService) forMode(modeName string) (ModeTools, error) {
	systemDefs, err := s.system.SystemToolsForMode(modeName)
	if err != nil {
		return ModeTools{}, err
	}
	systemTools := make([]SystemToolSummary, 0, len(systemDefs))
	for _, def := range systemDefs {
		systemTools = append(systemTools, SystemToolSummary{
			Name:        def.Name,
			Description: def.Description,
			AlwaysOn:    def.AlwaysOn,
		})
	}

	included, err := s.included.Get(modeName)
	if err != nil {
		return ModeTools{}, err
	}
	if included == nil {
		included = []string{}
	}

	return ModeTools{
		Mode:          modeName,
		SystemTools:   systemTools,
		IncludedTools: included,
	}, nil
}
