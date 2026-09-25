package toolschema

import (
	"encoding/json"
	"reflect"
	"testing"
)

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func TestSanitize(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want map[string]any
	}{
		{
			name: "absent schema becomes an empty object schema",
			in:   ``,
			want: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name: "unparseable schema falls back",
			in:   `not json`,
			want: map[string]any{"type": "object", "properties": map[string]any{}},
		},
		{
			name: "unsupported keywords are dropped",
			in: `{"$schema":"https://json-schema.org/draft/2020-12/schema","$id":"x",
			      "type":"object","additionalProperties":false,
			      "properties":{"path":{"type":"string","format":"uri"}}}`,
			want: map[string]any{
				"type":       "object",
				"properties": map[string]any{"path": map[string]any{"type": "string"}},
			},
		},
		{
			name: "local $ref is inlined and $defs removed",
			in: `{"type":"object","properties":{"target":{"$ref":"#/$defs/Repo"}},
			      "$defs":{"Repo":{"type":"object","properties":{"url":{"type":"string"}}}}}`,
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target": map[string]any{
						"type":       "object",
						"properties": map[string]any{"url": map[string]any{"type": "string"}},
					},
				},
			},
		},
		{
			name: "a $ref sibling description survives the inline",
			in: `{"type":"object","properties":{"target":{"$ref":"#/$defs/R","description":"where"}},
			      "$defs":{"R":{"type":"string"}}}`,
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"target": map[string]any{"type": "string", "description": "where"},
				},
			},
		},
		{
			name: "unresolvable $ref degrades to an open object",
			in:   `{"type":"object","properties":{"x":{"$ref":"#/$defs/Missing"}}}`,
			want: map[string]any{
				"type":       "object",
				"properties": map[string]any{"x": map[string]any{"type": "object"}},
			},
		},
		{
			name: "oneOf becomes anyOf",
			in:   `{"type":"object","properties":{"v":{"oneOf":[{"type":"string"},{"type":"number"}]}}}`,
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"v": map[string]any{"anyOf": []any{
						map[string]any{"type": "string"},
						map[string]any{"type": "number"},
					}},
				},
			},
		},
		{
			name: "nullable anyOf collapses to the single branch",
			in:   `{"type":"object","properties":{"v":{"anyOf":[{"type":"string"},{"type":"null"}],"description":"opt"}}}`,
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"v": map[string]any{"type": "string", "description": "opt"},
				},
			},
		},
		{
			name: "allOf merges into the parent",
			in: `{"type":"object","allOf":[
			        {"type":"object","properties":{"a":{"type":"string"}},"required":["a"]},
			        {"type":"object","properties":{"b":{"type":"number"}}}]}`,
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"a": map[string]any{"type": "string"},
					"b": map[string]any{"type": "number"},
				},
				"required": []string{"a"},
			},
		},
		{
			name: "const becomes a single-element enum",
			in:   `{"type":"object","properties":{"kind":{"const":"code"}}}`,
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"kind": map[string]any{"type": "string", "enum": []any{"code"}},
				},
			},
		},
		{
			name: "missing type is inferred",
			in: `{"properties":{"o":{"properties":{"x":{"type":"string"}}},
			                    "a":{"items":{"type":"string"}},
			                    "e":{"enum":["one","two"]}}}`,
			want: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"o": map[string]any{"type": "object",
						"properties": map[string]any{"x": map[string]any{"type": "string"}}},
					"a": map[string]any{"type": "array",
						"items": map[string]any{"type": "string"}},
					"e": map[string]any{"type": "string", "enum": []any{"one", "two"}},
				},
			},
		},
		{
			name: "required naming an absent property is pruned",
			in:   `{"type":"object","properties":{"a":{"type":"string"}},"required":["a","ghost"]}`,
			want: map[string]any{
				"type":       "object",
				"properties": map[string]any{"a": map[string]any{"type": "string"}},
				"required":   []string{"a"},
			},
		},
		{
			name: "root is always an object with properties",
			in:   `{"type":"string"}`,
			want: map[string]any{"type": "object", "properties": map[string]any{}},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := Sanitize(json.RawMessage(tc.in))
			if mustJSON(t, got) != mustJSON(t, tc.want) {
				t.Errorf("Sanitize()\n got = %s\nwant = %s", mustJSON(t, got), mustJSON(t, tc.want))
			}
		})
	}
}

// TestSanitizeCyclicRefTerminates is the guard that keeps a self-referential
// schema -- which real MCP servers publish for tree-shaped arguments -- from
// recursing forever.
func TestSanitizeCyclicRefTerminates(t *testing.T) {
	t.Parallel()

	in := `{"type":"object","properties":{"node":{"$ref":"#/$defs/Node"}},
	        "$defs":{"Node":{"type":"object","properties":{"child":{"$ref":"#/$defs/Node"}}}}}`

	got := Sanitize(json.RawMessage(in))
	if got["type"] != "object" {
		t.Fatalf("root type = %v, want object", got["type"])
	}
	// It must terminate and still describe the outer level.
	props, _ := got["properties"].(map[string]any)
	if _, has := props["node"]; !has {
		t.Errorf("node property was lost: %s", mustJSON(t, got))
	}
}

// TestSanitizeCapsDepth keeps a pathological published schema from being sent
// at whatever depth its generator produced.
func TestSanitizeCapsDepth(t *testing.T) {
	t.Parallel()

	in := `{"type":"object","properties":{"a":`
	for i := 0; i < 30; i++ {
		in += `{"type":"object","properties":{"a":`
	}
	in += `{"type":"string"}`
	for i := 0; i < 31; i++ {
		in += `}}`
	}
	in += `}`

	got := Sanitize(json.RawMessage(in))
	if depth(got) > maxDepth+2 {
		t.Errorf("depth = %d, want it capped near %d", depth(got), maxDepth)
	}
}

func depth(node any) int {
	m, ok := node.(map[string]any)
	if !ok {
		return 0
	}
	deepest := 0
	for _, v := range m {
		if d := depth(v); d > deepest {
			deepest = d
		}
		if props, ok := v.(map[string]any); ok {
			for _, sub := range props {
				if d := depth(sub); d > deepest {
					deepest = d
				}
			}
		}
	}
	return deepest + 1
}

// TestSanitizeIsIdempotent matters because the same ToolDef is converted on
// every round of a turn.
func TestSanitizeIsIdempotent(t *testing.T) {
	t.Parallel()

	in := `{"type":"object","properties":{"v":{"oneOf":[{"type":"string"},{"type":"null"}]}},
	        "required":["v"]}`
	once := Sanitize(json.RawMessage(in))
	raw, err := json.Marshal(once)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	twice := Sanitize(raw)
	if !reflect.DeepEqual(once, twice) {
		t.Errorf("Sanitize is not idempotent:\nonce  = %s\ntwice = %s",
			mustJSON(t, once), mustJSON(t, twice))
	}
}
