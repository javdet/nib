package mcpconfig

import (
	"path/filepath"
	"strings"
)

const defaultFileName = "mcp.json"

// ResolveFile returns the path used for the MCP servers JSON file.
// When mcpFile is empty, the default is filepath.Join(dataDir, "mcp.json").
// Absolute paths are used as-is; relative paths are joined under dataDir.
func ResolveFile(dataDir, mcpFile string) string {
	mcpFile = strings.TrimSpace(mcpFile)
	if mcpFile == "" {
		return filepath.Join(dataDir, defaultFileName)
	}
	if filepath.IsAbs(mcpFile) {
		return mcpFile
	}
	return filepath.Join(dataDir, mcpFile)
}
