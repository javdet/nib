package kbdoc

import (
	"errors"
	"log/slog"
	"time"

	"github.com/javdet/nib/internal/filestore"
	"github.com/javdet/nib/internal/repository"
)

// Source records where the returned document body came from.
type Source string

const (
	// SourceUploaded means the operator's last uploaded file was returned.
	SourceUploaded Source = "uploaded"
	// SourceTemplate means nothing has been uploaded for this collection and the
	// skeleton compiled into the binary was returned instead.
	SourceTemplate Source = "template"
)

// Document is the effective knowledge base source document for a collection.
type Document struct {
	Collection string
	Content    string
	Source     Source
	UpdatedAt  time.Time // zero for the template
}

// Service stores the original file behind each knowledge base collection as
// {dir}/{collection}.md on the data volume, so what was indexed can be read back
// and re-downloaded. The vector store stays the search path; this is the source
// of truth for the document text itself.
//
// A collection with no file falls back to the skeleton compiled into the binary.
// The skeleton is deliberately never written to disk: an install that has not
// uploaded anything keeps tracking the template shipped by the current image.
type Service struct {
	store *filestore.MarkdownStore
}

// NewService creates a Service. docsDir is the raw value from config
// knowledge_base.dir; it is resolved with ResolveDir against dataDir.
func NewService(dataDir, docsDir string) *Service {
	dir := ResolveDir(dataDir, docsDir)
	return &Service{
		store: filestore.NewMarkdownStore(dir, ".kbdoc-*.md", "knowledge document", ValidateName),
	}
}

// Dir returns the resolved document directory path.
func (s *Service) Dir() string {
	return s.store.Dir()
}

// Get returns the stored document for a collection, or the skeleton when none
// has been uploaded.
//
// An unreadable volume degrades to the skeleton with a warning rather than an
// error: viewing the knowledge base must keep working when the mount is sick,
// the same way a missing system prompt override falls back to the built-in one.
func (s *Service) Get(collection string) (Document, error) {
	if err := ValidateName(collection); err != nil {
		return Document{}, err
	}

	doc, err := s.store.Get(collection)
	switch {
	case err == nil:
		// A stat failure here is not worth failing the read over; the body is
		// already in hand and UpdatedAt is presentational.
		modTime, modErr := s.store.ModTime(collection)
		if modErr != nil {
			slog.Warn("knowledge document mtime unreadable",
				"collection", collection, "error", modErr)
		}
		return Document{
			Collection: collection,
			Content:    doc.Content,
			Source:     SourceUploaded,
			UpdatedAt:  modTime,
		}, nil
	case errors.Is(err, repository.ErrNotFound):
		// Expected for a collection that has never been uploaded to.
	default:
		slog.Warn("knowledge document unreadable, serving the built-in template",
			"collection", collection, "dir", s.store.Dir(), "error", err)
	}

	return Document{
		Collection: collection,
		Content:    Skeleton(),
		Source:     SourceTemplate,
	}, nil
}

// Save stores the uploaded document for a collection, replacing whatever was
// there before.
func (s *Service) Save(collection, content string) error {
	return s.store.Put(collection, content)
}
