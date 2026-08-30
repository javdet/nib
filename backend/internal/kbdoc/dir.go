package kbdoc

import "github.com/javdet/nib/internal/filestore"

const defaultRelDir = "knowledgebase"

// ResolveDir returns the directory holding the last uploaded source document per
// collection ({dir}/{collection}.md).
func ResolveDir(dataDir, docsDir string) string {
	return filestore.ResolveDir(dataDir, docsDir, defaultRelDir)
}
