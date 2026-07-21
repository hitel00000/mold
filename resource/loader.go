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

// Load parses raw YAML byte data into a Resource IR using canonical YAML syntax.
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
			if valNode.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("resource node must be a mapping with name attribute")
			}
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
		case "fields":
			if valNode.Kind != yaml.SequenceNode {
				return nil, fmt.Errorf("fields node must be a sequence of field mappings")
			}
			var fields []Field
			for _, item := range valNode.Content {
				var f Field
				if err := item.Decode(&f); err != nil {
					return nil, fmt.Errorf("failed to decode field: %w", err)
				}
				fields = append(fields, f)
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

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
