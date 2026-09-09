package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/javdet/nib/internal/repository"
)

const (
	// attachmentSweepInterval is how often the queue is drained. Orphaned bytes
	// are a disk-space problem, not a correctness one, so this is deliberately
	// unhurried.
	attachmentSweepInterval = 15 * time.Minute

	// attachmentSweepBatch caps one pass so a large dialog delete does not turn
	// into one enormous transaction of unlink calls.
	attachmentSweepBatch = 500

	// unsentAttachmentTTL is how long an upload that was never sent is kept.
	// Long enough to survive a composer left open over lunch.
	unsentAttachmentTTL = "24 hours"
)

// AttachmentSweeper deletes attachment files whose database rows are gone.
//
// chat_attachments is the only index of what lives under data/attachments, and
// both of its foreign keys cascade: deleting a dialog, or rewinding a
// conversation on a retry, destroys that index without any Go code running. An
// AFTER DELETE trigger captures each row's path on the way out, and this drains
// that queue.
type AttachmentSweeper struct {
	repo           repository.AttachmentRepository
	attachmentsDir string
}

func NewAttachmentSweeper(repo repository.AttachmentRepository, dataDir string) *AttachmentSweeper {
	return &AttachmentSweeper{
		repo:           repo,
		attachmentsDir: filepath.Join(dataDir, "attachments"),
	}
}

// Run sweeps once at startup and then on a ticker until ctx is cancelled.
func (s *AttachmentSweeper) Run(ctx context.Context) {
	if s == nil || s.repo == nil {
		return
	}
	ticker := time.NewTicker(attachmentSweepInterval)
	defer ticker.Stop()

	for {
		if err := s.Sweep(ctx); err != nil && !errors.Is(err, context.Canceled) {
			slog.Warn("attachment sweep", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// Sweep expires unsent uploads and then removes the files behind every queued
// deletion. A queue row is dropped only once its file is gone, so a crash
// mid-sweep repeats work rather than losing it.
func (s *AttachmentSweeper) Sweep(ctx context.Context) error {
	expired, err := s.repo.DeleteUnsentAttachments(ctx, unsentAttachmentTTL)
	if err != nil {
		return err
	}
	if expired > 0 {
		slog.Info("expired attachments that were uploaded but never sent", "count", expired)
	}

	removed := 0
	for {
		queued, err := s.repo.ClaimOrphanedAttachmentFiles(ctx, attachmentSweepBatch)
		if err != nil {
			return err
		}
		if len(queued) == 0 {
			break
		}

		done := make([]int64, 0, len(queued))
		for _, f := range queued {
			if err := s.removeFile(f.DialogID.String(), f.Path); err != nil {
				// Left queued so the next pass retries it; a permission problem
				// that never clears would otherwise be silently forgotten.
				slog.Warn("remove orphaned attachment file", "path", f.Path, "error", err)
				continue
			}
			done = append(done, f.ID)
		}
		if err := s.repo.DropOrphanedAttachmentFiles(ctx, done); err != nil {
			return err
		}
		removed += len(done)

		if len(queued) < attachmentSweepBatch {
			break
		}
	}
	if removed > 0 {
		slog.Info("removed orphaned attachment files", "count", removed)
	}
	return nil
}

// removeFile deletes one stored file and, when it was the last one, the dialog's
// directory. Only the base name of the stored path is trusted, matching how the
// read and delete paths rebuild it.
func (s *AttachmentSweeper) removeFile(dialogID, storedPath string) error {
	dir := filepath.Join(s.attachmentsDir, dialogID)
	if err := os.Remove(filepath.Join(dir, filepath.Base(storedPath))); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("remove attachment file: %w", err)
	}
	// Succeeds only while the directory is empty, which is exactly when it should
	// go; any other error means it is still in use and is not worth reporting.
	_ = os.Remove(dir)
	return nil
}
