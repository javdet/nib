package systemprompts

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/javdet/nib/internal/systemprompts/defaults"
)

// EditableName is the only prompt whose content may be changed at runtime.
// Every other prompt is fixed by the image it ships in.
const EditableName = "discuss"

// Auxiliary lists embedded prompts that are not modes. They are appended to a
// mode's prompt rather than selected in the interface, so they never belong in
// mode.Modes but must still ship in the image.
//
// plan_stage narrows the plan prompt to a single stage for a fan-out subagent.
// rollback_stage narrows it to the plan's one rollback list, for the agent that
// runs once every stage has been written.
var Auxiliary = []string{"plan_stage", "rollback_stage"}

const mdSuffix = ".md"

// defaultContent maps prompt name -> baked-in markdown. It is built once at
// package init from the embedded FS, so reads are lock-free and cannot fail at
// runtime. Entries with an unusable basename are skipped here and reported by
// ValidateEmbeddedDefaults instead.
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

// Default returns the prompt compiled into the binary for name.
func Default(name string) (string, bool) {
	content, ok := defaultContent[name]
	return content, ok
}

// DefaultNames returns the names of all baked-in prompts, sorted.
func DefaultNames() []string {
	names := make([]string, 0, len(defaultContent))
	for name := range defaultContent {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateEmbeddedDefaults returns an error naming every required prompt that has
// no usable baked-in .md. Callers pass mode.Modes; taking the list as an argument
// keeps this package independent of the mode package.
//
// main calls this at startup so an image built without a prompt fails to boot
// rather than silently degrading every chat turn in that mode.
func ValidateEmbeddedDefaults(required []string) error {
	var missing []string
	for _, name := range required {
		if content, ok := defaultContent[name]; !ok || strings.TrimSpace(content) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("no prompt compiled into the binary for: %s", strings.Join(missing, ", "))
	}
	return nil
}
