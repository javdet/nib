package mcpconfig

import (
	"encoding/json"
	"fmt"
	"strings"
)

// serverLikeFields mark an object as an attempt at a server definition. A key
// carrying any of them is one somebody meant the agent to call.
var serverLikeFields = []string{
	"url", "command", "type", "transport", "headers", "args", "env",
}

// Warnings reports what a valid mcp.json says that nothing will act on. The
// file still saves — these are notes for the editor, not errors — because the
// alternative, refusing content we merely find suspicious, is what makes a
// config editor feel like it is fighting you.
func Warnings(content string) []string {
	keys, values, err := topLevelFields(content)
	if err != nil {
		return nil
	}

	var warnings []string
	var stray []string
	for _, key := range keys {
		if key == "mcpServers" {
			continue
		}
		if looksLikeServer(values[key]) {
			stray = append(stray, key)
		}
	}
	if len(stray) == 1 {
		warnings = append(warnings, fmt.Sprintf(
			"%s looks like an MCP server but sits outside \"mcpServers\", so nothing reads it. Move it inside \"mcpServers\" to use it.",
			quoteList(stray),
		))
	} else if len(stray) > 1 {
		warnings = append(warnings, fmt.Sprintf(
			"%s look like MCP servers but sit outside \"mcpServers\", so nothing reads them. Move them inside \"mcpServers\" to use them.",
			quoteList(stray),
		))
	}

	if _, ok := values["mcpServers"]; !ok {
		warnings = append(warnings, `No "mcpServers" object, so no MCP servers are configured.`)
	}
	return warnings
}

// topLevelFields returns the root object's keys in the order they were written,
// so warnings read in the same order as the file.
func topLevelFields(content string) ([]string, map[string]json.RawMessage, error) {
	dec := json.NewDecoder(strings.NewReader(stripJSONComments(content)))
	tok, err := dec.Token()
	if err != nil {
		return nil, nil, err
	}
	if delim, ok := tok.(json.Delim); !ok || delim != '{' {
		return nil, nil, fmt.Errorf("%w: root value must be an object", ErrInvalidJSON)
	}

	var keys []string
	values := map[string]json.RawMessage{}
	for dec.More() {
		tok, err := dec.Token()
		if err != nil {
			return nil, nil, err
		}
		key, ok := tok.(string)
		if !ok {
			return nil, nil, fmt.Errorf("%w: object key must be a string", ErrInvalidJSON)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, nil, err
		}
		if _, seen := values[key]; !seen {
			keys = append(keys, key)
		}
		values[key] = value
	}
	return keys, values, nil
}

func looksLikeServer(value json.RawMessage) bool {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(value, &fields); err != nil {
		return false
	}
	for _, field := range serverLikeFields {
		if _, ok := fields[field]; ok {
			return true
		}
	}
	return false
}

func quoteList(items []string) string {
	quoted := make([]string, len(items))
	for i, item := range items {
		quoted[i] = fmt.Sprintf("%q", item)
	}
	switch len(quoted) {
	case 1:
		return quoted[0]
	case 2:
		return quoted[0] + " and " + quoted[1]
	default:
		return strings.Join(quoted[:len(quoted)-1], ", ") + ", and " + quoted[len(quoted)-1]
	}
}
