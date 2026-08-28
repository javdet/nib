package skills

import "github.com/javdet/nib/internal/filestore"

const defaultRelDir = "skills"

// ResolveDir returns the directory used for skill .md files.
func ResolveDir(dataDir, skillsDir string) string {
	return filestore.ResolveDir(dataDir, skillsDir, defaultRelDir)
}
