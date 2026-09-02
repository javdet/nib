package service

import (
	"strconv"
	"testing"
)

func TestParseActionPlanNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		input  string
		want   string
		wantOK bool
	}{
		{name: "first step", input: "1.1", want: "s0.step0", wantOK: true},
		{name: "later step", input: "3.2", want: "s2.step1", wantOK: true},
		{name: "check", input: "1.C1", want: "s0.check0", wantOK: true},
		{name: "check lowercase", input: "2.c3", want: "s1.check2", wantOK: true},
		{name: "rollback", input: "R1", want: "rollback.0", wantOK: true},
		{name: "rollback lowercase", input: "r4", want: "rollback.3", wantOK: true},

		// A model quotes what it sees, so tolerate the shapes it actually emits.
		{name: "surrounding space", input: "  1.1  ", want: "s0.step0", wantOK: true},
		{name: "leading hash", input: "#1.2", want: "s0.step1", wantOK: true},
		{name: "hash and space", input: "# R2", want: "rollback.1", wantOK: true},
		{name: "zero padded check", input: "1.C01", want: "s0.check0", wantOK: true},
		{name: "zero padded stage", input: "01.1", want: "s0.step0", wantOK: true},

		// Row keys pass through unchanged.
		{name: "step key", input: "s0.step0", want: "s0.step0", wantOK: true},
		{name: "check key", input: "s2.check1", want: "s2.check1", wantOK: true},
		{name: "rollback key", input: "rollback.0", want: "rollback.0", wantOK: true},

		// Labels are one-based on both halves, so a zero is not a label this
		// plan could have produced.
		{name: "stage zero", input: "0.1", wantOK: false},
		{name: "index zero", input: "1.0", wantOK: false},
		{name: "check index zero", input: "1.C0", wantOK: false},
		{name: "rollback zero", input: "R0", wantOK: false},

		{name: "empty", input: "", wantOK: false},
		{name: "whitespace", input: "   ", wantOK: false},
		{name: "prose", input: "the first one", wantOK: false},
		{name: "stage alone", input: "1", wantOK: false},
		{name: "three parts", input: "1.1.1", wantOK: false},
		{name: "negative", input: "-1.1", wantOK: false},
		{name: "not a number", input: "x.y", wantOK: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, ok := parseActionPlanNumber(tt.input)
			if ok != tt.wantOK {
				t.Fatalf("parseActionPlanNumber(%q) ok = %v, want %v", tt.input, ok, tt.wantOK)
			}
			if ok && got != tt.want {
				t.Errorf("parseActionPlanNumber(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

// The parser must invert the numbering exactly, or a number the operator sees
// would execute a different row than the one they pressed.
func TestParseActionPlanNumberRoundTrip(t *testing.T) {
	t.Parallel()

	for stage := 0; stage < 4; stage++ {
		for index := 0; index < 4; index++ {
			for _, scope := range []ActionPlanScope{ActionPlanScopeSteps, ActionPlanScopeChecks} {
				number := actionPlanItemNumber(stage, scope, index)
				wantKey := actionPlanItemKey(stage, scope, index)

				got, ok := parseActionPlanNumber(number)
				if !ok {
					t.Fatalf("parseActionPlanNumber(%q) failed for stage %d %s %d", number, stage, scope, index)
				}
				if got != wantKey {
					t.Errorf("parseActionPlanNumber(%q) = %q, want %q", number, got, wantKey)
				}
			}
		}

		number := actionPlanRollbackNumber(stage)
		got, ok := parseActionPlanNumber(number)
		if !ok {
			t.Fatalf("parseActionPlanNumber(%q) failed for rollback %d", number, stage)
		}
		if want := "rollback." + strconv.Itoa(stage); got != want {
			t.Errorf("parseActionPlanNumber(%q) = %q, want %q", number, got, want)
		}
	}
}
