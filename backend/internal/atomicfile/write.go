package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Write atomically writes data to path using a temporary file in the same directory.
//
// Both the file and its parent directory are fsynced: without that, a host or node
// crash can lose a rename that already returned success, which would silently
// discard a saved system prompt override. These writes are all low-frequency
// (prompts, rules, skills, mcp.json, dialog artifacts), so the two extra syncs cost
// nothing measurable.
func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".atomic-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("sync temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp file: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("sync directory %s: %w", dir, err)
	}
	return nil
}

// syncDir flushes the directory entry created by the rename. Some container
// overlay filesystems reject fsync on a directory; those refusals are not a
// durability failure we can act on, so they are ignored.
func syncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()

	if err := d.Sync(); err != nil {
		if errors.Is(err, syscall.ENOTSUP) || errors.Is(err, syscall.EINVAL) {
			return nil
		}
		return err
	}
	return nil
}

// WriteString atomically writes a string to path.
func WriteString(path, content string) error {
	return Write(path, []byte(content))
}
