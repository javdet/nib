package rules

import "github.com/javdet/nib/internal/filestore"

const defaultRelDir = "rules"

// ResolveDir returns the directory used for rule .md files.
func ResolveDir(dataDir, rulesDir string) string {
	return filestore.ResolveDir(dataDir, rulesDir, defaultRelDir)
}
