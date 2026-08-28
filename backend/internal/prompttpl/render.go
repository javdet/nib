// Package prompttpl renders system prompts through text/template using
// prompt_variables loaded from Postgres.
//
// Variables are grouped by scope. The top-level template data can be
// map[scope]map[name]value, so a template like `{{ .global.CompanyName }}`
// resolves to the row (scope='global', name='CompanyName'). Callers may pass
// richer structures (e.g. a merged map including `project`).
package prompttpl

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// templateDelim is the action delimiter used by text/template. Its presence in
// a raw prompt is a cheap signal that rendering is required.
const templateDelim = "{{"

// HasTemplate reports whether s contains a text/template action delimiter.
// Callers use it to skip DB connection and rendering for plain prompts.
func HasTemplate(s string) bool {
	return strings.Contains(s, templateDelim)
}

func tplFuncs() template.FuncMap {
	return template.FuncMap{
		"toYaml": toYAML,
		"nindent": func(spaces int, s string) string {
			return "\n" + indent(spaces, s)
		},
	}
}

func indent(spaces int, s string) string {
	pad := strings.Repeat(" ", spaces)
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = pad + line
	}
	return strings.Join(lines, "\n")
}

// toYAML marshals v to YAML and trims a single trailing newline. On error it
// returns an empty string (Helm-compatible behavior).
func toYAML(v interface{}) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(string(b), "\n")
}

// Render executes tpl as a text/template with data as the dot value. Helper
// functions toYaml and nindent mirror Helm/Sprig-style composition (e.g.
// {{ toYaml .foo | nindent 2 }}).
//
// Missing keys abort rendering with an error that preserves the unknown key
// name reported by text/template (e.g. `map has no entry for key "CompanyName"`).
func Render(tpl string, data interface{}) (string, error) {
	t, err := template.New("system").Funcs(tplFuncs()).Option("missingkey=error").Parse(tpl)
	if err != nil {
		return "", fmt.Errorf("prompttpl: render system prompt: %w", err)
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("prompttpl: render system prompt: %w", err)
	}
	return buf.String(), nil
}
