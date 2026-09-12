package executor

import "strings"

// The canonical GIT_PROVIDER values the agent container branches on.
const (
	GitProviderGitHub = "github"
	GitProviderGitLab = "gitlab"
)

// NormalizeGitProvider maps the free-text "Version control system" company
// setting onto the two providers the agent container implements. The setting is
// an operator-typed string ("GitLab", "self-hosted Gitlab EE", ""), so matching
// is substring and case-insensitive, and anything unrecognised — blank
// included — is GitHub.
func NormalizeGitProvider(raw string) string {
	if strings.Contains(strings.ToLower(strings.TrimSpace(raw)), GitProviderGitLab) {
		return GitProviderGitLab
	}
	return GitProviderGitHub
}
