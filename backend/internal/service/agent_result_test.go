package service

import (
	"strings"
	"testing"
)

func TestFormatAgentResultMessageSuccess(t *testing.T) {
	t.Parallel()

	got := formatAgentResultMessage(AgentRunResult{
		Status:       "success",
		ExitCode:     0,
		Result:       "Updated postgres_version to 17.",
		TargetBranch: "nib/DO-236",
		PRURL:        "https://github.com/org/repo/pull/42",
		DurationMS:   125000,
		NumTurns:     8,
		TotalCostUSD: 0.42,
	})

	if !strings.Contains(got, "Updated postgres_version to 17.") {
		t.Fatalf("missing result body: %q", got)
	}
	if !strings.Contains(got, "https://github.com/org/repo/pull/42") {
		t.Fatalf("missing PR url: %q", got)
	}
	if !strings.Contains(got, "`nib/DO-236`") {
		t.Fatalf("missing branch: %q", got)
	}
	if strings.Contains(got, "Agent run failed") {
		t.Fatalf("unexpected failure banner: %q", got)
	}
}

func TestFormatAgentResultMessageFailure(t *testing.T) {
	t.Parallel()

	got := formatAgentResultMessage(AgentRunResult{
		Status:   "failed",
		ExitCode: 1,
		Result:   "Could not apply the change.",
		LogTail:  "error: permission denied",
	})

	if !strings.Contains(got, "Agent run failed") {
		t.Fatalf("missing failure banner: %q", got)
	}
	if !strings.Contains(got, "Could not apply the change.") {
		t.Fatalf("missing result body: %q", got)
	}
	if !strings.Contains(got, "permission denied") {
		t.Fatalf("missing log tail: %q", got)
	}
}

func TestAgentRunMessageName(t *testing.T) {
	t.Parallel()

	if got := agentRunMessageName("nib-12345678"); got != "agent-run:nib-12345678" {
		t.Fatalf("agentRunMessageName() = %q", got)
	}
	if got := agentRunMessageName("  "); got != "" {
		t.Fatalf("agentRunMessageName(blank) = %q, want empty", got)
	}
}

// What a code action leaves behind for the actions after it. Deliberately not
// the operator-facing message: the cost, the turn count and the log tail are
// noise to an agent, while the branch is a value that lives nowhere else.
func TestCodeActionNoteText(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		res    AgentRunResult
		want   string
		absent []string
	}{
		{
			name: "the result and the branch, which nothing else records",
			res: AgentRunResult{
				Status:       "success",
				Result:       "Bumped the chart to 1.4.2.",
				TargetBranch: "nib/DO-236",
				PRURL:        "https://github.com/org/repo/pull/42",
				TotalCostUSD: 0.42,
				NumTurns:     8,
			},
			want: "Bumped the chart to 1.4.2.\n\nBranch: nib/DO-236",
			// The pull request is written onto the action as pr_url, which
			// get_action_list already returns beside the note.
			absent: []string{"pull/42", "0.42", "8 turns"},
		},
		{
			name: "a failed run still says how far it got",
			res: AgentRunResult{
				Status:   "failed",
				ExitCode: 1,
				Result:   "Edited the values file, then the test suite failed.",
			},
			want: "Edited the values file, then the test suite failed.",
		},
		{
			name: "nothing to report is nothing to record",
			res:  AgentRunResult{Status: "failed", ExitCode: 1},
			want: "",
		},
		{
			name: "a branch with no result text is still worth keeping",
			res:  AgentRunResult{Status: "success", TargetBranch: "nib/DO-240"},
			want: "Branch: nib/DO-240",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := codeActionNoteText(tt.res)
			if got != tt.want {
				t.Errorf("note = %q, want %q", got, tt.want)
			}
			for _, s := range tt.absent {
				if strings.Contains(got, s) {
					t.Errorf("note = %q, should not carry %q", got, s)
				}
			}
		})
	}
}
