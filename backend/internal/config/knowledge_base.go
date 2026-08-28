package config

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	knowledgeBaseYAMLKey = "knowledge_base"
	knowledgeBaseURIKey  = "uri"
)

// LoadKnowledgeBaseURI reads knowledge_base.uri from a YAML file. It does not
// validate or require llm settings. Unlike [ReadKnowledgeBaseURI], it returns an
// error when the URI is missing or empty.
func LoadKnowledgeBaseURI(path string) (string, error) {
	uri, err := ReadKnowledgeBaseURI(path)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(uri) == "" {
		return "", fmt.Errorf("config: knowledge_base.uri is missing or empty in %s", path)
	}
	return uri, nil
}

// ReadKnowledgeBaseURI returns knowledge_base.uri from the YAML file at path.
// A missing file section or key yields "" with no error.
func ReadKnowledgeBaseURI(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("config: path is empty")
	}

	root, err := loadYAMLDocument(path)
	if err != nil {
		return "", err
	}

	mapping := documentRootMapping(root)
	if mapping == nil {
		return "", nil
	}

	kb := mappingChild(mapping, knowledgeBaseYAMLKey)
	if kb == nil || kb.Kind != yaml.MappingNode {
		return "", nil
	}

	uri := mappingChild(kb, knowledgeBaseURIKey)
	if uri == nil || uri.Kind != yaml.ScalarNode {
		return "", nil
	}

	return strings.TrimSpace(uri.Value), nil
}

// WriteKnowledgeBaseURI updates or inserts knowledge_base.uri in the YAML file at path
// using a yaml.Node read-modify-write so other sections and comments are preserved.
func WriteKnowledgeBaseURI(path, uri string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("config: path is empty")
	}

	root, err := loadYAMLDocument(path)
	if err != nil {
		return err
	}

	mapping := documentRootMapping(root)
	if mapping == nil {
		return fmt.Errorf("config: %s: expected mapping at document root", path)
	}

	kb := mappingChild(mapping, knowledgeBaseYAMLKey)
	if kb == nil {
		kb = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		appendMappingEntry(mapping, scalarNode(knowledgeBaseYAMLKey), kb)
	} else if kb.Kind != yaml.MappingNode {
		return fmt.Errorf("config: %s: knowledge_base must be a mapping", path)
	}

	setMappingScalar(kb, knowledgeBaseURIKey, uri)

	return writeYAMLDocument(path, root)
}

func loadYAMLDocument(path string) (*yaml.Node, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file %s: %w", path, err)
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("parse config file %s: %w", path, err)
	}
	return &root, nil
}

func writeYAMLDocument(path string, root *yaml.Node) error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(root); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("close yaml encoder: %w", err)
	}
	return atomicWriteFile(path, buf.Bytes())
}

func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".config-*.yaml")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tmpName := tmp.Name()

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp config file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp config file: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp config file: %w", err)
	}
	return nil
}

func documentRootMapping(root *yaml.Node) *yaml.Node {
	if root == nil || len(root.Content) == 0 {
		return nil
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil
	}
	return doc
}

func mappingChild(mapping *yaml.Node, key string) *yaml.Node {
	if mapping == nil || mapping.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return mapping.Content[i+1]
		}
	}
	return nil
}

func setMappingScalar(mapping *yaml.Node, key, value string) {
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			v := mapping.Content[i+1]
			v.Kind = yaml.ScalarNode
			v.Tag = "!!str"
			v.Value = value
			return
		}
	}
	appendMappingEntry(mapping, scalarNode(key), scalarNode(value))
}

func appendMappingEntry(mapping, key, value *yaml.Node) {
	mapping.Content = append(mapping.Content, key, value)
}

func scalarNode(value string) *yaml.Node {
	return &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: value,
	}
}
