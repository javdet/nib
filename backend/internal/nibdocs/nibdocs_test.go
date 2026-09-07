package nibdocs

import (
	"slices"
	"strings"
	"testing"
)

func TestDocAndNames(t *testing.T) {
	t.Parallel()

	content, ok := Doc(RepoGuide)
	if !ok {
		t.Fatalf("Doc(%q) missing; expected internal/nibdocs/docs/%s.md", RepoGuide, RepoGuide)
	}
	if !strings.HasPrefix(content, "# CLAUDE.md") {
		t.Fatalf("Doc(%q) does not start with the CLAUDE.md title: %q", RepoGuide, firstLine(content))
	}
	if _, ok := Doc("nope"); ok {
		t.Fatal(`Doc("nope") reported a document that does not exist`)
	}
	if got := Names(); !slices.Contains(got, RepoGuide) {
		t.Fatalf("Names() = %v, want it to contain %q", got, RepoGuide)
	}
}

func TestValidateEmbedded(t *testing.T) {
	t.Parallel()

	if err := ValidateEmbedded(RepoGuide); err != nil {
		t.Fatalf("ValidateEmbedded(%q) = %v, want nil", RepoGuide, err)
	}

	err := ValidateEmbedded(RepoGuide, "ghost")
	if err == nil {
		t.Fatal("ValidateEmbedded with a missing document = nil, want error")
	}
	if !strings.Contains(err.Error(), "ghost") {
		t.Fatalf("error %q does not name the missing document", err)
	}
}

func TestSectionsAreInFileOrder(t *testing.T) {
	t.Parallel()

	sections := Sections(RepoGuide)
	for _, want := range []string{"Project", "Configuration split", "Architecture", "Conventions"} {
		if !slices.Contains(sections, want) {
			t.Fatalf("Sections(%q) = %v, want it to contain %q", RepoGuide, sections, want)
		}
	}

	project := slices.Index(sections, "Project")
	architecture := slices.Index(sections, "Architecture")
	if project > architecture {
		t.Fatalf("Sections() is not in file order: Project at %d, Architecture at %d", project, architecture)
	}
	if Sections("nope") != nil {
		t.Fatal(`Sections("nope") returned headings for a document that does not exist`)
	}
}

func TestSection(t *testing.T) {
	t.Parallel()

	body, err := Section(RepoGuide, "Architecture")
	if err != nil {
		t.Fatalf("Section(Architecture): %v", err)
	}
	if !strings.HasPrefix(body, "## Architecture") {
		t.Fatalf("Section() does not start with its own heading: %q", firstLine(body))
	}
	// The slice stops at the next "## " and keeps its own "### " subheadings.
	if strings.Contains(body, "\n## Conventions") {
		t.Fatal("Section(Architecture) ran into the next section")
	}
	if !strings.Contains(body, "### Backend layering") {
		t.Fatal("Section(Architecture) dropped its subheadings")
	}

	lower, err := Section(RepoGuide, "  arCHItecture ")
	if err != nil {
		t.Fatalf("Section() does not match case-insensitively: %v", err)
	}
	if lower != body {
		t.Fatal("Section() returned different bodies for the same heading in different case")
	}
}

// The error text is what the agent reads back from get_skill, so it has to name
// the headings that do exist -- otherwise a single typo looks like an empty doc.
func TestSectionMissingNamesTheAlternatives(t *testing.T) {
	t.Parallel()

	if _, err := Section(RepoGuide, "Nonexistent"); err == nil {
		t.Fatal("Section() with an unknown heading = nil error")
	} else if !strings.Contains(err.Error(), "Architecture") {
		t.Fatalf("error %q does not list the available headings", err)
	}

	if _, err := Section("nope", "Architecture"); err == nil {
		t.Fatal("Section() on an unknown document = nil error")
	}
}

// A fenced shell example containing a `## comment` line must not be read as a
// heading; CLAUDE.md is full of them.
func TestForEachSectionIgnoresFencedBlocks(t *testing.T) {
	t.Parallel()

	doc := strings.Join([]string{
		"## Real",
		"text",
		"```bash",
		"## not a heading",
		"```",
		"more text",
		"## Second",
		"tail",
	}, "\n")

	var headings []string
	forEachSection(doc, func(heading string, _ []string) {
		headings = append(headings, heading)
	})

	want := []string{"Real", "Second"}
	if !slices.Equal(headings, want) {
		t.Fatalf("forEachSection() headings = %v, want %v", headings, want)
	}
}

// The executable form of the requirement: the section the nib-configuration
// skill serves has to answer "how do I connect an MCP server?" correctly.
func TestOperatingSectionAnswersTheMCPTransportQuestion(t *testing.T) {
	t.Parallel()

	body, err := Section(RepoGuide, OperatingSection)
	if err != nil {
		t.Fatalf("Section(%q): %v", OperatingSection, err)
	}
	for _, want := range []string{"streamable", "stdio", "SSE"} {
		if !strings.Contains(body, want) {
			t.Fatalf("section %q does not mention %q, so the agent cannot answer how MCP servers connect", OperatingSection, want)
		}
	}
}

func firstLine(content string) string {
	line, _, _ := strings.Cut(content, "\n")
	return line
}
