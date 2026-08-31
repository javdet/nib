package service

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/javdet/nib/internal/mode"
	"github.com/google/uuid"
)

// systemToolsDialogID is a non-nil placeholder so dialog-scoped tool definitions
// are included when building the developer catalog. Handlers are never invoked.
var systemToolsDialogID = uuid.MustParse("00000000-0000-0000-0000-000000000001")

// systemToolsBinding also carries a placeholder stage, so tools that exist only
// for a plan stage subagent still appear in the catalog an operator browses.
func systemToolsBinding() toolBinding {
	b := newToolBinding(systemToolsDialogID)
	b.stage = "placeholder"
	return b
}

// SystemToolDef describes a built-in agent tool for the developer catalog.
type SystemToolDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Parameters  json.RawMessage `json:"parameters"`
	Modes       []string        `json:"modes"`
	AlwaysOn    bool            `json:"always_on,omitempty"`
}

// SystemToolDefs returns definitions for all built-in tools and their mode availability.
func (s *ChatService) SystemToolDefs() ([]SystemToolDef, error) {
	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	s.addLocalTools(&catalog, nil, systemToolsBinding())

	modeAllowLists, err := s.loadModeAllowLists()
	if err != nil {
		return nil, err
	}

	result := make([]SystemToolDef, 0, len(catalog.tools))
	for _, tool := range catalog.tools {
		def := SystemToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}
		if tool.Name == ChatNameToolName {
			def.AlwaysOn = true
		} else {
			def.Modes = modesForTool(tool.Name, modeAllowLists)
		}
		result = append(result, def)
	}
	return result, nil
}

// SystemToolsForMode returns built-in tool definitions enabled for a single mode
// according to the system allow list file (data/tools/{mode}.json).
func (s *ChatService) SystemToolsForMode(modeName string) ([]SystemToolDef, error) {
	if !mode.IsValid(modeName) {
		return nil, fmt.Errorf("invalid mode %q", modeName)
	}

	allow, err := mode.LoadAllowList(s.allowToolsDir, modeName)
	if err != nil {
		return nil, err
	}

	catalog := toolCatalog{
		mcpRoutes:     make(map[string]toolRoute),
		localHandlers: make(map[string]localToolHandler),
	}
	s.addLocalTools(&catalog, allow, systemToolsBinding())

	result := make([]SystemToolDef, 0, len(catalog.tools))
	for _, tool := range catalog.tools {
		def := SystemToolDef{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		}
		if tool.Name == ChatNameToolName {
			def.AlwaysOn = true
		}
		result = append(result, def)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Name < result[j].Name
	})
	return result, nil
}

func (s *ChatService) loadModeAllowLists() (map[string]map[string]struct{}, error) {
	out := make(map[string]map[string]struct{}, len(mode.Modes))
	for _, m := range mode.Modes {
		allow, err := mode.LoadAllowList(s.allowToolsDir, m)
		if err != nil {
			return nil, fmt.Errorf("load allow list for mode %q: %w", m, err)
		}
		out[m] = allow
	}
	return out, nil
}

func modesForTool(name string, modeAllowLists map[string]map[string]struct{}) []string {
	var modes []string
	for _, m := range mode.Modes {
		allow := modeAllowLists[m]
		if allow == nil {
			continue
		}
		if _, ok := allow[name]; ok {
			modes = append(modes, m)
		}
	}
	return modes
}
