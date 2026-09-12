package executor

import "testing"

func TestNormalizeGitProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		raw  string
		want string
	}{
		{name: "blank defaults to github", raw: "", want: GitProviderGitHub},
		{name: "whitespace only", raw: "   ", want: GitProviderGitHub},
		{name: "github lowercase", raw: "github", want: GitProviderGitHub},
		{name: "github mixed case", raw: "GitHub", want: GitProviderGitHub},
		{name: "gitlab lowercase", raw: "gitlab", want: GitProviderGitLab},
		{name: "gitlab mixed case", raw: "GitLab", want: GitProviderGitLab},
		{name: "gitlab padded and shouting", raw: "  GITLAB  ", want: GitProviderGitLab},
		{name: "gitlab inside a sentence", raw: "self-hosted GitLab EE", want: GitProviderGitLab},
		{name: "unknown provider falls back to github", raw: "Bitbucket", want: GitProviderGitHub},
		{name: "gitea is not gitlab", raw: "gitea", want: GitProviderGitHub},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := NormalizeGitProvider(tt.raw); got != tt.want {
				t.Errorf("NormalizeGitProvider(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}
