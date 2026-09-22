package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

// A successful run reports that it is done and nothing more: its text is working
// notes for the actions that follow, not something the operator has to read.
// Every other outcome is something somebody has to act on, so it keeps its body.
func TestFormatActionResultMessage(t *testing.T) {
	t.Parallel()

	const body = "created vpc-0a91f3 in eu-central-1"
	dialogID := uuid.New()

	tests := []struct {
		name     string
		status   ActionExecStatus
		body     string
		heading  string
		wantBody bool
	}{
		{
			name:     "a successful run keeps its result to itself",
			status:   ActionExecDone,
			body:     body,
			heading:  "**1.1 executed** ✅",
			wantBody: false,
		},
		{
			name:     "a blocked run shows what it needs",
			status:   ActionExecBlocked,
			body:     body,
			heading:  "**1.1 needs a decision** ⏸",
			wantBody: true,
		},
		{
			name:     "a failed run shows why",
			status:   ActionExecFailed,
			body:     body,
			heading:  "**1.1 failed** ❌",
			wantBody: true,
		},
		{
			name:     "a cancelled run shows what stopped it",
			status:   ActionExecCancelled,
			body:     body,
			heading:  "**1.1 cancelled** ⏹",
			wantBody: true,
		},
		{
			name:     "a run with no text is just its heading",
			status:   ActionExecFailed,
			body:     "",
			heading:  "**1.1 failed** ❌",
			wantBody: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := formatActionResultMessage("s0.step0", dialogID, tt.status, tt.body)

			if !strings.Contains(got, tt.heading) {
				t.Errorf("message = %q, want heading %q", got, tt.heading)
			}
			if strings.Contains(got, body) != tt.wantBody {
				t.Errorf("message = %q, body present = %v, want %v", got, !tt.wantBody, tt.wantBody)
			}
			// The transcript link is the operator's way into the detail, so it
			// survives every outcome -- and it is the whole answer for a
			// successful run.
			if !strings.Contains(got, "#dialog:"+dialogID.String()) {
				t.Errorf("message = %q, want a transcript link", got)
			}
		})
	}
}

// Without a dialog there is nothing to link to, and a dangling link would be
// worse than none.
func TestFormatActionResultMessageWithoutADialog(t *testing.T) {
	t.Parallel()

	got := formatActionResultMessage("rollback.0", uuid.Nil, ActionExecDone, "reverted")
	if strings.Contains(got, "#dialog:") {
		t.Errorf("message = %q, want no transcript link", got)
	}
	if !strings.Contains(got, "**R1 executed** ✅") {
		t.Errorf("message = %q", got)
	}
}

// "Done" on a check says the sub-agent finished, not that the check passed, so
// the finding has to survive into the chat -- an action's is held back, and a
// check reported as a bare tick would tell the operator nothing.
func TestFormatActionResultMessage_keepsACheckFinding(t *testing.T) {
	t.Parallel()

	const finding = "2/3 replicas ready, expected 3/3"

	got := formatActionResultMessage("s0.check0", uuid.Nil, ActionExecDone, finding)
	if !strings.Contains(got, "**1.C1 checked** 🔍") {
		t.Errorf("message = %q, want the check heading", got)
	}
	if !strings.Contains(got, finding) {
		t.Errorf("message = %q, want it to keep %q", got, finding)
	}

	// An action still reports as a tick alone: its text is working memory for the
	// agents that follow, not something the operator has to read.
	action := formatActionResultMessage("s0.step0", uuid.Nil, ActionExecDone, "created vpc-123")
	if strings.Contains(action, "created vpc-123") {
		t.Errorf("message = %q, want a finished action to hold its notes back", action)
	}
}

// The expectation is the whole point of running a check: without it the agent
// can quote what it saw but cannot say whether the check passed.
func TestBuildActionSeed_carriesACheckExpectation(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		summariesDir:     t.TempDir(),
		dagsDir:          t.TempDir(),
		planContractsDir: t.TempDir(),
		actionPlansDir:   t.TempDir(),
	}

	check := storedActionCheck{Check: "every pod is Running", Expectation: "3/3 ready"}
	seed, err := svc.buildActionSeed(uuid.New(), "s0.check0", "1.C1", check.asActionStep(nil))
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	for _, want := range []string{"Type: check", "every pod is Running", "### Expected result", "3/3 ready"} {
		if !strings.Contains(seed, want) {
			t.Errorf("seed = %q, want it to hold %q", seed, want)
		}
	}
}

// The agent carrying an action out is the one about to cause the outage, so the
// impact the planner declared reaches its seed rather than being left for it to
// infer from the action text.
func TestBuildActionSeed_carriesTheDeclaredImpact(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		step storedActionStep
		want string
	}{
		{
			name: "downtime",
			step: storedActionStep{Type: "shell", Action: "stop postgres", Downtime: "orders-db refuses every query for ~10 minutes"},
			want: "Downtime: orders-db refuses every query for ~10 minutes",
		},
		{
			name: "degraded",
			step: storedActionStep{Type: "shell", Action: "compact volumes", Degraded: "reads are slower while compaction runs"},
			want: "Degradation: reads are slower while compaction runs",
		},
		{
			// downtime supersedes degraded, so a model that set both against the
			// prompt's rule still gets one line, and it is the severe one.
			name: "both set",
			step: storedActionStep{Type: "shell", Action: "stop postgres", Downtime: "orders-db is down", Degraded: "reads are slower"},
			want: "Downtime: orders-db is down",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := &ChatService{
				summariesDir:     t.TempDir(),
				dagsDir:          t.TempDir(),
				planContractsDir: t.TempDir(),
				actionPlansDir:   t.TempDir(),
			}

			seed, err := svc.buildActionSeed(uuid.New(), "s0.step0", "1.1", tt.step)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if !strings.Contains(seed, tt.want) {
				t.Fatalf("seed = %q, want it to hold %q", seed, tt.want)
			}
			if tt.name == "both set" && strings.Contains(seed, "Degradation:") {
				t.Fatalf("seed = %q, downtime supersedes degraded", seed)
			}
		})
	}
}
