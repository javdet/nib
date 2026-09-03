package filestore

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Meta holds parsed frontmatter metadata for a markdown document.
type Meta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

// ParseFrontmatter extracts name and description from markdown file content.
func ParseFrontmatter(content string) Meta {
	var result Meta

	normalized := strings.TrimPrefix(content, "\uFEFF")
	if !strings.HasPrefix(normalized, "---") {
		return result
	}

	var afterOpen string
	switch {
	case strings.HasPrefix(normalized, "---\r\n"):
		afterOpen = normalized[5:]
	case strings.HasPrefix(normalized, "---\n"):
		afterOpen = normalized[4:]
	default:
		return result
	}

	block, ok := frontmatterBlock(afterOpen)
	if !ok {
		return result
	}

	for _, line := range strings.Split(block, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		colon := strings.Index(trimmed, ":")
		if colon < 0 {
			continue
		}
		key := strings.TrimSpace(trimmed[:colon])
		value := unquoteYAMLValue(strings.TrimSpace(trimmed[colon+1:]))
		switch key {
		case "name":
			result.Name = value
		case "description":
			result.Description = value
		}
	}
	return result
}

func frontmatterBlock(afterOpen string) (string, bool) {
	separators := []string{"\n---\n", "\n---\r\n", "\r\n---\n", "\r\n---\r\n"}
	best := -1
	for _, sep := range separators {
		i := strings.Index(afterOpen, sep)
		if i >= 0 && (best < 0 || i < best) {
			best = i
		}
	}
	if best >= 0 {
		return afterOpen[:best], true
	}

	for _, suffix := range []string{"\n---", "\r\n---"} {
		if strings.HasSuffix(afterOpen, suffix) {
			return afterOpen[:len(afterOpen)-len(suffix)], true
		}
	}
	return "", false
}

func unquoteYAMLValue(value string) string {
	if len(value) >= 2 {
		if value[0] == '"' && value[len(value)-1] == '"' {
			inner := value[1 : len(value)-1]
			inner = strings.ReplaceAll(inner, `\"`, `"`)
			return strings.ReplaceAll(inner, `\\`, `\`)
		}
		if value[0] == '\'' && value[len(value)-1] == '\'' {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// ListMeta returns the frontmatter metadata of every document, sorted by name.
// Name is always the basename on disk, whatever the frontmatter claims.
func (s *MarkdownStore) ListMeta() ([]Meta, error) {
	names, err := s.ListNames()
	if err != nil {
		return nil, err
	}
	metas := make([]Meta, 0, len(names))
	for _, name := range names {
		path := filepath.Join(s.Dir(), name+mdSuffix)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read %s %s: %w", s.entityLabel, name, err)
		}
		metas = append(metas, Meta{
			Name:        name,
			Description: ParseFrontmatter(string(data)).Description,
		})
	}
	return metas, nil
}
