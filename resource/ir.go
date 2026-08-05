package resource

import (
	"gopkg.in/yaml.v3"
)

// FieldType represents the supported primitive types in Mold.
type FieldType string

const (
	TypeString   FieldType = "string"
	TypeText     FieldType = "text"
	TypeMarkdown FieldType = "markdown"
	TypeInt      FieldType = "int"
	TypeFloat    FieldType = "float"
	TypeBool     FieldType = "bool"
	TypeDateTime FieldType = "datetime"
	TypeEnum     FieldType = "enum"
	TypeEmail    FieldType = "email"
	TypeURL      FieldType = "url"
	TypePassword FieldType = "password"
	TypeBlob     FieldType = "blob"
)

// Constraints defines validation and configuration constraints for a field.
type Constraints struct {
	MinLength *int     `yaml:"min_length,omitempty" json:"min_length,omitempty"`
	MaxLength *int     `yaml:"max_length,omitempty" json:"max_length,omitempty"`
	Min       *float64 `yaml:"min,omitempty" json:"min,omitempty"`
	Max       *float64 `yaml:"max,omitempty" json:"max,omitempty"`
	Pattern   string   `yaml:"pattern,omitempty" json:"pattern,omitempty"`
	Unique    bool     `yaml:"unique,omitempty" json:"unique,omitempty"`
	Values    []string `yaml:"values,omitempty" json:"values,omitempty"`
}

// Field represents a single field definition in a Resource IR.
type Field struct {
	Name            string      `yaml:"name" json:"name"`
	Type            FieldType   `yaml:"type" json:"type"`
	Nullable        bool        `yaml:"nullable" json:"nullable"`
	Default         any         `yaml:"default,omitempty" json:"default,omitempty"`
	ClientWritable  bool        `yaml:"client_writable" json:"client_writable"` // Default true. If false, rejected in CREATE/UPDATE payload and omitted from default View form input.
	Constraints     Constraints `yaml:"constraints,omitempty" json:"constraints,omitempty"`
	Deprecated      bool        `yaml:"deprecated" json:"deprecated"`
	DeprecatedSince *int        `yaml:"deprecated_since,omitempty" json:"deprecated_since,omitempty"`
}

// UnmarshalYAML implements custom YAML unmarshaling for Field to ensure ClientWritable defaults to true when omitted.
func (f *Field) UnmarshalYAML(value *yaml.Node) error {
	type rawField Field
	raw := rawField{
		ClientWritable: true,
	}
	if err := value.Decode(&raw); err != nil {
		return err
	}
	*f = Field(raw)
	return nil
}

// RelationKind defines the cardinality/direction of a resource relation.
type RelationKind string

const (
	KindHasMany            RelationKind = "has_many"
	KindBelongsTo          RelationKind = "belongs_to"
	KindHasAndBelongsToMany RelationKind = "has_and_belongs_to_many"
)

// OnDeleteAction defines the behavior when a related parent resource is deleted.
type OnDeleteAction string

const (
	OnDeleteRestrict    OnDeleteAction = "restrict"
	OnDeleteSoftCascade OnDeleteAction = "soft_cascade"
)

// Relation represents a relationship between resources.
type Relation struct {
	Name       string         `yaml:"name" json:"name"`
	Kind       RelationKind   `yaml:"kind" json:"kind"`
	Target     string         `yaml:"target" json:"target"`
	ForeignKey string         `yaml:"foreign_key" json:"foreign_key"`
	OnDelete   OnDeleteAction `yaml:"on_delete,omitempty" json:"on_delete,omitempty"`
}

// Permissions defines row-level access permissions per operation.
type Permissions struct {
	Create string `yaml:"create,omitempty" json:"create,omitempty"`
	Read   string `yaml:"read,omitempty" json:"read,omitempty"`
	Update string `yaml:"update,omitempty" json:"update,omitempty"`
	Delete string `yaml:"delete,omitempty" json:"delete,omitempty"`
}

// Auth defines authentication and authorization integration for a resource.
type Auth struct {
	OwnershipField string      `yaml:"ownership_field,omitempty" json:"ownership_field,omitempty"`
	Permissions    Permissions `yaml:"permissions,omitempty" json:"permissions,omitempty"`
}

// ResourceConstraints defines resource-level multi-column constraints.
type ResourceConstraints struct {
	UniqueTogether [][]string `yaml:"unique_together,omitempty" json:"unique_together,omitempty"`
}

// Resource represents the complete Intermediate Representation (IR) of a resource.
type Resource struct {
	Name          string               `yaml:"name" json:"name"`
	Table         string               `yaml:"table" json:"table"`
	SchemaVersion int                  `yaml:"schema_version" json:"schema_version"`
	Timestamps    bool                 `yaml:"timestamps" json:"timestamps"`
	SoftDelete    bool                 `yaml:"soft_delete" json:"soft_delete"`
	Fields        []Field              `yaml:"fields" json:"fields"`
	Relations     []Relation           `yaml:"relations" json:"relations"`
	Auth          *Auth                `yaml:"auth,omitempty" json:"auth,omitempty"`
	Constraints   *ResourceConstraints `yaml:"constraints,omitempty" json:"constraints,omitempty"`
}

// NormalizeFields returns the complete list of fields for the resource, expanding implicit foreign key
// fields derived from belongs_to relations while avoiding duplicate field declarations.
func (r *Resource) NormalizeFields() []Field {
	if r == nil {
		return nil
	}

	res := make([]Field, 0, len(r.Fields)+len(r.Relations))
	fieldMap := make(map[string]bool, len(r.Fields))

	for _, f := range r.Fields {
		res = append(res, f)
		fieldMap[f.Name] = true
	}

	for _, rel := range r.Relations {
		if rel.Kind == KindBelongsTo && rel.ForeignKey != "" {
			if !fieldMap[rel.ForeignKey] {
				res = append(res, Field{
					Name:           rel.ForeignKey,
					Type:           TypeInt,
					Nullable:       true,
					ClientWritable: true,
				})
				fieldMap[rel.ForeignKey] = true
			}
		}
	}

	return res
}
