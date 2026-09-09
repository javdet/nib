package service

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/javdet/nib/internal/includedtools"
	"github.com/javdet/nib/internal/mode"
)

type fakeSystemToolLister struct {
	defs  []SystemToolDef
	calls []string
	err   error
}

func (f *fakeSystemToolLister) SystemToolsForMode(modeName string) ([]SystemToolDef, error) {
	f.calls = append(f.calls, modeName)
	return f.defs, f.err
}

func newIncludedToolsFixture(t *testing.T) (*IncludedToolsService, *includedtools.Service) {
	t.Helper()

	store := includedtools.NewService(t.TempDir())
	system := &fakeSystemToolLister{defs: []SystemToolDef{
		{Name: "ask_question", Description: "ask the operator", AlwaysOn: true},
		{Name: "create_dag", Description: "draw the plan"},
	}}
	return NewIncludedToolsService(store, nil, system), store
}

// The mode guard used to live in the HTTP handler, where no other caller could
// reach it. It is checked before the write, so a bad mode name never creates a
// list on disk.
func TestIncludedToolsService_RejectsUnknownMode(t *testing.T) {
	t.Parallel()

	svc, store := newIncludedToolsFixture(t)

	if _, err := svc.ForMode("nonesuch"); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("ForMode error = %v, want ErrInvalidMode", err)
	}
	if _, err := svc.SetForMode("nonesuch", []string{"some_tool"}); !errors.Is(err, ErrInvalidMode) {
		t.Fatalf("SetForMode error = %v, want ErrInvalidMode", err)
	}

	all, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	if _, ok := all["nonesuch"]; ok {
		t.Fatalf("the refused mode was written to disk: %v", all)
	}
}

// Every real mode has to pass the guard, or the settings UI loses a tab.
func TestIncludedToolsService_AcceptsEveryMode(t *testing.T) {
	t.Parallel()

	svc, _ := newIncludedToolsFixture(t)
	for _, name := range mode.Modes {
		got, err := svc.ForMode(name)
		if err != nil {
			t.Errorf("ForMode(%q): %v", name, err)
			continue
		}
		if got.Mode != name {
			t.Errorf("ForMode(%q).Mode = %q", name, got.Mode)
		}
	}
}

// The response joins the built-in registry with the on-disk include list. A mode
// with no list yet must still encode as [] rather than null.
func TestIncludedToolsService_ForModeJoinsBothHalves(t *testing.T) {
	t.Parallel()

	svc, _ := newIncludedToolsFixture(t)

	got, err := svc.ForMode(mode.Default)
	if err != nil {
		t.Fatalf("ForMode: %v", err)
	}
	if got.IncludedTools == nil {
		t.Fatal("IncludedTools is nil, want an empty slice so it encodes as []")
	}
	names := make([]string, 0, len(got.SystemTools))
	for _, tool := range got.SystemTools {
		names = append(names, tool.Name)
	}
	if !slices.Equal(names, []string{"ask_question", "create_dag"}) {
		t.Fatalf("SystemTools = %v, want the registry's tools", names)
	}
	if !got.SystemTools[0].AlwaysOn || got.SystemTools[1].AlwaysOn {
		t.Fatalf("AlwaysOn not carried through: %+v", got.SystemTools)
	}
}

// Set returns the mode as it stands afterwards, so the UI needs one round trip
// rather than a write followed by a read.
func TestIncludedToolsService_SetForModeReturnsTheNewState(t *testing.T) {
	t.Parallel()

	svc, store := newIncludedToolsFixture(t)

	got, err := svc.SetForMode(mode.Default, []string{"mcp__jira__search"})
	if err != nil {
		t.Fatalf("SetForMode: %v", err)
	}
	if !slices.Equal(got.IncludedTools, []string{"mcp__jira__search"}) {
		t.Fatalf("IncludedTools = %v, want the tools just written", got.IncludedTools)
	}

	persisted, err := store.Get(mode.Default)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !slices.Equal(persisted, []string{"mcp__jira__search"}) {
		t.Fatalf("on disk = %v, want the tools just written", persisted)
	}
}

// The backend runs without a tool catalog, and the picker has to render empty
// rather than 500 in that case.
func TestIncludedToolsService_ListCatalogToolsWithoutCatalog(t *testing.T) {
	t.Parallel()

	svc, _ := newIncludedToolsFixture(t)
	tools, err := svc.ListCatalogTools(context.Background())
	if err != nil {
		t.Fatalf("ListCatalogTools: %v", err)
	}
	if tools == nil {
		t.Fatal("ListCatalogTools returned nil, want an empty slice so it encodes as []")
	}
	if len(tools) != 0 {
		t.Fatalf("ListCatalogTools = %v, want empty", tools)
	}
}
