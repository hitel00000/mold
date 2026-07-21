package resource

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFromFile reads a YAML file and parses it into a Resource IR.
func LoadFromFile(path string) (*Resource, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", path, err)
	}
	return Load(data)
}

// Load parses raw YAML byte data into a Resource IR.
func Load(data []byte) (*Resource, error) {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil {
		return nil, fmt.Errorf("failed to unmarshal yaml: %w", err)
	}

	if len(root.Content) == 0 {
		return nil, fmt.Errorf("empty yaml document")
	}

	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("expected top-level mapping in yaml")
	}

	r := &Resource{
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
	}

	for i := 0; i < len(doc.Content); i += 2 {
		keyNode := doc.Content[i]
		valNode := doc.Content[i+1]

		switch keyNode.Value {
		case "resource":
			if valNode.Kind == yaml.ScalarNode {
				r.Name = valNode.Value
			} else if valNode.Kind == yaml.MappingNode {
				for j := 0; j < len(valNode.Content); j += 2 {
					rkNode := valNode.Content[j]
					rvNode := valNode.Content[j+1]
					switch rkNode.Value {
					case "name":
						r.Name = rvNode.Value
					case "table":
						r.Table = rvNode.Value
					case "schema_version":
						var v int
						if err := rvNode.Decode(&v); err == nil {
							r.SchemaVersion = v
						}
					case "timestamps":
						var v bool
						if err := rvNode.Decode(&v); err == nil {
							r.Timestamps = v
						}
					case "soft_delete":
						var v bool
						if err := rvNode.Decode(&v); err == nil {
							r.SoftDelete = v
						}
					}
				}
			}
		case "fields":
			fields, err := parseFields(valNode)
			if err != nil {
				return nil, fmt.Errorf("failed to parse fields: %w", err)
			}
			r.Fields = fields
		case "relations":
			var rels []Relation
			if err := valNode.Decode(&rels); err != nil {
				return nil, fmt.Errorf("failed to parse relations: %w", err)
			}
			r.Relations = rels
		case "auth":
			var auth Auth
			if err := valNode.Decode(&auth); err != nil {
				return nil, fmt.Errorf("failed to parse auth: %w", err)
			}
			r.Auth = &auth
		}
	}

	if r.SchemaVersion == 0 {
		r.SchemaVersion = 1
	}

	// Infer default table name if not provided
	if r.Table == "" && r.Name != "" {
		r.Table = toSnakeCase(r.Name) + "s"
	}

	return r, nil
}

func parseFields(node *yaml.Node) ([]Field, error) {
	var fields []Field

	switch node.Kind {
	case yaml.SequenceNode:
		for _, item := range node.Content {
			var f Field
			if err := item.Decode(&f); err != nil {
				return nil, err
			}
			fields = append(fields, f)
		}
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			fieldName := keyNode.Value
			var f Field
			f.Name = fieldName

			if valNode.Kind == yaml.ScalarNode {
				f.Type = FieldType(valNode.Value)
			} else if valNode.Kind == yaml.MappingNode {
				if err := valNode.Decode(&f); err != nil {
					return nil, err
				}
				f.Name = fieldName
			} else {
				return nil, fmt.Errorf("invalid field specification for %s", fieldName)
			}
			fields = append(fields, f)
		}
	default:
		return nil, fmt.Errorf("unsupported fields YAML node kind")
	}

	return fields, nil
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
