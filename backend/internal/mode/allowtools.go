package mode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultRelDir = "tools"

// ResolveDir returns the directory used for per-mode allow-tools JSON files.
// When allowToolsDir is empty, the default is filepath.Join(dataDir, "tools").
// Absolute paths are used as-is; relative paths are joined under dataDir.
func ResolveDir(dataDir, allowToolsDir string) string {
	allowToolsDir = strings.TrimSpace(allowToolsDir)
	if allowToolsDir == "" {
		return filepath.Join(dataDir, defaultRelDir)
	}
	if filepath.IsAbs(allowToolsDir) {
		return allowToolsDir
	}
	return filepath.Join(dataDir, allowToolsDir)
}

// LoadAllowList reads {dir}/{mode}.json and returns the allow_tools name set.
// A missing file is not an error and yields (nil, nil) meaning no filtering.
func LoadAllowList(dir, mode string) (map[string]struct{}, error) {
	if dir == "" || mode == "" {
		return nil, nil
	}
	path := filepath.Join(dir, mode+".json")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("stat allow-tools mode file %q: %w", path, err)
	}
	return parseAllowToolsFile(path)
}

// parseAllowToolsFile reads a JSON object {"allow_tools":["name",...]} and returns
// a set of tool names. Each entry is trimmed; empty strings are skipped; duplicates
// are removed. It errors if the file cannot be read, JSON is invalid, allow_tools
// is missing, allow_tools is not an array of strings, or the resulting list is
// empty after trimming.
func parseAllowToolsFile(path string) (map[string]struct{}, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read allow-tools file: %w", err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, fmt.Errorf("parse allow-tools file %q: invalid JSON: %w", path, err)
	}
	rawList, ok := top["allow_tools"]
	if !ok {
		return nil, fmt.Errorf("parse allow-tools file %q: missing allow_tools key", path)
	}
	var names []string
	if err := json.Unmarshal(rawList, &names); err != nil {
		return nil, fmt.Errorf("parse allow-tools file %q: allow_tools must be a JSON array of strings: %w", path, err)
	}
	out := make(map[string]struct{})
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		out[n] = struct{}{}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("parse allow-tools file %q: allow_tools is empty after trimming", path)
	}
	return out, nil
}
