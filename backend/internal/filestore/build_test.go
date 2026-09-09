package filestore

import "testing"

func TestBuildDocumentRoundTrip(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		meta Meta
	}{
		{name: "plain", meta: Meta{Name: "postgres", Description: "How we run Postgres"}},
		{name: "colon", meta: Meta{Name: "postgres", Description: "Postgres: never expose 5432"}},
		{name: "quotes", meta: Meta{Name: "nginx", Description: `the "edge" proxy`}},
		{name: "hash", meta: Meta{Name: "nginx", Description: "ticket #42"}},
		{name: "backslash", meta: Meta{Name: "windows", Description: `paths like C:\srv`}},
		{name: "empty description", meta: Meta{Name: "bare", Description: ""}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ParseFrontmatter(BuildDocument(tt.meta, "body text"))
			if got.Name != tt.meta.Name {
				t.Fatalf("Name = %q, want %q", got.Name, tt.meta.Name)
			}
			if got.Description != tt.meta.Description {
				t.Fatalf("Description = %q, want %q", got.Description, tt.meta.Description)
			}
		})
	}
}

func TestBuildDocumentLayout(t *testing.T) {
	t.Parallel()

	const want = "---\nname: postgres\ndescription: How we run Postgres\n---\n\nnever expose 5432"
	got := BuildDocument(Meta{Name: "postgres", Description: "How we run Postgres"}, "\n\nnever expose 5432")
	if got != want {
		t.Fatalf("BuildDocument() = %q, want %q", got, want)
	}
}

// A body-less document must not end in a blank line the parser would then read
// as the start of the body.
func TestBuildDocumentWithoutBody(t *testing.T) {
	t.Parallel()

	const want = "---\nname: bare\ndescription: nothing yet\n---"
	if got := BuildDocument(Meta{Name: "bare", Description: "nothing yet"}, "\n"); got != want {
		t.Fatalf("BuildDocument() = %q, want %q", got, want)
	}
}
