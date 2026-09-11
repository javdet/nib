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
