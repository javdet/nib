package mode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func readNames(t *testing.T, path string) []string {
	t.Helper()

	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc struct {
		AllowTools []string `json:"allow_tools"`
	}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return doc.AllowTools
}

func writeNames(t *testing.T, path string, names []string) {
	t.Helper()

	b, err := json.Marshal(map[string][]string{"allow_tools": names})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSeedAllowListsWritesMissingLists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	result, err := SeedAllowLists(dir)
	if err != nil {
		t.Fatalf("SeedAllowLists: %v", err)
	}
	if len(result.Created) == 0 {
		t.Fatal("nothing was created on an empty directory")
	}

	names := readNames(t, filepath.Join(dir, "decompose.json"))
	for _, want := range []string{"get_action_list", "execute_action"} {
		if !slices.Contains(names, want) {
			t.Errorf("decompose list is missing %q: %v", want, names)
		}
	}
}

// The upgrade path this exists for: a volume that already has a list must gain a
// tool added by a later release without losing the operator's own edits.
func TestSeedAllowListsAddsNewToolToExistingList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "decompose.json")
	writeNames(t, path, []string{"knowledge_search", "an_operators_own_tool"})

	result, err := SeedAllowLists(dir)
	if err != nil {
		t.Fatalf("SeedAllowLists: %v", err)
	}
	if slices.Contains(result.Created, "decompose") {
		t.Error("an existing list was reported as created")
	}

	names := readNames(t, path)
	if !slices.Contains(names, "an_operators_own_tool") {
		t.Errorf("the operator's own entry was dropped: %v", names)
	}
	if !slices.Contains(names, "execute_action") {
		t.Errorf("execute_action was not added: %v", names)
	}
	if names[0] != "knowledge_search" || names[1] != "an_operators_own_tool" {
		t.Errorf("existing order was not preserved: %v", names)
	}
}

// A tool the operator deletes must stay deleted, or every restart would undo
// their configuration.
func TestSeedAllowListsLeavesARemovedToolRemoved(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "decompose.json")

	if _, err := SeedAllowLists(dir); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	kept := slices.DeleteFunc(readNames(t, path), func(n string) bool {
		return n == "execute_action"
	})
	writeNames(t, path, kept)

	if _, err := SeedAllowLists(dir); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if names := readNames(t, path); slices.Contains(names, "execute_action") {
		t.Errorf("a removed tool came back: %v", names)
	}
}

func TestSeedAllowListsIsIdempotent(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := SeedAllowLists(dir); err != nil {
		t.Fatalf("first seed: %v", err)
	}
	before := readNames(t, filepath.Join(dir, "execute.json"))

	result, err := SeedAllowLists(dir)
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if len(result.Created) != 0 || len(result.Added) != 0 {
		t.Errorf("second run changed something: %+v", result)
	}
	if after := readNames(t, filepath.Join(dir, "execute.json")); !slices.Equal(before, after) {
		t.Errorf("list changed: %v -> %v", before, after)
	}
}

// The lists the binary ships must stay loadable by the reader that gates every
// turn, or a mode would silently run unfiltered.
func TestSeededListsLoadBack(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if _, err := SeedAllowLists(dir); err != nil {
		t.Fatalf("SeedAllowLists: %v", err)
	}

	for _, name := range Modes {
		allow, err := LoadAllowList(dir, name)
		if err != nil {
			t.Errorf("LoadAllowList(%s): %v", name, err)
			continue
		}
		if len(allow) == 0 {
			t.Errorf("mode %s seeded an empty allow list", name)
		}
	}
}
