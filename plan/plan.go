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

// RelationPlan represents a target-agnostic relation specification.
type RelationPlan struct {
	Name       string
	Kind       resource.RelationKind
	Target     string
	ForeignKey string
}

// Plan represents the target-agnostic execution plan derived from a single Resource IR.
type Plan struct {
	ResourceName  string
	Table         string
	SchemaVersion int
	Timestamps    bool
	SoftDelete    bool

	// Auth contains authentication and row-level permission specifications.
	Auth *resource.Auth

	// Fields contains explicit fields plus belongs_to derived FK fields.
	Fields []FieldPlan

	// Relations contains normalized relation plans.
	Relations []RelationPlan
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
		Auth:          res.Auth,
		Fields:        make([]FieldPlan, 0, len(res.Fields)+len(res.Relations)),
		Relations:     make([]RelationPlan, 0, len(res.Relations)),
	}

	for _, rel := range res.Relations {
		p.Relations = append(p.Relations, RelationPlan{
			Name:       rel.Name,
			Kind:       rel.Kind,
			Target:     rel.Target,
			ForeignKey: rel.ForeignKey,
		})
	}

	explicitNames := make(map[string]bool, len(res.Fields))
	for _, f := range res.Fields {
		explicitNames[f.Name] = true
	}

	// Single unified iteration over res.NormalizeFields() (explicit + derived FK fields)
	for _, f := range res.NormalizeFields() {
		fp := FieldPlan{
			Name:            f.Name,
			Type:            f.Type,
			Nullable:        f.Nullable,
			Default:         f.Default,
			Constraints:     f.Constraints,
			Deprecated:      f.Deprecated,
			DeprecatedSince: f.DeprecatedSince,
			IsSystemColumn:  false,
			IsDerivedFK:     !explicitNames[f.Name],
		}
		p.Fields = append(p.Fields, fp)
	}

	return p
}
