package nibdocs

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRepoGuideMatchesRepoRoot fails when the embedded copy has drifted from the
// repository-root CLAUDE.md, which is the file everyone actually edits.
//
// It skips when the root file is not there: the project's own toolchain mounts
// only `backend` into the container (docker run -v "$PWD/backend":/app), so the
// root is invisible there. CI checks out the whole repository and runs go test
// from backend/, so drift still turns the build red.
func TestRepoGuideMatchesRepoRoot(t *testing.T) {
	t.Parallel()

	want, err := os.ReadFile(filepath.Join("..", "..", "..", "CLAUDE.md"))
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("repository root CLAUDE.md is not mounted")
	}
	if err != nil {
		t.Fatalf("read repository root CLAUDE.md: %v", err)
	}

	got, ok := Doc(RepoGuide)
	if !ok {
		t.Fatalf("Doc(%q) missing from the binary", RepoGuide)
	}
	if got != string(want) {
		t.Fatalf("the embedded copy of CLAUDE.md has drifted from the repository root; run `make sync-docs`")
	}
}
