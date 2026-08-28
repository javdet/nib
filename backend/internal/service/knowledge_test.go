package service

import (
	"testing"

	"github.com/javdet/nib/internal/kb"
)

func TestKnowledgeService_resolveCollection(t *testing.T) {
	t.Parallel()

	svc := &KnowledgeService{defaultCollection: kb.DefaultCollectionName}

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{
			name:  "empty falls back to default",
			input: "",
			want:  kb.DefaultCollectionName,
		},
		{
			name:  "whitespace falls back to default",
			input: "   ",
			want:  kb.DefaultCollectionName,
		},
		{
			name:  "default explicit",
			input: "default",
			want:  "default",
		},
		{
			name:  "custom name",
			input: "infra-docs",
			want:  "infra-docs",
		},
		{
			name:  "dots underscores hyphens",
			input: "team_a.v2-beta",
			want:  "team_a.v2-beta",
		},
		{
			name:    "leading hyphen invalid",
			input:   "-bad",
			wantErr: ErrInvalidCollectionName,
		},
		{
			name:    "space invalid",
			input:   "bad name",
			wantErr: ErrInvalidCollectionName,
		},
		{
			name:    "too long invalid",
			input:   "abcdefghijklmnopqrstuvwxyzabcdefghijklmnopqrstuvwxyzabcdefghijklm",
			wantErr: ErrInvalidCollectionName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := svc.resolveCollection(tt.input)
			if tt.wantErr != nil {
				if err != tt.wantErr {
					t.Fatalf("resolveCollection(%q) error = %v, want %v", tt.input, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveCollection(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.want {
				t.Fatalf("resolveCollection(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
