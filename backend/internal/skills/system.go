package skills

import (
	"errors"
	"fmt"
	"sort"

	"github.com/javdet/nib/internal/nibdocs"
)

// ErrSystemSkill is returned when a write targets a skill the image owns.
var ErrSystemSkill = errors.New("skill is provided by the system and cannot be changed")

// systemSkill is a skill served from the binary rather than from the data
// volume. Its metadata lives here instead of in frontmatter so the body can stay
// byte-identical to the document it is sliced from -- see internal/nibdocs.
type systemSkill struct {
	name        string
	description string
	load        func() (string, error)
}

// systemSkills are how the agent answers a question about nib itself. They are
// never written to the data volume, never editable, and never listed by the HTTP
// API: an operator configures nib, the agent only reads about it.
var systemSkills = []systemSkill{
	{
		name: "nib-configuration",
		description: "How nib itself is configured and operated: MCP servers (streamable HTTP only), " +
			"the LLM provider, modes, prompts, prompt variables, skills, rules, tools, secrets, the " +
			"executor and the data volume. Load this for any question about configuring nib or about " +
			"how you yourself are set up.",
		load: func() (string, error) {
			return nibdocs.Section(nibdocs.RepoGuide, nibdocs.OperatingSection)
		},
	},
	{
		name: "nib-internals",
		description: "nib's own repository guide: architecture and layering, the agent loop, " +
			"orchestration, dialog artifacts, secrets, the executor, observability and conventions. " +
			"Load this when a question about nib needs more detail than nib-configuration carries.",
		load: func() (string, error) {
			content, ok := nibdocs.Doc(nibdocs.RepoGuide)
			if !ok {
				return "", fmt.Errorf("document %q is not shipped in this build", nibdocs.RepoGuide)
			}
			return content, nil
		},
	},
}

// IsSystem reports whether name belongs to the system tier, so a caller can
// refuse a write or skip the template rendering that only operator text needs.
func IsSystem(name string) bool {
	for _, skill := range systemSkills {
		if skill.name == name {
			return true
		}
	}
	return false
}

// SystemNames returns the names of every system skill, sorted.
func SystemNames() []string {
	names := make([]string, 0, len(systemSkills))
	for _, skill := range systemSkills {
		names = append(names, skill.name)
	}
	sort.Strings(names)
	return names
}

// SystemMeta returns the catalog entries of the system skills, sorted by name.
func SystemMeta() []Meta {
	metas := make([]Meta, 0, len(systemSkills))
	for _, skill := range systemSkills {
		metas = append(metas, Meta{Name: skill.name, Description: skill.description})
	}
	sort.Slice(metas, func(i, j int) bool { return metas[i].Name < metas[j].Name })
	return metas
}

// systemContent loads a system skill's body.
func systemContent(name string) (string, error) {
	for _, skill := range systemSkills {
		if skill.name != name {
			continue
		}
		content, err := skill.load()
		if err != nil {
			return "", fmt.Errorf("load system skill %s: %w", name, err)
		}
		return content, nil
	}
	return "", fmt.Errorf("%w: %s", ErrSystemSkill, name)
}

// ValidateSystemSkills reports whether every system skill can actually be
// loaded. An image whose embedded documentation is missing must fail to boot
// rather than answer configuration questions with an error at the tool call.
func ValidateSystemSkills() error {
	for _, skill := range systemSkills {
		content, err := skill.load()
		if err != nil {
			return err
		}
		if content == "" {
			return fmt.Errorf("system skill %s loaded empty content", skill.name)
		}
		if skill.description == "" {
			return fmt.Errorf("system skill %s has no description", skill.name)
		}
	}
	return nil
}
