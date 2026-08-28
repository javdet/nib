package systemprompts

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/repository"
)

// Source records where the effective prompt body came from.
type Source string

const (
	// SourceDefault means the prompt was read from the binary.
	SourceDefault Source = "default"
	// SourceOverride means a runtime override file on the data volume was used.
	SourceOverride Source = "override"
)

// Prompt is the effective system prompt for a mode.
type Prompt struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Source  Source `json:"source"`
}

// Housekeeping reports what the startup scan of the override directory found.
type Housekeeping struct {
	OverridePath   string
	OverrideActive bool
	Pruned         bool     // a redundant discuss override was removed
	Stale          []string // files present in the directory but no longer honored
}

// Service serves the system prompts compiled into the binary, with a single
// runtime-editable override for discuss stored as {dir}/discuss.md on the data
// volume (a PVC in Kubernetes, a bind mount under compose) so UI edits survive
// container and image rebuilds.
type Service struct {
	// mu serializes override mutations only. Reads go straight to the file
	// system: os.Rename and os.Remove are atomic, so a concurrent Get sees the
	// whole old body, the whole new body, or ENOENT (and then the baked-in
	// default).
	mu  sync.Mutex
	dir string
}

// NewService creates a Service. promptsDir is the raw value from config prompts.dir;
// it is resolved with ResolveDir against dataDir and holds runtime overrides only.
func NewService(dataDir, promptsDir string) *Service {
	return &Service{dir: ResolveDir(dataDir, promptsDir)}
}

// Dir returns the resolved override directory path.
func (s *Service) Dir() string {
	return s.dir
}

// OverridePath returns the file a runtime override for name would live in.
func (s *Service) OverridePath(name string) string {
	return filepath.Join(s.dir, name+mdSuffix)
}

// Get returns the effective prompt: the override file when present and readable,
// otherwise the prompt compiled into the binary.
//
// Reads are deliberately uncached — one os.ReadFile per call, usually a single
// failing openat. This matches the rules and skills stores and keeps out-of-band
// edits to the override file live. Do not turn this into a cache without also
// solving invalidation.
func (s *Service) Get(name string) (Prompt, error) {
	if err := ValidateName(name); err != nil {
		return Prompt{}, err
	}

	path := s.OverridePath(name)
	data, readErr := os.ReadFile(path)
	if readErr == nil {
		return Prompt{Name: name, Content: string(data), Source: SourceOverride}, nil
	}
	if !errors.Is(readErr, os.ErrNotExist) {
		// An unreadable volume must degrade to a working chat, not break every turn.
		slog.Warn("system prompt override unreadable, using the built-in prompt",
			"name", name, "path", path, "error", readErr)
	}

	content, ok := Default(name)
	if !ok {
		if errors.Is(readErr, os.ErrNotExist) {
			return Prompt{}, repository.ErrNotFound
		}
		return Prompt{}, fmt.Errorf("read prompt override %s: %w", name, readErr)
	}
	return Prompt{Name: name, Content: content, Source: SourceDefault}, nil
}

// Set stores a runtime override. Only EditableName is accepted; every other
// prompt is fixed by the image.
func (s *Service) Set(name, content string) error {
	if err := s.checkEditable(name); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return fmt.Errorf("create prompts directory: %w", err)
	}
	if err := atomicfile.WriteString(s.OverridePath(name), content); err != nil {
		return fmt.Errorf("write prompt override %s: %w", name, err)
	}
	return nil
}

// Reset removes the runtime override so the baked-in prompt takes effect again.
// It is idempotent: an absent override is not an error.
func (s *Service) Reset(name string) error {
	if err := s.checkEditable(name); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := os.Remove(s.OverridePath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove prompt override %s: %w", name, err)
	}
	return nil
}

// HasOverride reports whether an override file exists for name.
func (s *Service) HasOverride(name string) (bool, error) {
	if err := ValidateName(name); err != nil {
		return false, err
	}
	if _, err := os.Stat(s.OverridePath(name)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, fmt.Errorf("stat prompt override %s: %w", name, err)
	}
	return true, nil
}

// Prepare readies the override directory for use. It creates the directory,
// removes a discuss override identical to the baked-in prompt (so an install that
// never edited it keeps tracking future image updates), and reports files that are
// no longer honored.
//
// Nothing else is ever deleted: per-mode prompt files left over from when prompts
// lived on the volume may hold operator edits, and discarding them is not this
// code's call. The trade-off is that an operator can no longer pin the current
// default by saving it verbatim — which is the point of shipping prompts in the image.
func (s *Service) Prepare() (Housekeeping, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	hk := Housekeeping{OverridePath: s.OverridePath(EditableName)}

	if err := os.MkdirAll(s.dir, 0o755); err != nil {
		return hk, fmt.Errorf("create prompts directory: %w", err)
	}

	switch data, err := os.ReadFile(hk.OverridePath); {
	case err == nil && sameIgnoringTrailingSpace(string(data), defaultContent[EditableName]):
		// A UI save round-trip can add or drop a trailing newline; a
		// whitespace-only diff is not an edit worth pinning forever.
		if err := os.Remove(hk.OverridePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return hk, fmt.Errorf("remove redundant prompt override: %w", err)
		}
		hk.Pruned = true
	case err == nil:
		hk.OverrideActive = true
	case !errors.Is(err, os.ErrNotExist):
		return hk, fmt.Errorf("read prompt override %s: %w", EditableName, err)
	}

	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return hk, fmt.Errorf("read prompts directory: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || e.Name() == EditableName+mdSuffix {
			continue
		}
		hk.Stale = append(hk.Stale, e.Name())
	}
	sort.Strings(hk.Stale)

	return hk, nil
}

func (s *Service) checkEditable(name string) error {
	if err := ValidateName(name); err != nil {
		return err
	}
	if name != EditableName {
		return fmt.Errorf("%w: %s (only %s can be changed at runtime)", ErrNotEditable, name, EditableName)
	}
	return nil
}

func sameIgnoringTrailingSpace(a, b string) bool {
	const cutset = " \t\r\n"
	return a == b || strings.TrimRight(a, cutset) == strings.TrimRight(b, cutset)
}
