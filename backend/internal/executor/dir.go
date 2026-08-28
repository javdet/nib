package executor

import (
	"path/filepath"
	"strings"
)

const defaultFileName = "executor.json"

// ResolveFile returns the path used for the executor JSON config file.
// When executorFile is empty, the default is filepath.Join(dataDir, "executor.json").
// Absolute paths are used as-is; relative paths are joined under dataDir.
func ResolveFile(dataDir, executorFile string) string {
	executorFile = strings.TrimSpace(executorFile)
	if executorFile == "" {
		return filepath.Join(dataDir, defaultFileName)
	}
	if filepath.IsAbs(executorFile) {
		return executorFile
	}
	return filepath.Join(dataDir, executorFile)
}
