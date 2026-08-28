package skills

import (
	"strings"
)

const (
	CategorySearchable = "searchable"
	CategoryIncluded   = "included"
)

// Meta holds parsed frontmatter metadata for a skill.
type Meta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Category    string `json:"category"`
}

// ParseFrontmatter extracts name, description, and category from skill file content.
// Category defaults to searchable when missing or invalid.
func ParseFrontmatter(content string) Meta {
	result := Meta{Category: CategorySearchable}

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
		case "category":
			result.Category = normalizeCategory(value)
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

func normalizeCategory(category string) string {
	switch strings.TrimSpace(strings.ToLower(category)) {
	case CategoryIncluded:
		return CategoryIncluded
	default:
		return CategorySearchable
	}
}
