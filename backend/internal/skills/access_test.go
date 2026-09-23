package skills_test

import (
	"testing"

	"github.com/javdet/nib/internal/skills"
)

func TestParseAccess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		want    skills.Access
	}{
		{name: "no frontmatter", content: "just a body", want: skills.AccessEnabled},
		{name: "no access key", content: "---\nname: a\ndescription: b\n---\nbody", want: skills.AccessEnabled},
		{name: "explicit", content: "---\nname: a\naccess: explicit\n---\nbody", want: skills.AccessExplicit},
		{name: "disabled in capitals", content: "---\naccess: DISABLED\n---\nbody", want: skills.AccessDisabled},
		{name: "quoted value", content: "---\naccess: \"explicit\"\n---\nbody", want: skills.AccessExplicit},
		{name: "enabled spelled out", content: "---\naccess: enabled\n---\nbody", want: skills.AccessEnabled},
		{name: "unknown value", content: "---\naccess: sometimes\n---\nbody", want: skills.AccessEnabled},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := skills.ParseAccess(tt.content); got != tt.want {
				t.Fatalf("ParseAccess() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestListAndAccessOfCarryTheSetting(t *testing.T) {
	t.Parallel()

	svc := skills.NewService(t.TempDir(), "")
	for name, content := range map[string]string{
		"plain":  "---\nname: plain\ndescription: d\n---\nbody",
		"hidden": "---\nname: hidden\ndescription: d\naccess: explicit\n---\nbody",
		"off":    "---\nname: off\ndescription: d\naccess: disabled\n---\nbody",
	} {
		if err := svc.Create(name, content); err != nil {
			t.Fatalf("Create(%q) error = %v", name, err)
		}
	}

	list, err := svc.List()
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	want := map[string]skills.Access{
		"hidden": skills.AccessExplicit,
		"off":    skills.AccessDisabled,
		"plain":  skills.AccessEnabled,
	}
	if len(list.Skills) != len(want) {
		t.Fatalf("List() = %+v, want %d skills", list.Skills, len(want))
	}
	for _, meta := range list.Skills {
		if meta.Access != want[meta.Name] {
			t.Fatalf("List() access of %q = %q, want %q", meta.Name, meta.Access, want[meta.Name])
		}
		got, err := svc.AccessOf(meta.Name)
		if err != nil {
			t.Fatalf("AccessOf(%q) error = %v", meta.Name, err)
		}
		if got != want[meta.Name] {
			t.Fatalf("AccessOf(%q) = %q, want %q", meta.Name, got, want[meta.Name])
		}
	}

	for _, name := range skills.SystemNames() {
		got, err := svc.AccessOf(name)
		if err != nil {
			t.Fatalf("AccessOf(%q) error = %v", name, err)
		}
		if got != skills.AccessEnabled {
			t.Fatalf("AccessOf(%q) = %q, want enabled", name, got)
		}
	}
	for _, meta := range skills.SystemMeta() {
		if meta.Access != skills.AccessEnabled {
			t.Fatalf("SystemMeta() access of %q = %q, want enabled", meta.Name, meta.Access)
		}
	}
}
