package skills

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/repository"
)

// seedMarkerFile records which built-in skills have already been offered to this
// data volume. The dot prefix and the missing .md suffix keep it out of List.
const seedMarkerFile = ".seeded-defaults"

// SeedResult reports what the startup seeding of built-in skills did.
type SeedResult struct {
	Created []string // written to the skills directory on this start
	Skipped []string // seeded by an earlier start, or the name was already taken
}

// Seed writes the skills compiled into the binary into the skills directory so a
// fresh install starts with a usable catalog.
//
// Each built-in is seeded at most once per data volume: the names it has handled
// are appended to a marker file, so a skill the operator edited, renamed, or
// deleted is not resurrected by the next restart. A built-in added in a later
// image is not in the marker yet and therefore still lands on an existing
// install. A name already taken by an operator's own skill is left alone and
// recorded as handled — the image never overwrites skill content.
func (s *Service) Seed() (SeedResult, error) {
	dir := s.store.Dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return SeedResult{}, fmt.Errorf("create skills directory: %w", err)
	}

	seeded, err := readSeedMarker(dir)
	if err != nil {
		return SeedResult{}, err
	}

	var result SeedResult
	for _, name := range DefaultNames() {
		if _, done := seeded[name]; done {
			continue
		}
		// A system skill is served from the binary and must never gain an
		// editable copy on the volume; the two registries are disjoint, and
		// TestSystemAndDefaultNamesAreDisjoint keeps them that way.
		if IsSystem(name) {
			continue
		}
		seeded[name] = struct{}{}

		switch err := s.store.Create(name, defaultContent[name]); {
		case err == nil:
			result.Created = append(result.Created, name)
		case errors.Is(err, repository.ErrAlreadyExists):
			result.Skipped = append(result.Skipped, name)
		default:
			return result, fmt.Errorf("seed skill %s: %w", name, err)
		}
	}

	if len(result.Created) == 0 && len(result.Skipped) == 0 {
		return result, nil
	}
	if err := writeSeedMarker(dir, seeded); err != nil {
		return result, err
	}
	return result, nil
}

func readSeedMarker(dir string) (map[string]struct{}, error) {
	seeded := make(map[string]struct{})

	data, err := os.ReadFile(filepath.Join(dir, seedMarkerFile))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return seeded, nil
		}
		return nil, fmt.Errorf("read skill seed marker: %w", err)
	}

	for _, line := range strings.Split(string(data), "\n") {
		if name := strings.TrimSpace(line); name != "" {
			seeded[name] = struct{}{}
		}
	}
	return seeded, nil
}

func writeSeedMarker(dir string, seeded map[string]struct{}) error {
	names := make([]string, 0, len(seeded))
	for name := range seeded {
		names = append(names, name)
	}
	sort.Strings(names)

	path := filepath.Join(dir, seedMarkerFile)
	if err := atomicfile.WriteString(path, strings.Join(names, "\n")+"\n"); err != nil {
		return fmt.Errorf("write skill seed marker: %w", err)
	}
	return nil
}
