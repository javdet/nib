package mcpconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// knownEntryFields are the ServerEntry fields this package models. Everything
// else in a server object belongs to another MCP client (Cursor's "type",
// Claude Desktop's "disabled", …) and is carried through untouched.
var knownEntryFields = []string{
	"url", "transport", "headers", "command", "args", "env", "description",
}

// rawDoc is mcp.json decoded just far enough to edit one server: the top-level
// keys and the server objects stay as raw JSON, so a structured edit rewrites
// only the entry it was given and leaves other keys — including ones this
// package knows nothing about — in place.
type rawDoc struct {
	root    map[string]json.RawMessage
	servers map[string]json.RawMessage
}

func parseRawDoc(content string) (rawDoc, error) {
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(stripJSONComments(content)), &root); err != nil {
		return rawDoc{}, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	servers := map[string]json.RawMessage{}
	if raw, ok := root["mcpServers"]; ok {
		if err := json.Unmarshal(raw, &servers); err != nil {
			return rawDoc{}, fmt.Errorf("%w: mcpServers: %v", ErrInvalidJSON, err)
		}
	}
	return rawDoc{root: root, servers: servers}, nil
}

func (d rawDoc) has(name string) bool {
	_, ok := d.servers[name]
	return ok
}

func (d rawDoc) delete(name string) {
	delete(d.servers, name)
}

// set writes entry over the server called name, keeping any fields of an
// existing entry that ServerEntry does not model.
func (d rawDoc) set(name string, entry ServerEntry) error {
	merged := map[string]json.RawMessage{}
	if existing, ok := d.servers[name]; ok {
		if err := json.Unmarshal(existing, &merged); err != nil {
			// An entry that is not an object has nothing worth preserving.
			merged = map[string]json.RawMessage{}
		}
	}
	for _, field := range knownEntryFields {
		delete(merged, field)
	}

	encoded, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal mcp server %q: %w", name, err)
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(encoded, &fields); err != nil {
		return fmt.Errorf("marshal mcp server %q: %w", name, err)
	}
	for field, value := range fields {
		merged[field] = value
	}

	out, err := json.Marshal(merged)
	if err != nil {
		return fmt.Errorf("marshal mcp server %q: %w", name, err)
	}
	d.servers[name] = out
	return nil
}

// format renders the document, pretty-printed the way the raw editor shows it.
func (d rawDoc) format() (string, error) {
	root := make(map[string]json.RawMessage, len(d.root)+1)
	for key, value := range d.root {
		root[key] = value
	}
	servers, err := json.Marshal(d.servers)
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	root["mcpServers"] = servers

	compact, err := json.Marshal(root)
	if err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	var indented bytes.Buffer
	if err := json.Indent(&indented, compact, "", jsonIndent); err != nil {
		return "", fmt.Errorf("marshal mcp config: %w", err)
	}
	return indented.String() + "\n", nil
}
