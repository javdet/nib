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
