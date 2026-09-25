// Package toolschema rewrites the JSON Schema of a tool's parameters onto the
// subset every OpenAI-compatible provider accepts.
//
// It exists because nib passes published schemas straight through to the
// model. Local tools carry keywords OpenAI tolerates, and MCP servers publish
// whatever their SDK generates -- $ref, $defs, anyOf, format. A provider with a
// stricter validator does not reject the offending tool, it rejects the whole
// request, which takes down every turn that offers that tool rather than only
// the calls that use it.
//
// The rules are a deny-list rather than an allow-list on purpose: the accepted
// subset is a fairly rich slice of OpenAPI, and an allow-list would also strip
// the minimum/pattern/enum hints the permissive providers use well.
package toolschema

import (
	"encoding/json"
)

// maxDepth caps how deep a rewritten schema may nest. Providers impose their
// own limits, and inlining a $ref can multiply depth, so a pathological
// published schema is truncated rather than sent.
const maxDepth = 12

// droppedKeywords are removed wherever they appear. Each is either unsupported
// by at least one target provider or meaningless once the rest has been
// rewritten.
var droppedKeywords = map[string]bool{
	"$schema":               true,
	"$id":                   true,
	"$anchor":               true,
	"$comment":              true,
	"$defs":                 true,
	"definitions":           true,
	"not":                   true,
	"if":                    true,
	"then":                  true,
	"else":                  true,
	"patternProperties":     true,
	"dependentSchemas":      true,
	"dependentRequired":     true,
	"unevaluatedProperties": true,
	"unevaluatedItems":      true,
	"prefixItems":           true,
	"contains":              true,
	"propertyNames":         true,
	"additionalItems":       true,
	"format":                true,
	// additionalProperties is dropped rather than coerced to false: as a
	// schema it declares an open map, which has no equivalent in the stricter
	// providers' function-call subset, and as a bool it is redundant there.
	"additionalProperties": true,
}

// Sanitize rewrites raw into a parameters object safe to send to any provider.
// An absent or unparseable schema yields the bare object schema, which is what
// the converters fell back to before.
func Sanitize(raw json.RawMessage) map[string]any {
	fallback := map[string]any{"type": "object", "properties": map[string]any{}}
	if len(raw) == 0 {
		return fallback
	}
	var root map[string]any
	if err := json.Unmarshal(raw, &root); err != nil || len(root) == 0 {
		return fallback
	}

	defs := collectDefs(root)
	out, ok := sanitizeNode(root, defs, map[string]bool{}, 0).(map[string]any)
	if !ok || len(out) == 0 {
		return fallback
	}

	// The parameters of a function are always an object, whatever the schema
	// claimed, and a provider that requires `properties` gets an empty one
	// rather than a missing key.
	out["type"] = "object"
	if _, has := out["properties"]; !has {
		out["properties"] = map[string]any{}
	}
	return out
}

// collectDefs indexes the root's $defs and definitions so local $refs can be
// inlined. Only root-level definitions are indexed, which is where every
// generator nib has met puts them.
func collectDefs(root map[string]any) map[string]map[string]any {
	defs := map[string]map[string]any{}
	for _, key := range []string{"$defs", "definitions"} {
		block, ok := root[key].(map[string]any)
		if !ok {
			continue
		}
		for name, node := range block {
			if schema, ok := node.(map[string]any); ok {
				defs["#/"+key+"/"+name] = schema
			}
		}
	}
	return defs
}

// sanitizeNode rewrites one schema node. resolving carries the $refs already
// being expanded on this path, so a cycle degrades to an open object instead
// of recursing forever.
func sanitizeNode(node any, defs map[string]map[string]any, resolving map[string]bool, depth int) any {
	switch typed := node.(type) {
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, sanitizeNode(item, defs, resolving, depth))
		}
		return out
	case map[string]any:
		if depth >= maxDepth {
			return map[string]any{"type": "string"}
		}
		return sanitizeObject(typed, defs, resolving, depth)
	default:
		return node
	}
}

func sanitizeObject(node map[string]any, defs map[string]map[string]any, resolving map[string]bool, depth int) map[string]any {
	node = inlineRef(node, defs, resolving, depth)

	out := map[string]any{}
	for key, value := range node {
		if droppedKeywords[key] {
			continue
		}
		switch key {
		case "properties":
			props, ok := value.(map[string]any)
			if !ok {
				continue
			}
			rewritten := map[string]any{}
			for name, sub := range props {
				rewritten[name] = sanitizeNode(sub, defs, resolving, depth+1)
			}
			out["properties"] = rewritten
		case "oneOf":
			// anyOf is accepted where oneOf is not, and the distinction --
			// exactly one branch versus at least one -- does not survive into
			// a function-call schema anyway.
			out["anyOf"] = sanitizeNode(value, defs, resolving, depth+1)
		case "const":
			out["enum"] = []any{value}
		case "allOf":
			// Merged into the parent below, not carried.
		default:
			out[key] = sanitizeNode(value, defs, resolving, depth+1)
		}
	}

	if branches, ok := node["allOf"].([]any); ok {
		mergeAllOf(out, branches, defs, resolving, depth)
	}
	collapseNullableAnyOf(out)
	ensureType(out)
	pruneRequired(out)
	return out
}

// inlineRef replaces a node carrying a local $ref with the definition it names,
// keeping any sibling keywords, which JSON Schema 2020-12 allows beside a $ref.
func inlineRef(node map[string]any, defs map[string]map[string]any, resolving map[string]bool, depth int) map[string]any {
	ref, ok := node["$ref"].(string)
	if !ok {
		return node
	}
	target, found := defs[ref]
	// An unresolvable or already-expanding ref becomes an open object: the
	// argument still type-checks, and the model reads the shape off the
	// description instead.
	if !found || resolving[ref] || depth >= maxDepth {
		merged := map[string]any{"type": "object"}
		copySiblings(merged, node)
		return merged
	}

	resolving[ref] = true
	expanded := sanitizeObject(target, defs, resolving, depth)
	delete(resolving, ref)

	merged := map[string]any{}
	for k, v := range expanded {
		merged[k] = v
	}
	copySiblings(merged, node)
	return merged
}

// copySiblings carries the keys written beside a $ref onto the expansion,
// without letting them overwrite what the definition already said.
func copySiblings(dst, src map[string]any) {
	for k, v := range src {
		if k == "$ref" {
			continue
		}
		if _, exists := dst[k]; !exists {
			dst[k] = v
		}
	}
}

// mergeAllOf folds object branches into the parent: their properties are
// shallow-merged and their required names unioned. A branch that is not an
// object schema is dropped, since there is nothing to merge.
func mergeAllOf(out map[string]any, branches []any, defs map[string]map[string]any, resolving map[string]bool, depth int) {
	props, _ := out["properties"].(map[string]any)
	if props == nil {
		props = map[string]any{}
	}
	required := toStringSlice(out["required"])

	for _, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		clean := sanitizeObject(branch, defs, resolving, depth)
		if sub, ok := clean["properties"].(map[string]any); ok {
			for name, schema := range sub {
				if _, exists := props[name]; !exists {
					props[name] = schema
				}
			}
		}
		required = appendMissing(required, toStringSlice(clean["required"]))
	}

	if len(props) > 0 {
		out["properties"] = props
		if _, has := out["type"]; !has {
			out["type"] = "object"
		}
	}
	if len(required) > 0 {
		out["required"] = required
	}
}

// collapseNullableAnyOf splices `anyOf: [X, {"type":"null"}]` -- the usual way
// a generator spells an optional field -- down to X, which the stricter
// providers accept where the union does not.
func collapseNullableAnyOf(out map[string]any) {
	branches, ok := out["anyOf"].([]any)
	if !ok || len(branches) == 0 {
		return
	}
	var kept []map[string]any
	for _, raw := range branches {
		branch, ok := raw.(map[string]any)
		if !ok {
			return
		}
		if branch["type"] == "null" {
			continue
		}
		kept = append(kept, branch)
	}
	if len(kept) != 1 {
		return
	}
	delete(out, "anyOf")
	for k, v := range kept[0] {
		// The parent's own description wins: it is the one written for this
		// field rather than for the shared definition behind it.
		if _, exists := out[k]; !exists {
			out[k] = v
		}
	}
}

// ensureType supplies a type for a node that has none. Providers that validate
// strictly reject an untyped schema outright.
func ensureType(out map[string]any) {
	if _, has := out["type"]; has {
		return
	}
	if _, has := out["anyOf"]; has {
		return
	}
	switch {
	case out["properties"] != nil:
		out["type"] = "object"
	case out["items"] != nil:
		out["type"] = "array"
	case out["enum"] != nil:
		out["type"] = "string"
	case len(out) == 0:
		out["type"] = "string"
	default:
		out["type"] = "string"
	}
}

// pruneRequired drops required names with no matching property, which at least
// one provider rejects rather than ignoring.
func pruneRequired(out map[string]any) {
	names := toStringSlice(out["required"])
	if len(names) == 0 {
		delete(out, "required")
		return
	}
	props, ok := out["properties"].(map[string]any)
	if !ok {
		delete(out, "required")
		return
	}
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if _, exists := props[name]; exists {
			kept = append(kept, name)
		}
	}
	if len(kept) == 0 {
		delete(out, "required")
		return
	}
	out["required"] = kept
}

func toStringSlice(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func appendMissing(dst, src []string) []string {
	seen := make(map[string]bool, len(dst))
	for _, s := range dst {
		seen[s] = true
	}
	for _, s := range src {
		if !seen[s] {
			dst = append(dst, s)
			seen[s] = true
		}
	}
	return dst
}
