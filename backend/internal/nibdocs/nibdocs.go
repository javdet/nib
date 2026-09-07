// Package nibdocs serves nib's own documentation from inside the binary, so the
// agent can answer a question about how nib is configured from the same text a
// developer reads.
package nibdocs

import (
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

// RepoGuide is the repository guide, a copy of the root CLAUDE.md.
//
// The file behind it is deliberately not named claude.md: the root CLAUDE.md is
// listed in .gitignore on some checkouts, and with core.ignorecase set -- the
// default on macOS -- that pattern would swallow the embedded copy too, so it
// would never be committed and the image would boot without its own
// documentation.
const RepoGuide = "repo-guide"

// OperatingSection is the CLAUDE.md section holding the operator-facing answers
// about configuring nib itself. It is served as its own skill so a question
// about nib does not have to pull the whole repository guide into the context.
const OperatingSection = "Operating and configuring nib"

const mdSuffix = ".md"

// docContent maps document name -> markdown. It is built once at package init
// from the embedded FS, so reads are lock-free and cannot fail at runtime;
// a missing document is reported by ValidateEmbedded instead.
var docContent = loadDocs()

func loadDocs() map[string]string {
	out := make(map[string]string)

	entries, err := fs.ReadDir(docsFS, "docs")
	if err != nil {
		return out
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), mdSuffix) {
			continue
		}
		data, err := docsFS.ReadFile("docs/" + entry.Name())
		if err != nil {
			continue
		}
		out[strings.TrimSuffix(entry.Name(), mdSuffix)] = string(data)
	}
	return out
}

// Doc returns a whole document by name.
func Doc(name string) (string, bool) {
	content, ok := docContent[name]
	return content, ok
}

// Names returns every embedded document name, sorted.
func Names() []string {
	names := make([]string, 0, len(docContent))
	for name := range docContent {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// ValidateEmbedded reports whether every named document made it into the
// binary. An image built without one must fail to boot rather than answer
// configuration questions from nothing.
func ValidateEmbedded(names ...string) error {
	var missing []string
	for _, name := range names {
		if _, ok := docContent[name]; !ok {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("documents missing from the binary: %s", strings.Join(missing, ", "))
	}
	return nil
}

// Sections returns the "## " headings of a document, in file order.
func Sections(name string) []string {
	content, ok := docContent[name]
	if !ok {
		return nil
	}

	var headings []string
	forEachSection(content, func(heading string, _ []string) {
		headings = append(headings, heading)
	})
	return headings
}

// Section returns one "## " slice of a document, heading line included and any
// "### " subheadings kept inside it. The heading is matched case-insensitively.
//
// On a miss the error names every heading the document has: that text is what
// the agent reads back from get_skill, so it can correct itself on the next
// call instead of concluding the document is empty.
func Section(name, heading string) (string, error) {
	content, ok := docContent[name]
	if !ok {
		return "", fmt.Errorf("document %q is not shipped in this build", name)
	}

	want := strings.ToLower(strings.TrimSpace(heading))
	var found []string
	forEachSection(content, func(got string, lines []string) {
		if found == nil && strings.ToLower(got) == want {
			found = lines
		}
	})
	if found == nil {
		return "", fmt.Errorf("document %q has no section %q; it has: %s",
			name, heading, strings.Join(Sections(name), ", "))
	}
	return strings.TrimRight(strings.Join(found, "\n"), "\n") + "\n", nil
}

// forEachSection walks the "## " sections of a document. Fenced blocks are
// tracked because CLAUDE.md is full of shell examples, and a `## comment`
// inside one is not a heading.
func forEachSection(content string, visit func(heading string, lines []string)) {
	var (
		heading string
		lines   []string
		fenced  bool
	)
	flush := func() {
		if heading != "" {
			visit(heading, lines)
		}
	}

	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
		}
		if !fenced && strings.HasPrefix(line, "## ") {
			flush()
			heading = strings.TrimSpace(strings.TrimPrefix(line, "## "))
			lines = []string{line}
			continue
		}
		if heading != "" {
			lines = append(lines, line)
		}
	}
	flush()
}
