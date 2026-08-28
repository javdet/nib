package mcpconfig

// stripJSONComments removes // line comments and /* */ block comments from JSONC
// content while preserving comment-like sequences inside string literals.
func stripJSONComments(content string) string {
	inString := false
	escaped := false
	out := make([]byte, 0, len(content))

	for i := 0; i < len(content); i++ {
		ch := content[i]

		if inString {
			out = append(out, ch)
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == '"' {
				inString = false
			}
			continue
		}

		if ch == '"' {
			inString = true
			out = append(out, ch)
			continue
		}

		if ch == '/' && i+1 < len(content) {
			next := content[i+1]
			if next == '/' {
				i += 2
				for i < len(content) && content[i] != '\n' {
					i++
				}
				if i < len(content) {
					out = append(out, '\n')
				}
				continue
			}
			if next == '*' {
				i += 2
				for i+1 < len(content) && !(content[i] == '*' && content[i+1] == '/') {
					i++
				}
				if i+1 < len(content) {
					i++
				}
				continue
			}
		}

		out = append(out, ch)
	}

	return string(out)
}
