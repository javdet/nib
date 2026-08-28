package service

import (
	"context"
	"os"
	"testing"

	"github.com/javdet/nib/internal/domain"
	"github.com/google/uuid"
)

func TestEnsureExecutorDialogCreatesAndReuses(t *testing.T) {
	dir := t.TempDir()
	planID := uuid.New()
	repo := &mapDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{}}
	svc := &ChatService{
		actionPlansDir: dir,
		dialogRepo:     repo,
	}

	ctx := context.Background()

	first, created, err := svc.EnsureExecutorDialog(ctx, planID)
	if err != nil {
		t.Fatalf("EnsureExecutorDialog first: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on first call")
	}
	if first.Mode != executeDialogMode {
		t.Fatalf("mode = %q, want %q", first.Mode, executeDialogMode)
	}
	if first.Title != executorDialogTitle {
		t.Fatalf("title = %q, want %q", first.Title, executorDialogTitle)
	}
	if first.ParentID == nil || *first.ParentID != planID {
		t.Fatalf("parentId = %v, want %v", first.ParentID, planID)
	}

	second, created, err := svc.EnsureExecutorDialog(ctx, planID)
	if err != nil {
		t.Fatalf("EnsureExecutorDialog second: %v", err)
	}
	if created {
		t.Fatal("expected created=false on second call")
	}
	if second.ID != first.ID {
		t.Fatalf("dialog id = %s, want %s", second.ID, first.ID)
	}

	execPath := actionPlanExecutorPath(dir, planID)
	if _, err := os.Stat(execPath); err != nil {
		t.Fatalf("executor pointer file missing: %v", err)
	}
}

func TestEnsureExecutorDialogRecreatesAfterDeletion(t *testing.T) {
	dir := t.TempDir()
	planID := uuid.New()
	repo := &mapDialogRepo{dialogs: map[uuid.UUID]domain.Dialog{}}
	svc := &ChatService{
		actionPlansDir: dir,
		dialogRepo:     repo,
	}

	ctx := context.Background()

	first, created, err := svc.EnsureExecutorDialog(ctx, planID)
	if err != nil {
		t.Fatalf("EnsureExecutorDialog first: %v", err)
	}
	if !created {
		t.Fatal("expected created=true on first call")
	}

	delete(repo.dialogs, first.ID)

	again, created, err := svc.EnsureExecutorDialog(ctx, planID)
	if err != nil {
		t.Fatalf("EnsureExecutorDialog after delete: %v", err)
	}
	if !created {
		t.Fatal("expected created=true after stored dialog was deleted")
	}
	if again.ID == first.ID {
		t.Fatal("expected a new dialog id after deletion")
	}

	storedID, found, err := svc.ReadActionPlanExecutorDialog(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanExecutorDialog: %v", err)
	}
	if !found {
		t.Fatal("expected executor pointer after recreation")
	}
	if storedID != again.ID {
		t.Fatalf("stored id = %s, want %s", storedID, again.ID)
	}
}

func TestReadWriteActionPlanExecutorDialog(t *testing.T) {
	dir := t.TempDir()
	planID := uuid.New()
	execID := uuid.New()
	svc := &ChatService{actionPlansDir: dir}

	if err := svc.WriteActionPlanExecutorDialog(planID, execID); err != nil {
		t.Fatalf("WriteActionPlanExecutorDialog: %v", err)
	}

	got, found, err := svc.ReadActionPlanExecutorDialog(planID)
	if err != nil {
		t.Fatalf("ReadActionPlanExecutorDialog: %v", err)
	}
	if !found {
		t.Fatal("expected found=true")
	}
	if got != execID {
		t.Fatalf("id = %s, want %s", got, execID)
	}
}
