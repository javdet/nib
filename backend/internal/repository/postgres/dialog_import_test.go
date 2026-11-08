package postgres

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

func TestDialogRepoImportDialogs(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)
	attachments := NewAttachmentRepo(pool)

	rootID, firstID, secondID, attID := uuid.New(), uuid.New(), uuid.New(), uuid.New()
	created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	err := repo.ImportDialogs(ctx, []domain.DialogImport{
		{
			Dialog: domain.Dialog{ID: rootID, Mode: "main", Title: "imported\x00 plan", Categories: []string{"k8s"}, CreatedAt: created},
			Messages: []domain.DialogMessage{
				{Role: "system", Content: "prompt", CreatedAt: created},
				{Role: "user", Content: "see file", CreatedAt: created.Add(time.Minute), Attachments: []domain.Attachment{{
					ID: attID, Filename: "a.png", ContentType: "image/png",
					Kind: domain.AttachmentKindImage, SizeBytes: 3, Path: "attachments/a.png",
				}}},
			},
		},
		// Two children created a second apart, imported out of order: ListChildren
		// has to follow the kept created_at, not the insertion order.
		{Dialog: domain.Dialog{ID: secondID, Mode: "plan", Title: "second", ParentID: &rootID, CreatedAt: created.Add(2 * time.Second)}},
		{Dialog: domain.Dialog{ID: firstID, Mode: "decompose", Title: "first", ParentID: &rootID, CreatedAt: created.Add(time.Second)}},
	})
	if err != nil {
		t.Fatalf("ImportDialogs: %v", err)
	}

	root, err := repo.GetDialog(ctx, rootID)
	if err != nil {
		t.Fatalf("GetDialog: %v", err)
	}
	if root.Title != "imported plan" {
		t.Errorf("Title = %q, want NUL stripped", root.Title)
	}
	if !root.CreatedAt.Equal(created) {
		t.Errorf("CreatedAt = %v, want the source's %v", root.CreatedAt, created)
	}
	// updated_at is the import time, which is what lists the plan first.
	if !root.UpdatedAt.After(created) {
		t.Errorf("UpdatedAt = %v, want the import time, after %v", root.UpdatedAt, created)
	}
	if len(root.Categories) != 1 || root.Categories[0] != "k8s" {
		t.Errorf("Categories = %q, want [k8s]", root.Categories)
	}

	children, err := repo.ListChildren(ctx, rootID)
	if err != nil {
		t.Fatalf("ListChildren: %v", err)
	}
	if len(children) != 2 || children[0].ID != firstID || children[1].ID != secondID {
		t.Fatalf("ListChildren = %v, want [first second] by created_at", dialogIDs(children))
	}

	msgs, err := repo.ListMessages(ctx, rootID)
	if err != nil {
		t.Fatalf("ListMessages: %v", err)
	}
	if len(msgs) != 2 || msgs[0].Seq != 0 || msgs[1].Seq != 1 {
		t.Fatalf("messages = %+v, want seqs 0 and 1", msgs)
	}
	if !msgs[1].CreatedAt.Equal(created.Add(time.Minute)) {
		t.Errorf("message CreatedAt = %v, want the source's", msgs[1].CreatedAt)
	}

	att, err := attachments.GetAttachment(ctx, rootID, attID)
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if att.MessageID == nil || *att.MessageID != msgs[1].ID {
		t.Fatalf("attachment MessageID = %v, want the row of the message carrying it (%d)", att.MessageID, msgs[1].ID)
	}

	// The next live message continues the imported sequence.
	next, err := repo.AppendMessage(ctx, rootID, domain.DialogMessage{Role: "user", Content: "more"})
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if next.Seq != 2 {
		t.Fatalf("seq after import = %d, want 2", next.Seq)
	}
}

func TestDialogRepoImportDialogsDefaultsMissingCreatedAt(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)

	id := uuid.New()
	start := time.Now().Add(-time.Minute)
	if err := repo.ImportDialogs(ctx, []domain.DialogImport{{
		Dialog:   domain.Dialog{ID: id, Mode: "main", Title: "no times"},
		Messages: []domain.DialogMessage{{Role: "user", Content: "x"}},
	}}); err != nil {
		t.Fatalf("ImportDialogs: %v", err)
	}
	d, err := repo.GetDialog(ctx, id)
	if err != nil {
		t.Fatalf("GetDialog: %v", err)
	}
	if d.CreatedAt.Before(start) {
		t.Fatalf("CreatedAt = %v, want now rather than the zero time", d.CreatedAt)
	}
	msgs, err := repo.ListMessages(ctx, id)
	if err != nil || len(msgs) != 1 {
		t.Fatalf("ListMessages = %v, %v, want one message", msgs, err)
	}
	if msgs[0].CreatedAt.Before(start) {
		t.Fatalf("message CreatedAt = %v, want now rather than the zero time", msgs[0].CreatedAt)
	}
}

// One bad row aborts the whole import: a half-imported plan is worse than none.
func TestDialogRepoImportDialogsIsAllOrNothing(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewDialogRepo(pool)

	existing := mustCreateDialog(t, repo, "main", "already here", nil)
	fresh := uuid.New()

	err := repo.ImportDialogs(ctx, []domain.DialogImport{
		{Dialog: domain.Dialog{ID: fresh, Mode: "main", Title: "new"}, Messages: []domain.DialogMessage{{Role: "user", Content: "x"}}},
		{Dialog: domain.Dialog{ID: existing.ID, Mode: "plan", Title: "collides", ParentID: &fresh}},
	})
	if err == nil {
		t.Fatal("ImportDialogs with a colliding id succeeded, want an error")
	}
	if _, err := repo.GetDialog(ctx, fresh); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetDialog of the first imported dialog = %v, want ErrNotFound after the rollback", err)
	}
	if n := countRows(t, pool, `SELECT count(*) FROM chat_dialog_messages WHERE dialog_id = $1`, fresh); n != 0 {
		t.Fatalf("messages left behind by a failed import = %d, want 0", n)
	}
}
