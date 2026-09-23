package service

import (
	"testing"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
)

func TestPlanSelectionRoundTrip(t *testing.T) {
	t.Parallel()

	svc := &ChatService{planSelectionDir: t.TempDir()}
	rootID := uuid.New()

	if _, found, err := svc.ReadPlanSelection(rootID); err != nil || found {
		t.Fatalf("ReadPlanSelection before write = found %v, err %v", found, err)
	}

	want := domain.Selection{Project: "kiss", Environment: "prod", Cloud: "aws", Location: "eu-west-1"}
	if err := svc.WritePlanSelection(rootID, want); err != nil {
		t.Fatalf("WritePlanSelection err = %v", err)
	}

	got, found, err := svc.ReadPlanSelection(rootID)
	if err != nil {
		t.Fatalf("ReadPlanSelection err = %v", err)
	}
	if !found {
		t.Fatal("ReadPlanSelection found = false after a write")
	}
	if got != want {
		t.Fatalf("ReadPlanSelection = %+v, want %+v", got, want)
	}
}

// TestPlanSelectionPrefersTheSnapshot is the whole point of recording it: the
// header selection is global and may have moved on by the time the plan ends.
func TestPlanSelectionPrefersTheSnapshot(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		planSelectionDir: t.TempDir(),
		selection:        selectionStoreFor("something-else"),
	}
	rootID := uuid.New()
	if err := svc.WritePlanSelection(rootID, domain.Selection{Project: "kiss"}); err != nil {
		t.Fatalf("WritePlanSelection err = %v", err)
	}

	if got := svc.planSelection(rootID).Project; got != "kiss" {
		t.Fatalf("planSelection().Project = %q, want %q", got, "kiss")
	}
}

// TestPlanSelectionFallsBackToTheLiveOne covers a plan created before the
// snapshot existed: it still resolves a collection rather than being skipped.
func TestPlanSelectionFallsBackToTheLiveOne(t *testing.T) {
	t.Parallel()

	svc := &ChatService{
		planSelectionDir: t.TempDir(),
		selection:        selectionStoreFor("kiss"),
	}

	if got := svc.planSelection(uuid.New()).Project; got != "kiss" {
		t.Fatalf("planSelection().Project = %q, want %q", got, "kiss")
	}
}

func TestPlanSelectionWithoutASelectionStore(t *testing.T) {
	t.Parallel()

	svc := &ChatService{planSelectionDir: t.TempDir()}
	if got := svc.planSelection(uuid.New()); got != (domain.Selection{}) {
		t.Fatalf("planSelection() = %+v, want the zero selection", got)
	}
}
