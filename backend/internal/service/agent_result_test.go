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
