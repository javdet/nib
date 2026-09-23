package filestore_test

import (
	"testing"

	"github.com/javdet/nib/internal/filestore"
)

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		wantName string
		wantDesc string
	}{
		{
			name: "with body",
			content: `---
name: jira-todo-tasks
description: Retrieve todo tasks from Jira
---

body`,
			wantName: "jira-todo-tasks",
			wantDesc: "Retrieve todo tasks from Jira",
		},
		{
			name: "without body",
			content: `---
name: test-skill
description: A test skill
---`,
			wantName: "test-skill",
			wantDesc: "A test skill",
		},
		{
			name: "unknown keys are ignored",
			content: `---
name: test-skill
description: A test skill
category: included
---

do something`,
			wantName: "test-skill",
			wantDesc: "A test skill",
		},
		{
			name:     "no frontmatter",
			content:  "plain body",
			wantName: "",
			wantDesc: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := filestore.ParseFrontmatter(tt.content)
			if got.Name != tt.wantName {
				t.Fatalf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Description != tt.wantDesc {
				t.Fatalf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
		})
	}
}

func TestParseFrontmatterFieldsKeepsEveryKey(t *testing.T) {
	t.Parallel()

	content := "---\nname: demo\naccess: \"explicit\"\n# a comment\nowner: ops\n---\nbody"
	got := filestore.ParseFrontmatterFields(content)
	for key, want := range map[string]string{"name": "demo", "access": "explicit", "owner": "ops"} {
		if got[key] != want {
			t.Fatalf("fields[%q] = %q, want %q", key, got[key], want)
		}
	}
	if len(got) != 3 {
		t.Fatalf("len(fields) = %d, want 3: %v", len(got), got)
	}

	if got := filestore.ParseFrontmatterFields("no frontmatter"); len(got) != 0 {
		t.Fatalf("fields without frontmatter = %v, want empty", got)
	}
}
