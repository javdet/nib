package service

import (
	"encoding/json"
	"log/slog"
	"sort"
	"strings"
)

// stepCategoriesKey is the property carrying the tool categories of one action.
// It sits on a stage step and on a rollback entry alike, because both are handed
// to the same kind of execute sub-agent.
const stepCategoriesKey = "categories"

const stepCategoriesDescription = "Tool categories the sub-agent that executes this action will need. " +
	"They decide which MCP tools that sub-agent is given, so name every surface the action touches " +
	"and send an empty array when it needs none. A category left out is a tool the executor will not have."

const stepCategoryItemDescription = "Tool category name (e.g. kubernetes, monitoring, code-repository)."

// withStepCategories stamps the per-action `categories` property onto every
// step-shaped object of a plan tool schema, constrained to names.
//
// It is applied on top of the loaded schema instead of being left to
// seed/tools/schemas, for two reasons. Those files are copied to the data volume
// only when absent, so a property added to a seed would never reach an install
// that already has one. And the valid names come from the toolCategories
// variable, which a file on disk cannot follow.
//
// A document whose shape it does not recognise comes back unchanged, the same
// way the schema loaders fall back to their embedded literal.
func withStepCategories(params json.RawMessage, names []string) json.RawMessage {
	if len(params) == 0 {
		return params
	}

	var doc any
	if err := json.Unmarshal(params, &doc); err != nil {
		slog.Warn("action plan schema: parse for category injection", "error", err)
		return params
	}
	if !stampStepCategories(doc, names) {
		return params
	}

	out, err := json.Marshal(doc)
	if err != nil {
		slog.Warn("action plan schema: marshal after category injection", "error", err)
		return params
	}
	return json.RawMessage(out)
}

// stampStepCategories walks a JSON schema and gives every step-shaped object the
// categories property, reporting whether it found one.
func stampStepCategories(node any, names []string) bool {
	switch n := node.(type) {
	case map[string]any:
		found := false
		if isStepSchema(n) {
			setStepCategoriesProperty(n, names)
			found = true
		}
		for _, child := range n {
			if stampStepCategories(child, names) {
				found = true
			}
		}
		return found
	case []any:
		found := false
		for _, child := range n {
			if stampStepCategories(child, names) {
				found = true
			}
		}
		return found
	}
	return false
}

// isStepSchema reports whether a schema object describes a plan action. Both a
// stage step and a rollback entry are the object that carries `type` and
// `action`, and nothing else in the three plan schemas carries both -- which is
// what lets this survive an operator's edits to the files.
func isStepSchema(schema map[string]any) bool {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return false
	}
	_, hasType := props["type"]
	_, hasAction := props["action"]
	return hasType && hasAction
}

// setStepCategoriesProperty adds the property, or refreshes the enum of one the
// schema file already describes. The description on an existing property is left
// alone: it may be an operator's wording, and only the enum has to follow the
// toolCategories variable.
func setStepCategoriesProperty(schema map[string]any, names []string) {
	props, ok := schema["properties"].(map[string]any)
	if !ok {
		return
	}

	prop, ok := props[stepCategoriesKey].(map[string]any)
	if !ok {
		prop = map[string]any{
			"type":        "array",
			"description": stepCategoriesDescription,
		}
		props[stepCategoriesKey] = prop
	}

	item, ok := prop["items"].(map[string]any)
	if !ok {
		item = map[string]any{
			"type":        "string",
			"description": stepCategoryItemDescription,
		}
		prop["items"] = item
	}
	// An empty enum would forbid every value, so a catalog that cannot be read
	// widens the field rather than closing it.
	if len(names) > 0 {
		item["enum"] = names
	} else {
		delete(item, "enum")
	}

	schema["required"] = appendRequired(schema["required"], stepCategoriesKey)
}

// appendRequired adds name to a JSON Schema `required` list if it is not there.
// With no fallback in the executor an omitted array silently costs the sub-agent
// every MCP tool, so the model is made to state the empty case rather than left
// free to skip the field.
func appendRequired(raw any, name string) any {
	list, ok := raw.([]any)
	if !ok {
		return []any{name}
	}
	for _, item := range list {
		if s, ok := item.(string); ok && s == name {
			return list
		}
	}
	return append(list, name)
}

// normalizeActionPlanCategories normalises the categories of every action in a
// plan document, in place: names are lowercased and deduped, and ones the tool
// catalog does not know are dropped. It returns the dropped names so the tool
// result can say which, the way set_category answers an unknown name.
//
// A dropped name never fails the write. A stage is worth storing even when one
// of its categories was a guess, and the agent is told so it can correct it.
//
// An empty valid list means the categories could not be read, not that none
// exist, so nothing is dropped in that case.
func normalizeActionPlanCategories(node any, valid []string) []string {
	validSet := make(map[string]struct{}, len(valid))
	for _, name := range valid {
		validSet[strings.ToLower(strings.TrimSpace(name))] = struct{}{}
	}

	unknown := map[string]struct{}{}
	normalizeCategoriesNode(node, validSet, unknown)
	if len(unknown) == 0 {
		return nil
	}

	out := make([]string, 0, len(unknown))
	for name := range unknown {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func normalizeCategoriesNode(node any, valid map[string]struct{}, unknown map[string]struct{}) {
	switch n := node.(type) {
	case map[string]any:
		if raw, ok := n[stepCategoriesKey]; ok {
			kept := filterCategories(normalizeCategories(argStringSlice(raw)), valid, unknown)
			if len(kept) == 0 {
				delete(n, stepCategoriesKey)
			} else {
				n[stepCategoriesKey] = kept
			}
		}
		for _, child := range n {
			normalizeCategoriesNode(child, valid, unknown)
		}
	case []any:
		for _, child := range n {
			normalizeCategoriesNode(child, valid, unknown)
		}
	}
}

func filterCategories(names []string, valid map[string]struct{}, unknown map[string]struct{}) []string {
	if len(valid) == 0 {
		return names
	}
	kept := make([]string, 0, len(names))
	for _, name := range names {
		if _, ok := valid[name]; ok {
			kept = append(kept, name)
			continue
		}
		unknown[name] = struct{}{}
	}
	return kept
}

// unknownCategoryNote is the sentence appended to a plan tool's result when it
// dropped a category. Naming the valid set turns a silent drop into something
// the agent can fix on its next call.
func unknownCategoryNote(unknown, valid []string) string {
	if len(unknown) == 0 {
		return ""
	}
	note := " Dropped unknown categories: " + strings.Join(unknown, ", ") + "."
	if len(valid) > 0 {
		note += " Valid categories: " + strings.Join(valid, ", ") + "."
	}
	return note
}
