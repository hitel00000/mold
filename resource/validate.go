package resource

import (
	"fmt"
)

var validFieldTypes = map[FieldType]bool{
	TypeString:   true,
	TypeText:     true,
	TypeMarkdown: true,
	TypeInt:      true,
	TypeFloat:    true,
	TypeBool:     true,
	TypeDateTime: true,
	TypeEnum:     true,
	TypeEmail:    true,
	TypeURL:      true,
}

var validRelationKinds = map[RelationKind]bool{
	KindHasMany:            true,
	KindBelongsTo:          true,
	KindHasAndBelongsToMany: true,
}

var validOnDeleteActions = map[OnDeleteAction]bool{
	OnDeleteRestrict:    true,
	OnDeleteSoftCascade: true,
}

// Validate performs metaschema validation on a single Resource IR.
func Validate(r *Resource) error {
	if r == nil {
		return fmt.Errorf("resource is nil")
	}
	if r.Name == "" {
		return fmt.Errorf("resource name is required")
	}

	// Validate fields
	fieldNames := make(map[string]bool)
	for i, f := range r.Fields {
		if f.Name == "" {
			return fmt.Errorf("resource '%s': field at index %d has no name", r.Name, i)
		}
		if fieldNames[f.Name] {
			return fmt.Errorf("resource '%s': duplicate field name '%s'", r.Name, f.Name)
		}
		fieldNames[f.Name] = true

		if f.Type == "" {
			return fmt.Errorf("resource '%s': field '%s' type is required", r.Name, f.Name)
		}
		if !validFieldTypes[f.Type] {
			return fmt.Errorf("resource '%s': field '%s' has unsupported type '%s'", r.Name, f.Name, f.Type)
		}

		if err := validateConstraints(r.Name, f); err != nil {
			return err
		}
	}

	// Validate relations
	relNames := make(map[string]bool)
	for i, rel := range r.Relations {
		if rel.Name == "" {
			return fmt.Errorf("resource '%s': relation at index %d has no name", r.Name, i)
		}
		if relNames[rel.Name] {
			return fmt.Errorf("resource '%s': duplicate relation name '%s'", r.Name, rel.Name)
		}
		relNames[rel.Name] = true

		if rel.Kind == "" {
			return fmt.Errorf("resource '%s': relation '%s' kind is required", r.Name, rel.Name)
		}
		if !validRelationKinds[rel.Kind] {
			return fmt.Errorf("resource '%s': relation '%s' has invalid kind '%s'", r.Name, rel.Name, rel.Kind)
		}
		if rel.Target == "" {
			return fmt.Errorf("resource '%s': relation '%s' target is required", r.Name, rel.Name)
		}
		if rel.ForeignKey == "" {
			return fmt.Errorf("resource '%s': relation '%s' foreign_key is required", r.Name, rel.Name)
		}
		if rel.OnDelete != "" && !validOnDeleteActions[rel.OnDelete] {
			return fmt.Errorf("resource '%s': relation '%s' has invalid on_delete action '%s'", r.Name, rel.Name, rel.OnDelete)
		}
	}

	return nil
}

func validateConstraints(resName string, f Field) error {
	c := f.Constraints

	if c.MinLength != nil || c.MaxLength != nil {
		switch f.Type {
		case TypeString, TypeText, TypeMarkdown, TypeEmail, TypeURL:
			// allowed
		default:
			return fmt.Errorf("resource '%s': field '%s' constraint min_length/max_length is invalid for type '%s'", resName, f.Name, f.Type)
		}
	}

	if c.Min != nil || c.Max != nil {
		switch f.Type {
		case TypeInt, TypeFloat:
			// allowed
		default:
			return fmt.Errorf("resource '%s': field '%s' constraint min/max is invalid for type '%s'", resName, f.Name, f.Type)
		}
	}

	if f.Type == TypeEnum {
		if len(c.Values) == 0 {
			return fmt.Errorf("resource '%s': enum field '%s' requires constraint values", resName, f.Name)
		}
	} else if len(c.Values) > 0 {
		return fmt.Errorf("resource '%s': constraint values is invalid for non-enum field '%s'", resName, f.Name)
	}

	if c.Pattern != "" {
		switch f.Type {
		case TypeString, TypeText, TypeMarkdown, TypeEmail, TypeURL:
			// allowed
		default:
			return fmt.Errorf("resource '%s': field '%s' constraint pattern is invalid for type '%s'", resName, f.Name, f.Type)
		}
	}

	return nil
}

// ValidateTargetResources verifies that all relation targets exist in the given lookup function.
func ValidateTargetResources(r *Resource, exists func(target string) bool) error {
	for _, rel := range r.Relations {
		if !exists(rel.Target) {
			return fmt.Errorf("resource '%s': relation '%s' target resource '%s' does not exist in registry", r.Name, rel.Name, rel.Target)
		}
	}
	return nil
}
