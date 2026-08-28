package skills_test

import (
	"testing"

	"github.com/javdet/nib/internal/skills"
)

func TestParseFrontmatter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		content  string
		wantName string
		wantDesc string
		wantCat  string
	}{
		{
			name: "with category included",
			content: `---
name: jira-todo-tasks
description: Retrieve todo tasks from Jira
category: included
---

body`,
			wantName: "jira-todo-tasks",
			wantDesc: "Retrieve todo tasks from Jira",
			wantCat:  skills.CategoryIncluded,
		},
		{
			name: "without category defaults searchable",
			content: `---
name: test-skill
description: A test skill
---

do something`,
			wantName: "test-skill",
			wantDesc: "A test skill",
			wantCat:  skills.CategorySearchable,
		},
		{
			name: "explicit searchable",
			content: `---
name: test-skill
description: A test skill
category: searchable
---`,
			wantName: "test-skill",
			wantDesc: "A test skill",
			wantCat:  skills.CategorySearchable,
		},
		{
			name: "invalid category defaults searchable",
			content: `---
name: test-skill
description: A test skill
category: unknown
---`,
			wantName: "test-skill",
			wantDesc: "A test skill",
			wantCat:  skills.CategorySearchable,
		},
		{
			name:     "no frontmatter",
			content:  "plain body",
			wantName: "",
			wantDesc: "",
			wantCat:  skills.CategorySearchable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := skills.ParseFrontmatter(tt.content)
			if got.Name != tt.wantName {
				t.Fatalf("Name = %q, want %q", got.Name, tt.wantName)
			}
			if got.Description != tt.wantDesc {
				t.Fatalf("Description = %q, want %q", got.Description, tt.wantDesc)
			}
			if got.Category != tt.wantCat {
				t.Fatalf("Category = %q, want %q", got.Category, tt.wantCat)
			}
		})
	}
}
