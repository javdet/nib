package filestore

import "strings"

// BuildDocument renders a markdown document from frontmatter metadata and a
// body, the same shape the Skills and Rules editors write: a name/description
// frontmatter block followed by a blank line and the body.
//
// It is the write side of ParseFrontmatter and mirrors the frontend's
// buildContent (src/lib/frontmatter.ts), so a document written here and one
// saved from the interface are indistinguishable.
func BuildDocument(meta Meta, body string) string {
	var sb strings.Builder
	sb.WriteString("---\nname: ")
	sb.WriteString(quoteYAMLValue(meta.Name))
	sb.WriteString("\ndescription: ")
	sb.WriteString(quoteYAMLValue(meta.Description))
	sb.WriteString("\n---")

	body = strings.TrimLeft(body, "\n")
	if body == "" {
		return sb.String()
	}
	sb.WriteString("\n\n")
	sb.WriteString(body)
	return sb.String()
}

// quoteYAMLValue double-quotes a scalar whose plain form would not survive a
// round trip through unquoteYAMLValue or a real YAML parser.
func quoteYAMLValue(value string) string {
	if !needsQuoting(value) {
		return value
	}
	escaped := strings.ReplaceAll(value, `\`, `\\`)
	escaped = strings.ReplaceAll(escaped, `"`, `\"`)
	return `"` + escaped + `"`
}

func needsQuoting(value string) bool {
	if value == "" {
		return true
	}
	if strings.ContainsAny(value, ":#\n\"'") {
		return true
	}
	if strings.TrimSpace(value) != value {
		return true
	}
	return strings.ContainsAny(value[:1], "&*!|>@[]{},`")
}
