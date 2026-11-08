package mode

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/mode/defaults"
)

// seedMarkerFile records which built-in tool names have already been offered to
// this data volume. The dot prefix keeps it out of any {mode}.json lookup.
const seedMarkerFile = ".seeded-tools"

// renamedTools maps a built-in tool that was replaced to the tools that
// replaced it. A list still carrying the old name gets the new ones in its
// place, once; a list whose operator had taken the old name out does not get
// the new ones either. Without this the ordinary rule would hand
// execute_command's successors to an operator who had removed it on purpose,
// and leave a dead name in every list that kept it.
var renamedTools = map[string][]string{
	"execute_command": {"run_command", "wait_job", "read_output", "cancel_job", "wait_until"},
}

// SeedResult reports what the startup reconcile of the allow lists did.
type SeedResult struct {
	Created []string // modes whose list did not exist and was written
	Added   map[string][]string
	// Renamed lists, per mode, the retired names replaced by their successors.
	Renamed map[string][]string
}

// SeedAllowLists reconciles the per-mode allow lists on the data volume with the
// ones compiled into the binary.
//
// A mode with no list gets the built-in one. A mode that already has a list keeps
// it, and only gains built-in tool names this volume has never been offered
// before -- so a tool added in a later release reaches an existing install, while
// a tool the operator deliberately removed stays removed. Nothing is ever taken
// out of a list, and the order the operator's file is in survives.
//
// Without this, a new tool would be dead on every existing install: the allow
// lists are seeded from the image only when the file is absent, and nothing in
// the interface edits them.
//
// The first run on a volume that predates the marker cannot tell a tool the
// operator removed from one they never had, so it restores any built-in they had
// taken out. That happens once; from then on a removal sticks.
func SeedAllowLists(dir string) (SeedResult, error) {
	result := SeedResult{Added: map[string][]string{}, Renamed: map[string][]string{}}
	if strings.TrimSpace(dir) == "" {
		return result, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return result, fmt.Errorf("create tools directory: %w", err)
	}

	seeded, err := readSeedMarker(dir)
	if err != nil {
		return result, err
	}

	changed := false
	for _, name := range Modes {
		builtin, err := defaultAllowList(name)
		if err != nil {
			return result, err
		}

		path := filepath.Join(dir, name+".json")
		current, err := readAllowListOrdered(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
			if len(builtin) == 0 {
				continue
			}
			if err := writeAllowList(path, builtin); err != nil {
				return result, err
			}
			result.Created = append(result.Created, name)
			for _, tool := range builtin {
				seeded[markerKey(name, tool)] = struct{}{}
				changed = true
			}
			continue
		case err != nil:
			return result, err
		}

		current, renamed, markerChanged := applyRenames(name, current, seeded)
		if markerChanged {
			changed = true
		}
		if len(renamed) > 0 {
			result.Renamed[name] = renamed
		}

		have := make(map[string]struct{}, len(current))
		for _, tool := range current {
			have[tool] = struct{}{}
		}

		var added []string
		for _, tool := range builtin {
			key := markerKey(name, tool)
			if _, offered := seeded[key]; offered {
				continue
			}
			seeded[key] = struct{}{}
			changed = true
			if _, present := have[tool]; present {
				continue
			}
			current = append(current, tool)
			added = append(added, tool)
		}
		if len(added) == 0 && len(renamed) == 0 {
			continue
		}
		if err := writeAllowList(path, current); err != nil {
			return result, err
		}
		if len(added) > 0 {
			result.Added[name] = added
		}
	}

	if !changed {
		return result, nil
	}
	return result, writeSeedMarker(dir, seeded)
}

// applyRenames replaces every retired name in current with its successors and
// records the successors as offered, so the ordinary rule never adds them a
// second time. When the retired name was offered to this mode and is gone from
// its list, the operator removed it, and the successors are recorded as
// offered without being added.
func applyRenames(mode string, current []string, seeded map[string]struct{}) ([]string, []string, bool) {
	olds := make([]string, 0, len(renamedTools))
	for old := range renamedTools {
		olds = append(olds, old)
	}
	sort.Strings(olds)

	var renamed []string
	markerChanged := false
	markOffered := func(successors []string) {
		for _, tool := range successors {
			key := markerKey(mode, tool)
			if _, ok := seeded[key]; !ok {
				seeded[key] = struct{}{}
				markerChanged = true
			}
		}
	}
	for _, old := range olds {
		successors := renamedTools[old]
		idx := slices.Index(current, old)
		if idx < 0 {
			if _, offered := seeded[markerKey(mode, old)]; offered {
				markOffered(successors)
			}
			continue
		}
		replacement := make([]string, 0, len(successors))
		for _, tool := range successors {
			if !slices.Contains(current, tool) {
				replacement = append(replacement, tool)
			}
		}
		current = slices.Concat(current[:idx:idx], replacement, current[idx+1:])
		markOffered(successors)
		renamed = append(renamed, old)
	}
	return current, renamed, markerChanged
}

// markerKey namespaces a tool name by its mode: the same tool can be built into
// two modes and removed from only one of them.
func markerKey(mode, tool string) string {
	return mode + "/" + tool
}

func defaultAllowList(mode string) ([]string, error) {
	b, err := defaults.FS.ReadFile(mode + ".json")
	if err != nil {
		// A mode with no built-in list is not an error: it simply ships without
		// filtering.
		return nil, nil
	}
	names, err := parseAllowToolsBytes(b, mode+".json (built-in)")
	if err != nil {
		return nil, err
	}
	return names, nil
}

func readAllowListOrdered(path string) ([]string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseAllowToolsBytes(b, path)
}

// parseAllowToolsBytes reads {"allow_tools":[...]} preserving order and dropping
// blanks and duplicates.
func parseAllowToolsBytes(b []byte, label string) ([]string, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		return nil, fmt.Errorf("parse allow-tools file %q: invalid JSON: %w", label, err)
	}
	rawList, ok := top["allow_tools"]
	if !ok {
		return nil, fmt.Errorf("parse allow-tools file %q: missing allow_tools key", label)
	}
	var names []string
	if err := json.Unmarshal(rawList, &names); err != nil {
		return nil, fmt.Errorf("parse allow-tools file %q: allow_tools must be a JSON array of strings: %w", label, err)
	}

	seen := make(map[string]struct{}, len(names))
	out := make([]string, 0, len(names))
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		out = append(out, n)
	}
	return out, nil
}

func writeAllowList(path string, names []string) error {
	data, err := json.MarshalIndent(map[string][]string{"allow_tools": names}, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal allow-tools %q: %w", path, err)
	}
	if err := atomicfile.Write(path, append(data, '\n')); err != nil {
		return fmt.Errorf("write allow-tools %q: %w", path, err)
	}
	return nil
}

func readSeedMarker(dir string) (map[string]struct{}, error) {
	seeded := make(map[string]struct{})

	data, err := os.ReadFile(filepath.Join(dir, seedMarkerFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return seeded, nil
		}
		return nil, fmt.Errorf("read allow-tools seed marker: %w", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if entry := strings.TrimSpace(line); entry != "" {
			seeded[entry] = struct{}{}
		}
	}
	return seeded, nil
}

func writeSeedMarker(dir string, seeded map[string]struct{}) error {
	entries := make([]string, 0, len(seeded))
	for entry := range seeded {
		entries = append(entries, entry)
	}
	sort.Strings(entries)

	path := filepath.Join(dir, seedMarkerFile)
	if err := atomicfile.WriteString(path, strings.Join(entries, "\n")+"\n"); err != nil {
		return fmt.Errorf("write allow-tools seed marker: %w", err)
	}
	return nil
}
