package systemprompts

import "github.com/javdet/nib/internal/filestore"

const defaultRelDir = "prompts"

// ResolveDir returns the directory used for system prompt .md files.
func ResolveDir(dataDir, promptsDir string) string {
	return filestore.ResolveDir(dataDir, promptsDir, defaultRelDir)
}
