package service

import (
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestReadActionPlanNotesMissingFile(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	notes, err := svc.readActionPlanNotes(uuid.New())
	if err != nil {
		t.Fatalf("readActionPlanNotes: %v", err)
	}
	if len(notes) != 0 {
		t.Fatalf("notes = %#v, want empty", notes)
	}
}

func TestWriteReadActionPlanNotesRoundTrip(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	planID := uuid.New()
	want := map[string]string{"s0.step0": "vpc-0a91f3", "rollback.0": "deleted vpc-0a91f3"}

	if err := svc.writeActionPlanNotes(planID, want); err != nil {
		t.Fatalf("writeActionPlanNotes: %v", err)
	}
	got, err := svc.readActionPlanNotes(planID)
	if err != nil {
		t.Fatalf("readActionPlanNotes: %v", err)
	}
	for key, text := range want {
		if got[key] != text {
			t.Errorf("notes[%q] = %q, want %q", key, got[key], text)
		}
	}
}

func TestRecordActionNote(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", maxActionNoteBytes*2)

	tests := []struct {
		name  string
		texts []string
		want  func(t *testing.T, got string, present bool)
	}{
		{
			name:  "blank text is not stored at all",
			texts: []string{"   \n\t "},
			want: func(t *testing.T, _ string, present bool) {
				if present {
					t.Error("a blank result was stored")
				}
			},
		},
		{
			name:  "surrounding whitespace is trimmed",
			texts: []string{"\n  created vpc-0a91f3  \n"},
			want: func(t *testing.T, got string, _ bool) {
				if got != "created vpc-0a91f3" {
					t.Errorf("note = %q", got)
				}
			},
		},
		{
			name:  "a rerun overwrites the previous attempt",
			texts: []string{"first attempt", "second attempt"},
			want: func(t *testing.T, got string, _ bool) {
				if got != "second attempt" {
					t.Errorf("note = %q, want the latest attempt", got)
				}
			},
		},
		{
			name:  "an over-long note is cut and says so",
			texts: []string{long},
			want: func(t *testing.T, got string, _ bool) {
				if len(got) > maxActionNoteBytes {
					t.Errorf("note is %d bytes, want at most %d", len(got), maxActionNoteBytes)
				}
				if !strings.HasSuffix(got, actionNoteTruncationMarker) {
					t.Errorf("note = %q, want the truncation marker", got)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			svc := execTestService(t)
			planID := uuid.New()
			for _, text := range tt.texts {
				svc.recordActionNote(planID, "s0.step0", text)
			}

			notes, err := svc.readActionPlanNotes(planID)
			if err != nil {
				t.Fatalf("readActionPlanNotes: %v", err)
			}
			got, present := notes["s0.step0"]
			tt.want(t, got, present)
		})
	}
}

// A note lands on the row it belongs to and nowhere else, which is what makes it
// safe to hand the whole map to get_action_list.
func TestRecordActionNoteKeepsRowsApart(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	planID := uuid.New()

	svc.recordActionNote(planID, "s0.step0", "first")
	svc.recordActionNote(planID, "s0.step1", "second")

	notes, err := svc.readActionPlanNotes(planID)
	if err != nil {
		t.Fatalf("readActionPlanNotes: %v", err)
	}
	if notes["s0.step0"] != "first" || notes["s0.step1"] != "second" {
		t.Fatalf("notes = %#v", notes)
	}
}

// recordActionNote takes the plan lock, and so does finishActionExecRun. The two
// run back to back on every finished action, so a lock left held by one would
// wedge the other -- and with it the single execution slot.
func TestRecordActionNoteDoesNotDeadlockWithExecRun(t *testing.T) {
	t.Parallel()

	svc := execTestService(t)
	planID := uuid.New()

	if _, err := svc.startActionExecRun(planID, "s0.step0"); err != nil {
		t.Fatalf("startActionExecRun: %v", err)
	}
	svc.finishActionExecRun(planID, "s0.step0", ActionExecDone, "")
	svc.recordActionNote(planID, "s0.step0", "done")

	notes, err := svc.readActionPlanNotes(planID)
	if err != nil {
		t.Fatalf("readActionPlanNotes: %v", err)
	}
	if notes["s0.step0"] != "done" {
		t.Fatalf("notes = %#v", notes)
	}
}

func TestActionPlanNotesPath(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	if got, want := actionPlanNotesPath("/data", id), "/data/"+id.String()+".notes.json"; got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}
