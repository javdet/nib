package includedtools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/javdet/nib/internal/mode"
)

func TestSyncCatalogTurnsNewToolsOnEverywhere(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	res, err := svc.SyncCatalog([]string{"beta", "alpha", "alpha", " gamma "})
	if err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if want := []string{"alpha", "beta", "gamma"}; !equalStrings(res.Added, want) {
		t.Fatalf("added = %v, want %v", res.Added, want)
	}
	if len(res.Removed) != 0 {
		t.Fatalf("removed = %v, want none", res.Removed)
	}

	for _, modeName := range mode.Modes {
		got, err := svc.Get(modeName)
		if err != nil {
			t.Fatalf("Get(%s): %v", modeName, err)
		}
		if want := []string{"alpha", "beta", "gamma"}; !equalStrings(got, want) {
			t.Fatalf("mode %s = %v, want %v", modeName, got, want)
		}
	}
}

func TestSyncCatalogKeepsOperatorExclusion(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if _, err := svc.SyncCatalog([]string{"alpha", "beta"}); err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if err := svc.Set("plan", []string{"alpha"}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	// A reindex that discovers nothing new must not undo the exclusion, and the
	// one tool that is new must land in every mode including the edited one.
	res, err := svc.SyncCatalog([]string{"alpha", "beta", "delta"})
	if err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if want := []string{"delta"}; !equalStrings(res.Added, want) {
		t.Fatalf("added = %v, want %v", res.Added, want)
	}

	got, err := svc.Get("plan")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if want := []string{"alpha", "delta"}; !equalStrings(got, want) {
		t.Fatalf("plan = %v, want %v", got, want)
	}
}

func TestSyncCatalogDropsToolsGoneFromCatalog(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if _, err := svc.SyncCatalog([]string{"alpha", "beta"}); err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}

	res, err := svc.SyncCatalog([]string{"alpha"})
	if err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if want := []string{"beta"}; !equalStrings(res.Removed, want) {
		t.Fatalf("removed = %v, want %v", res.Removed, want)
	}

	for _, modeName := range mode.Modes {
		got, err := svc.Get(modeName)
		if err != nil {
			t.Fatalf("Get(%s): %v", modeName, err)
		}
		if want := []string{"alpha"}; !equalStrings(got, want) {
			t.Fatalf("mode %s = %v, want %v", modeName, got, want)
		}
	}

	// A tool that left the catalog is forgotten, so re-adding its server turns
	// it back on rather than leaving it silently excluded forever.
	res, err = svc.SyncCatalog([]string{"alpha", "beta"})
	if err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if want := []string{"beta"}; !equalStrings(res.Added, want) {
		t.Fatalf("added = %v, want %v", res.Added, want)
	}
}

func TestSyncCatalogIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if _, err := svc.SyncCatalog([]string{"alpha"}); err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	res, err := svc.SyncCatalog([]string{"alpha"})
	if err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if res.Changed() {
		t.Fatalf("second sync changed lists: %+v", res)
	}
}

func TestSyncCatalogEmptyCatalogRecordsFirstRun(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if _, err := svc.SyncCatalog(nil); err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if _, err := os.Stat(svc.KnownPath()); err != nil {
		t.Fatalf("stat known file: %v", err)
	}

	res, err := svc.SyncCatalog([]string{"alpha"})
	if err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if want := []string{"alpha"}; !equalStrings(res.Added, want) {
		t.Fatalf("added = %v, want %v", res.Added, want)
	}
}

func TestSyncCatalogRejectsCorruptKnownFile(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if _, err := svc.SyncCatalog([]string{"alpha"}); err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if err := svc.Set("plan", []string{}); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := os.WriteFile(svc.KnownPath(), []byte("{"), 0o644); err != nil {
		t.Fatalf("write known file: %v", err)
	}

	if _, err := svc.SyncCatalog([]string{"alpha"}); err == nil {
		t.Fatal("expected error for corrupt known file")
	}
	got, err := svc.Get("plan")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("plan = %v, want the exclusion left alone", got)
	}
}

func TestSyncCatalogWritesKnownFileBesideIncluded(t *testing.T) {
	dir := t.TempDir()
	svc := NewService(dir)

	if _, err := svc.SyncCatalog([]string{"alpha"}); err != nil {
		t.Fatalf("SyncCatalog: %v", err)
	}
	if want := filepath.Join(dir, knownFileName); svc.KnownPath() != want {
		t.Fatalf("KnownPath = %s, want %s", svc.KnownPath(), want)
	}
	data, err := os.ReadFile(svc.KnownPath())
	if err != nil {
		t.Fatalf("read known file: %v", err)
	}
	if !strings.Contains(string(data), `"alpha"`) {
		t.Fatalf("known file = %s", data)
	}
}

func equalStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}
