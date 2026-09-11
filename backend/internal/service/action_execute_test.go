package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func testActionPlan() storedActionPlan {
	return storedActionPlan{
		Stages: []storedActionStage{
			{
				Number: 1,
				Title:  "Prepare",
				Steps: []storedActionStep{
					{Type: "shell", Action: "check cluster"},
					{Type: "code", Action: "bump version", Repository: "ansible-roles", PRTitle: "chore: bump"},
				},
			},
			{
				Number: 2,
				Title:  "Apply",
				Steps: []storedActionStep{
					{Type: "code", Action: "roll out", Repository: "infra", PRTitle: "feat: roll out"},
				},
			},
		},
		Rollback: []storedActionStep{
			{Type: "code", Action: "revert", Repository: "infra", PRTitle: "revert: roll out"},
		},
	}
}

func TestFindActionPlanStep(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		key        string
		wantFound  bool
		wantAction string
	}{
		{name: "first stage first step", key: "s0.step0", wantFound: true, wantAction: "check cluster"},
		{name: "first stage second step", key: "s0.step1", wantFound: true, wantAction: "bump version"},
		{name: "second stage", key: "s1.step0", wantFound: true, wantAction: "roll out"},
		{name: "rollback", key: "rollback.0", wantFound: true, wantAction: "revert"},
		{name: "check key is not a step", key: "s0.check0"},
		{name: "step out of range", key: "s0.step9"},
		{name: "stage out of range", key: "s9.step0"},
		{name: "rollback out of range", key: "rollback.9"},
		{name: "empty key", key: ""},
		{name: "garbage key", key: "not-a-key"},
	}

	plan := testActionPlan()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			step, ok := findActionPlanStep(plan, tt.key)
			if ok != tt.wantFound {
				t.Fatalf("findActionPlanStep(%q) found = %v, want %v", tt.key, ok, tt.wantFound)
			}
			if tt.wantFound && step.Action != tt.wantAction {
				t.Fatalf("findActionPlanStep(%q) action = %q, want %q", tt.key, step.Action, tt.wantAction)
			}
		})
	}
}

func TestBuildActionRunPrompt(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		action  string
		comment string
		want    string
	}{
		{name: "no comment", action: "bump version", want: "bump version"},
		{name: "blank comment", action: "bump version", comment: "   ", want: "bump version"},
		{name: "with comment", action: "bump version", comment: "use 17.2", want: "bump version\n\n## Comment\n\nuse 17.2"},
		{name: "trims action", action: "  bump version\n", want: "bump version"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildActionRunPrompt(tt.action, tt.comment); got != tt.want {
				t.Fatalf("buildActionRunPrompt() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildExecuteDialogTitle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step storedActionStep
		want string
	}{
		{
			name: "uses pr title",
			step: storedActionStep{PRTitle: "chore: bump", Action: "bump the version"},
			want: "chore: bump",
		},
		{
			name: "falls back to first line of action",
			step: storedActionStep{Action: "bump the version\nin all roles"},
			want: "bump the version",
		},
		{
			name: "trims blank pr title",
			step: storedActionStep{PRTitle: "   ", Action: "bump the version"},
			want: "bump the version",
		},
		{
			name: "truncates to the title limit",
			step: storedActionStep{Action: strings.Repeat("x", executeDialogTitleMaxLen+10)},
			want: strings.Repeat("x", executeDialogTitleMaxLen),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildExecuteDialogTitle(tt.step); got != tt.want {
				t.Fatalf("buildExecuteDialogTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildActionTargetBranch(t *testing.T) {
	t.Parallel()

	dialogID := uuid.MustParse("0f2b9f5c-0f0e-4f7b-9d2e-2f2c7c9a1111")

	tests := []struct {
		name   string
		taskID string
		want   string
	}{
		{name: "with task id", taskID: "DO-236", want: "nib/DO-236"},
		{name: "trims task id", taskID: "  DO-236  ", want: "nib/DO-236"},
		{name: "falls back to chat id", taskID: "", want: "nib/" + dialogID.String()},
		{name: "blank task id falls back", taskID: "   ", want: "nib/" + dialogID.String()},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildActionTargetBranch(tt.taskID, dialogID); got != tt.want {
				t.Fatalf("buildActionTargetBranch() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestBuildActionRepoURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		baseURL    string
		repository string
		want       string
	}{
		{name: "plain", baseURL: "https://github.com/my-org", repository: "infra", want: "https://github.com/my-org/infra"},
		{name: "trailing slash on base", baseURL: "https://github.com/my-org/", repository: "infra", want: "https://github.com/my-org/infra"},
		{name: "leading slash on repository", baseURL: "https://github.com/my-org", repository: "/infra", want: "https://github.com/my-org/infra"},
		{name: "surrounding whitespace", baseURL: " https://github.com/my-org ", repository: " infra ", want: "https://github.com/my-org/infra"},
		{name: "nested repository path", baseURL: "https://gitlab.com", repository: "group/subgroup/infra", want: "https://gitlab.com/group/subgroup/infra"},
		{name: "full clone url is not appended", baseURL: "https://github.com/my-org", repository: "https://github.com/my-org/infra", want: "https://github.com/my-org/infra"},
		{name: "full clone url from another owner", baseURL: "https://github.com/my-org", repository: "https://github.com/other-org/infra.git", want: "https://github.com/other-org/infra.git"},
		{name: "full clone url with trailing slash", baseURL: "https://github.com/my-org", repository: " https://github.com/my-org/infra/ ", want: "https://github.com/my-org/infra"},
		{name: "ssh clone url", baseURL: "https://github.com/my-org", repository: "git@github.com:my-org/infra.git", want: "git@github.com:my-org/infra.git"},
		{name: "scheme-less repeat of the base", baseURL: "https://github.com/my-org", repository: "github.com/my-org/infra", want: "https://github.com/my-org/infra"},
		{name: "scheme-less repeat ignores case", baseURL: "https://github.com/My-Org", repository: "GitHub.com/my-org/infra", want: "https://github.com/My-Org/infra"},
		{name: "host prefix without a repository is appended", baseURL: "https://github.com/my-org", repository: "github.com/my-org", want: "https://github.com/my-org/github.com/my-org"},
		{name: "empty repository keeps the base", baseURL: "https://github.com/my-org", repository: "  ", want: "https://github.com/my-org"},
		{name: "empty base keeps the repository", baseURL: "", repository: "/my-org/infra", want: "my-org/infra"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := buildActionRepoURL(tt.baseURL, tt.repository); got != tt.want {
				t.Fatalf("buildActionRepoURL() = %q, want %q", got, tt.want)
			}
		})
	}
}
