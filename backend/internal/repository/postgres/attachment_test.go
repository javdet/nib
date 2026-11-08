package postgres

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
)

func mustCreateAttachment(t *testing.T, repo *AttachmentRepo, dialogID uuid.UUID, path string) domain.Attachment {
	t.Helper()
	a, err := repo.CreateAttachment(context.Background(), domain.Attachment{
		ID: uuid.New(), DialogID: dialogID, Filename: path, ContentType: "text/plain",
		Kind: domain.AttachmentKindText, SizeBytes: 5, Path: path,
	})
	if err != nil {
		t.Fatalf("CreateAttachment(%s): %v", path, err)
	}
	return a
}

func queuedPaths(t *testing.T, pool *pgxpool.Pool) []string {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT path FROM attachment_files_to_delete ORDER BY id`)
	if err != nil {
		t.Fatalf("query queue: %v", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			t.Fatalf("scan queue: %v", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate queue: %v", err)
	}
	return out
}

func TestAttachmentRepoScopedToItsDialog(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewAttachmentRepo(pool)
	mine, other := testDialog(t, pool), testDialog(t, pool)

	a := mustCreateAttachment(t, repo, mine, "a.txt")
	if a.MessageID != nil || a.Kind != domain.AttachmentKindText || a.Path != "a.txt" {
		t.Fatalf("CreateAttachment = %+v, want an unlinked text attachment", a)
	}
	mustCreateAttachment(t, repo, other, "b.txt")

	got, err := repo.GetAttachment(ctx, mine, a.ID)
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if got.ID != a.ID || got.SizeBytes != 5 {
		t.Fatalf("GetAttachment = %+v, want %+v", got, a)
	}
	// Knowing an id is not enough: another dialog cannot read or delete it.
	if _, err := repo.GetAttachment(ctx, other, a.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("GetAttachment from another dialog = %v, want ErrNotFound", err)
	}
	if err := repo.DeleteAttachment(ctx, other, a.ID); !errors.Is(err, repository.ErrNotFound) {
		t.Fatalf("DeleteAttachment from another dialog = %v, want ErrNotFound", err)
	}

	list, err := repo.ListAttachmentsByDialog(ctx, mine)
	if err != nil {
		t.Fatalf("ListAttachmentsByDialog: %v", err)
	}
	if len(list) != 1 || list[0].ID != a.ID {
		t.Fatalf("ListAttachmentsByDialog = %+v, want only a.txt", list)
	}

	if _, err := repo.CreateAttachment(ctx, domain.Attachment{
		ID: uuid.New(), DialogID: mine, Filename: "x", Kind: "video", Path: "x",
	}); err == nil {
		t.Fatal("CreateAttachment with an unknown kind succeeded, want the kind CHECK to refuse it")
	}
}

func TestAttachmentRepoLinkAttachmentsToMessage(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewAttachmentRepo(pool)
	dialogs := NewDialogRepo(pool)
	d, other := testDialog(t, pool), testDialog(t, pool)

	msg, err := dialogs.AppendMessage(ctx, d, domain.DialogMessage{Role: "user", Content: "files"})
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	a := mustCreateAttachment(t, repo, d, "a.txt")
	b := mustCreateAttachment(t, repo, d, "b.txt")
	foreign := mustCreateAttachment(t, repo, other, "c.txt")

	if err := repo.LinkAttachmentsToMessage(ctx, d, msg.ID, nil); err != nil {
		t.Fatalf("LinkAttachmentsToMessage(none) = %v, want nil", err)
	}
	if err := repo.LinkAttachmentsToMessage(ctx, d, msg.ID, []uuid.UUID{a.ID}); err != nil {
		t.Fatalf("LinkAttachmentsToMessage: %v", err)
	}
	got, err := repo.GetAttachment(ctx, d, a.ID)
	if err != nil {
		t.Fatalf("GetAttachment: %v", err)
	}
	if got.MessageID == nil || *got.MessageID != msg.ID {
		t.Fatalf("MessageID = %v, want %d", got.MessageID, msg.ID)
	}

	tests := []struct {
		name string
		ids  []uuid.UUID
	}{
		{name: "already linked", ids: []uuid.UUID{b.ID, a.ID}},
		{name: "another dialog's", ids: []uuid.UUID{b.ID, foreign.ID}},
		{name: "unknown", ids: []uuid.UUID{b.ID, uuid.New()}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := repo.LinkAttachmentsToMessage(ctx, d, msg.ID, tt.ids); err == nil {
				t.Fatal("LinkAttachmentsToMessage succeeded, want an error")
			}
			// b is first in every batch: the failure must roll its link back.
			got, err := repo.GetAttachment(ctx, d, b.ID)
			if err != nil {
				t.Fatalf("GetAttachment: %v", err)
			}
			if got.MessageID != nil {
				t.Fatalf("b.MessageID = %d after a failed batch, want it still unlinked", *got.MessageID)
			}
		})
	}
}

// Every way a row goes — deleted outright, cascaded with its dialog, or swept as
// an abandoned upload — must leave its path in the queue, or the file leaks.
func TestAttachmentRepoQueuesFilesOfDeletedRows(t *testing.T) {
	pool := testPool(t)
	ctx := context.Background()
	repo := NewAttachmentRepo(pool)
	dialogs := NewDialogRepo(pool)
	d, gone := testDialog(t, pool), testDialog(t, pool)

	direct := mustCreateAttachment(t, repo, d, "direct.txt")
	mustCreateAttachment(t, repo, gone, "cascaded.txt")
	stale := mustCreateAttachment(t, repo, d, "stale.txt")
	fresh := mustCreateAttachment(t, repo, d, "fresh.txt")
	sentOld := mustCreateAttachment(t, repo, d, "sent-old.txt")

	msg, err := dialogs.AppendMessage(ctx, d, domain.DialogMessage{Role: "user", Content: "x"})
	if err != nil {
		t.Fatalf("AppendMessage: %v", err)
	}
	if err := repo.LinkAttachmentsToMessage(ctx, d, msg.ID, []uuid.UUID{sentOld.ID}); err != nil {
		t.Fatalf("LinkAttachmentsToMessage: %v", err)
	}
	if _, err := pool.Exec(ctx,
		`UPDATE chat_attachments SET created_at = now() - interval '2 hours' WHERE id = ANY($1)`,
		[]uuid.UUID{stale.ID, sentOld.ID}); err != nil {
		t.Fatalf("backdate attachments: %v", err)
	}

	if err := repo.DeleteAttachment(ctx, d, direct.ID); err != nil {
		t.Fatalf("DeleteAttachment: %v", err)
	}
	if err := dialogs.DeleteDialog(ctx, gone); err != nil {
		t.Fatalf("DeleteDialog: %v", err)
	}
	// Only an unsent upload past the age is swept; a sent one is kept however old.
	n, err := repo.DeleteUnsentAttachments(ctx, "1 hour")
	if err != nil {
		t.Fatalf("DeleteUnsentAttachments: %v", err)
	}
	if n != 1 {
		t.Fatalf("DeleteUnsentAttachments removed %d rows, want 1", n)
	}
	for _, keep := range []uuid.UUID{fresh.ID, sentOld.ID} {
		if _, err := repo.GetAttachment(ctx, d, keep); err != nil {
			t.Fatalf("GetAttachment(%s) after the sweep: %v, want it kept", keep, err)
		}
	}

	if got, want := queuedPaths(t, pool), []string{"direct.txt", "cascaded.txt", "stale.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("queued paths = %q, want %q", got, want)
	}

	claimed, err := repo.ClaimOrphanedAttachmentFiles(ctx, 2)
	if err != nil {
		t.Fatalf("ClaimOrphanedAttachmentFiles: %v", err)
	}
	if len(claimed) != 2 || claimed[0].Path != "direct.txt" || claimed[1].Path != "cascaded.txt" {
		t.Fatalf("ClaimOrphanedAttachmentFiles(2) = %+v, want the two oldest", claimed)
	}
	if claimed[1].DialogID != gone {
		t.Fatalf("claimed DialogID = %s, want the deleted dialog %s", claimed[1].DialogID, gone)
	}
	// Claiming does not dequeue: a crash before the files are removed costs a repeat.
	if got := queuedPaths(t, pool); len(got) != 3 {
		t.Fatalf("queue after a claim = %q, want all three still queued", got)
	}

	if err := repo.DropOrphanedAttachmentFiles(ctx, nil); err != nil {
		t.Fatalf("DropOrphanedAttachmentFiles(none) = %v, want nil", err)
	}
	if err := repo.DropOrphanedAttachmentFiles(ctx, []int64{claimed[0].ID, claimed[1].ID}); err != nil {
		t.Fatalf("DropOrphanedAttachmentFiles: %v", err)
	}
	if got, want := queuedPaths(t, pool), []string{"stale.txt"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("queue after a drop = %q, want %q", got, want)
	}
}
