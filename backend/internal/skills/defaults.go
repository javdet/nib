package skills

import (
	"io/fs"
	"sort"
	"strings"

	"github.com/javdet/nib/internal/skills/defaults"
)

// defaultContent maps skill name -> markdown baked into the binary. It is built
// once at package init from the embedded FS, so reads are lock-free and cannot
// fail at runtime. Entries with an unusable basename are skipped here and
// reported by ValidateEmbeddedDefaults instead.
var defaultContent = loadDefaults()

func loadDefaults() map[string]string {
	out := make(map[string]string)

	_ = fs.WalkDir(defaults.FS, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(d.Name(), mdSuffix) {
			return nil
		}
		name := strings.TrimSuffix(d.Name(), mdSuffix)
		if ValidateName(name) != nil {
			return nil
		}
		data, err := defaults.FS.ReadFile(path)
		if err != nil {
			return nil
		}
		out[name] = string(data)
		return nil
	})

	return out
}

// Default returns the skill compiled into the binary for name.
func Default(name string) (string, bool) {
	content, ok := defaultContent[name]
	return content, ok
}

// DefaultNames returns the names of all baked-in skills, sorted.
func DefaultNames() []string {
	names := make([]string, 0, len(defaultContent))
	for name := range defaultContent {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
