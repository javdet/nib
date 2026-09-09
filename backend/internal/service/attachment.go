package service

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/javdet/nib/internal/atomicfile"
	"github.com/javdet/nib/internal/domain"
	"github.com/javdet/nib/internal/repository"
	"github.com/google/uuid"
)

const attachmentMaxBytes = 16 << 20 // 16 MiB

var (
	ErrUnsupportedAttachmentType = errors.New("unsupported attachment type")
	ErrAttachmentNotFound        = errors.New("attachment not found")
)

var textExtensions = map[string]struct{}{
	".txt": {}, ".md": {}, ".csv": {}, ".json": {}, ".log": {},
	".yaml": {}, ".yml": {}, ".xml": {}, ".html": {}, ".htm": {},
	".ts": {}, ".tsx": {}, ".js": {}, ".jsx": {}, ".py": {}, ".go": {},
	".sh": {}, ".sql": {}, ".toml": {}, ".ini": {}, ".cfg": {}, ".conf": {},
}

// SaveAttachment stores a file on disk and records metadata in the database.
func (s *ChatService) SaveAttachment(
	ctx context.Context,
	dialogID uuid.UUID,
	filename, contentType string,
	content []byte,
) (domain.Attachment, error) {
	if s.attachmentRepo == nil {
		return domain.Attachment{}, fmt.Errorf("save attachment: attachment repository is not configured")
	}

	filename = sanitizeAttachmentFilename(filename)
	if filename == "" {
		return domain.Attachment{}, fmt.Errorf("save attachment: filename is required")
	}
	if len(content) == 0 {
		return domain.Attachment{}, fmt.Errorf("save attachment: file is empty")
	}
	if int64(len(content)) > attachmentMaxBytes {
		return domain.Attachment{}, fmt.Errorf("save attachment: file exceeds maximum size of %d bytes", attachmentMaxBytes)
	}

	kind, ct, err := classifyAttachment(filename, contentType)
	if err != nil {
		return domain.Attachment{}, err
	}

	if _, err := s.dialogRepo.GetDialog(ctx, dialogID); err != nil {
		return domain.Attachment{}, fmt.Errorf("save attachment: %w", err)
	}

	id := uuid.New()
	storedName := id.String() + "_" + filename
	relPath := filepath.Join("attachments", dialogID.String(), storedName)
	dir := filepath.Join(s.attachmentsDir, dialogID.String())
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return domain.Attachment{}, fmt.Errorf("save attachment: create directory: %w", err)
	}

	dest := filepath.Join(s.attachmentsDir, dialogID.String(), storedName)
	if err := writeAttachmentFile(dest, content); err != nil {
		return domain.Attachment{}, err
	}

	a := domain.Attachment{
		ID:          id,
		DialogID:    dialogID,
		Filename:    filename,
		ContentType: ct,
		Kind:        kind,
		SizeBytes:   int64(len(content)),
		Path:        relPath,
		URL:         attachmentAPIURL(dialogID, id),
	}

	created, err := s.attachmentRepo.CreateAttachment(ctx, a)
	if err != nil {
		_ = os.Remove(dest)
		return domain.Attachment{}, fmt.Errorf("save attachment: %w", err)
	}
	created.URL = attachmentAPIURL(dialogID, created.ID)
	return created, nil
}

// ListAttachments returns all attachments for a dialog.
func (s *ChatService) ListAttachments(ctx context.Context, dialogID uuid.UUID) ([]domain.Attachment, error) {
	if s.attachmentRepo == nil {
		return nil, fmt.Errorf("list attachments: attachment repository is not configured")
	}
	attachments, err := s.attachmentRepo.ListAttachmentsByDialog(ctx, dialogID)
	if err != nil {
		return nil, fmt.Errorf("list attachments: %w", err)
	}
	for i := range attachments {
		attachments[i].URL = attachmentAPIURL(dialogID, attachments[i].ID)
	}
	return attachments, nil
}

// DeleteAttachment removes an attachment from disk and the database.
func (s *ChatService) DeleteAttachment(ctx context.Context, dialogID, attachmentID uuid.UUID) error {
	if s.attachmentRepo == nil {
		return fmt.Errorf("delete attachment: attachment repository is not configured")
	}

	a, err := s.attachmentRepo.GetAttachment(ctx, dialogID, attachmentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAttachmentNotFound
		}
		return fmt.Errorf("delete attachment: %w", err)
	}

	if err := s.attachmentRepo.DeleteAttachment(ctx, dialogID, attachmentID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return ErrAttachmentNotFound
		}
		return fmt.Errorf("delete attachment: %w", err)
	}

	// The row is gone first, so the AFTER DELETE trigger has already queued this
	// path. Removing it here is the fast path; if it fails, the sweeper retries
	// rather than the file being lost track of.
	absPath := filepath.Join(s.attachmentsDir, dialogID.String(), filepath.Base(a.Path))
	if err := os.Remove(absPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		slog.Warn("remove attachment file, left for the sweeper", "path", a.Path, "error", err)
	}
	return nil
}

// ReadAttachmentContent returns the raw bytes of an attachment file.
func (s *ChatService) ReadAttachmentContent(ctx context.Context, dialogID, attachmentID uuid.UUID) (domain.Attachment, []byte, error) {
	if s.attachmentRepo == nil {
		return domain.Attachment{}, nil, fmt.Errorf("read attachment: attachment repository is not configured")
	}

	a, err := s.attachmentRepo.GetAttachment(ctx, dialogID, attachmentID)
	if err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			return domain.Attachment{}, nil, ErrAttachmentNotFound
		}
		return domain.Attachment{}, nil, fmt.Errorf("read attachment: %w", err)
	}

	absPath := filepath.Join(s.attachmentsDir, dialogID.String(), filepath.Base(a.Path))
	content, err := os.ReadFile(absPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return domain.Attachment{}, nil, ErrAttachmentNotFound
		}
		return domain.Attachment{}, nil, fmt.Errorf("read attachment file: %w", err)
	}
	a.URL = attachmentAPIURL(dialogID, a.ID)
	return a, content, nil
}

func classifyAttachment(filename, contentType string) (domain.AttachmentKind, string, error) {
	ct := strings.TrimSpace(contentType)
	if ct == "" {
		ct = mime.TypeByExtension(filepath.Ext(filename))
	}
	if ct == "" {
		ct = "application/octet-stream"
	}

	if strings.HasPrefix(ct, "image/") {
		return domain.AttachmentKindImage, ct, nil
	}

	ext := strings.ToLower(filepath.Ext(filename))
	if strings.HasPrefix(ct, "text/") {
		return domain.AttachmentKindText, ct, nil
	}
	if _, ok := textExtensions[ext]; ok {
		if ct == "application/octet-stream" {
			ct = "text/plain"
		}
		return domain.AttachmentKindText, ct, nil
	}

	return "", "", fmt.Errorf("%w: %s (%s)", ErrUnsupportedAttachmentType, filename, ct)
}

func sanitizeAttachmentFilename(name string) string {
	name = filepath.Base(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, string(os.PathSeparator), "_")
	return name
}

func writeAttachmentFile(path string, content []byte) error {
	return atomicfile.Write(path, content)
}

func attachmentAPIURL(dialogID, attachmentID uuid.UUID) string {
	return fmt.Sprintf("/api/v1/dialogs/%s/attachments/%s", dialogID, attachmentID)
}

func (s *ChatService) EnrichMessagesWithAttachments(ctx context.Context, dialogID uuid.UUID, msgs []domain.DialogMessage) ([]domain.DialogMessage, error) {
	if s.attachmentRepo == nil || len(msgs) == 0 {
		return msgs, nil
	}
	attachments, err := s.attachmentRepo.ListAttachmentsByDialog(ctx, dialogID)
	if err != nil {
		return nil, fmt.Errorf("enrich messages: %w", err)
	}
	byMessage := groupAttachmentsByMessage(attachments)
	for i := range msgs {
		if atts, ok := byMessage[msgs[i].ID]; ok {
			for j := range atts {
				atts[j].URL = attachmentAPIURL(dialogID, atts[j].ID)
			}
			msgs[i].Attachments = atts
		}
	}
	return msgs, nil
}

func parseAttachmentIDs(ids []string) ([]uuid.UUID, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	out := make([]uuid.UUID, 0, len(ids))
	for _, raw := range ids {
		id, err := uuid.Parse(strings.TrimSpace(raw))
		if err != nil {
			return nil, fmt.Errorf("invalid attachment id %q: %w", raw, err)
		}
		out = append(out, id)
	}
	return out, nil
}
