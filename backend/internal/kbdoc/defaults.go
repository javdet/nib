package kbdoc

import (
	"errors"
	"strings"

	"github.com/javdet/nib/internal/kbdoc/defaults"
)

const skeletonFile = "skeleton.md"

// skeletonContent is read once at package init from the embedded FS, so reads
// are lock-free and cannot fail at runtime. An empty value means the image was
// built without the template; ValidateEmbeddedSkeleton reports that at startup.
var skeletonContent = loadSkeleton()

func loadSkeleton() string {
	data, err := defaults.FS.ReadFile(skeletonFile)
	if err != nil {
		return ""
	}
	return string(data)
}

// Skeleton returns the knowledge base template compiled into the binary.
func Skeleton() string {
	return skeletonContent
}

// ValidateEmbeddedSkeleton reports whether the binary carries a usable template.
// main calls this at startup so an image built without it fails to boot rather
// than serving an empty document to every project.
func ValidateEmbeddedSkeleton() error {
	if strings.TrimSpace(skeletonContent) == "" {
		return errors.New("no knowledge base skeleton compiled into the binary")
	}
	return nil
}
