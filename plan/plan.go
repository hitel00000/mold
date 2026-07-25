package plan

import (
	"github.com/hitel00000/mold/resource"
)

// FieldPlan represents a target-agnostic, normalized field specification.
type FieldPlan struct {
	Name            string
	Type            resource.FieldType
	Nullable        bool
	Default         any
	Constraints     resource.Constraints
	Deprecated      bool
	DeprecatedSince *int
	IsSystemColumn  bool
	IsDerivedFK     bool
}

// Plan represents the target-agnostic execution plan derived from a single Resource IR.
type Plan struct {
	ResourceName  string
	Table         string
	SchemaVersion int
	Timestamps    bool
	SoftDelete    bool

	// Fields contains explicit fields plus belongs_to derived FK fields.
	Fields []FieldPlan
}

// Build constructs a target-agnostic Plan from a single resource.Resource IR.
// It iterates only over the given resource's own fields and belongs_to relations,
// ensuring strict 1:1 resource scope with zero circular dependencies.
func Build(res *resource.Resource) *Plan {
	if res == nil {
		return nil
	}

	p := &Plan{
		ResourceName:  res.Name,
		Table:         res.Table,
		SchemaVersion: res.SchemaVersion,
		Timestamps:    res.Timestamps,
		SoftDelete:    res.SoftDelete,
		Fields:        make([]FieldPlan, 0, len(res.Fields)+len(res.Relations)),
	}

	existingNames := make(map[string]bool)

	// 1. Process explicit fields
	for _, f := range res.Fields {
		fp := FieldPlan{
			Name:            f.Name,
			Type:            f.Type,
			Nullable:        f.Nullable,
			Default:         f.Default,
			Constraints:     f.Constraints,
			Deprecated:      f.Deprecated,
			DeprecatedSince: f.DeprecatedSince,
			IsSystemColumn:  false,
			IsDerivedFK:     false,
		}
		p.Fields = append(p.Fields, fp)
		existingNames[f.Name] = true
	}

	// 2. Process belongs_to relations to derive FK fields if not already explicitly defined
	for _, rel := range res.Relations {
		if rel.Kind == resource.KindBelongsTo && rel.ForeignKey != "" {
			if !existingNames[rel.ForeignKey] {
				fp := FieldPlan{
					Name:        rel.ForeignKey,
					Type:        resource.TypeInt, // Default foreign key primitive type
					Nullable:    false,
					IsDerivedFK: true,
				}
				p.Fields = append(p.Fields, fp)
				existingNames[rel.ForeignKey] = true
			}
		}
	}

	return p
}
