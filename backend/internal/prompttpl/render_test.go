package prompttpl

import (
	"strings"
	"testing"
)

func TestHasTemplate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{name: "plain", in: "You are a helpful assistant.", want: false},
		{name: "empty", in: "", want: false},
		{name: "single brace", in: "price is {1}", want: false},
		{name: "action", in: "hello {{ .global.Name }}", want: true},
		{name: "action no space", in: "{{.global.Name}}", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := HasTemplate(tc.in); got != tc.want {
				t.Fatalf("HasTemplate(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestRender_plainPassthrough(t *testing.T) {
	t.Parallel()
	const in = "You are a helpful assistant.\nNo variables here."
	got, err := Render(in, nil)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if got != in {
		t.Fatalf("Render passthrough: got %q, want %q", got, in)
	}
}

func TestRender_singleScope(t *testing.T) {
	t.Parallel()
	vars := map[string]map[string]string{
		"global": {"CompanyName": "AutomagicOps"},
	}
	got, err := Render("Company: {{ .global.CompanyName }}.", vars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	const want = "Company: AutomagicOps."
	if got != want {
		t.Fatalf("Render single scope: got %q, want %q", got, want)
	}
}

func TestRender_multipleScopes(t *testing.T) {
	t.Parallel()
	vars := map[string]map[string]string{
		"global": {"CompanyName": "AutomagicOps"},
		"team":   {"Channel": "#ops"},
	}
	const tpl = "{{ .global.CompanyName }} in {{ .team.Channel }}"
	got, err := Render(tpl, vars)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	const want = "AutomagicOps in #ops"
	if got != want {
		t.Fatalf("Render multiple scopes: got %q, want %q", got, want)
	}
}

func TestRender_missingVariable(t *testing.T) {
	t.Parallel()
	vars := map[string]map[string]string{
		"global": {"CompanyName": "AutomagicOps"},
	}
	_, err := Render("{{ .global.CompanyName }} {{ .global.Missing }}", vars)
	if err == nil {
		t.Fatal("Render: expected error for missing key, got nil")
	}
	msg := err.Error()
	if !strings.HasPrefix(msg, "prompttpl: render system prompt:") {
		t.Fatalf("error prefix: %q", msg)
	}
	if !strings.Contains(msg, `"Missing"`) {
		t.Fatalf("error should mention missing key name, got %q", msg)
	}
}

func TestRender_missingScope(t *testing.T) {
	t.Parallel()
	vars := map[string]map[string]string{
		"global": {"CompanyName": "AutomagicOps"},
	}
	_, err := Render("{{ .team.Channel }}", vars)
	if err == nil {
		t.Fatal("Render: expected error for missing scope, got nil")
	}
	if !strings.Contains(err.Error(), `"team"`) {
		t.Fatalf("error should mention missing scope name, got %q", err.Error())
	}
}

func TestRender_malformedTemplate(t *testing.T) {
	t.Parallel()
	_, err := Render("hello {{ .global.Name", nil)
	if err == nil {
		t.Fatal("Render: expected error for malformed template, got nil")
	}
	if !strings.HasPrefix(err.Error(), "prompttpl: render system prompt:") {
		t.Fatalf("error prefix: %q", err.Error())
	}
}

func TestRender_toYaml_sliceOfMaps(t *testing.T) {
	t.Parallel()
	data := map[string]interface{}{
		"items": []map[string]string{
			{"name": "a", "region": "us"},
			{"name": "b", "region": "eu"},
		},
	}
	got, err := Render(`{{ toYaml .items }}`, data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := strings.TrimSpace(`
- name: a
  region: us
- name: b
  region: eu
`)
	if strings.TrimSpace(got) != want {
		t.Fatalf("toYaml slice: got %q, want %q", got, want)
	}
}

func TestRender_toYamlNindent(t *testing.T) {
	t.Parallel()
	data := map[string]interface{}{
		"items": []map[string]string{{"k": "v"}},
	}
	got, err := Render(`block:{{ toYaml .items | nindent 2 }}`, data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := "block:\n  - k: v"
	if got != want {
		t.Fatalf("nindent: got %q, want %q", got, want)
	}
}

func TestRender_conditionalBuiltinCloud(t *testing.T) {
	t.Parallel()
	const tpl = `{{ if ne .builtin.Cloud "any" }}Limit the task scheduling scope to only cloud provider {{ .builtin.Cloud }}.{{ end }}`

	t.Run("specific cloud", func(t *testing.T) {
		t.Parallel()
		vars := map[string]map[string]string{
			"builtin": {"Cloud": "digitalocean"},
		}
		got, err := Render(tpl, vars)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		const want = "Limit the task scheduling scope to only cloud provider digitalocean."
		if got != want {
			t.Fatalf("conditional cloud: got %q, want %q", got, want)
		}
	})

	t.Run("any cloud", func(t *testing.T) {
		t.Parallel()
		vars := map[string]map[string]string{
			"builtin": {"Cloud": "any"},
		}
		got, err := Render(tpl, vars)
		if err != nil {
			t.Fatalf("Render: %v", err)
		}
		if got != "" {
			t.Fatalf("conditional any: got %q, want empty", got)
		}
	})
}

func TestRender_mergedGlobalAndProject(t *testing.T) {
	t.Parallel()
	data := map[string]interface{}{
		"global": map[string]string{"CompanyName": "ACME"},
		"project": map[string]interface{}{
			"name":         "demo",
			"clouds":       []interface{}{map[string]interface{}{"name": "aws"}},
			"environments": []interface{}{},
		},
	}
	tpl := `company={{ .global.CompanyName }} project={{ .project.name }}
clouds:
{{ toYaml .project.clouds | nindent 2 }}`
	got, err := Render(tpl, data)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	want := `company=ACME project=demo
clouds:

  - name: aws`
	if got != want {
		t.Fatalf("merged: got %q, want %q", got, want)
	}
}
