package view

import (
	"fmt"

	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
)

type WidgetKind string

const (
	WidgetInput    WidgetKind = "input"
	WidgetTextarea WidgetKind = "textarea"
	WidgetSelect   WidgetKind = "select"
	WidgetCheckbox WidgetKind = "checkbox"
)

type FieldWidget struct {
	Name        string
	Label       string
	Type        string // HTML input type (text, number, email, url, datetime-local, etc.)
	Kind        WidgetKind
	Value       any
	Required    bool
	Options     []string // For enum select
	MinLength   *int
	MaxLength   *int
	Min         *float64
	Max         *float64
	Pattern     string
	Description string
}

// BuildFormFields constructs the list of form input widgets for Create and Edit forms using plan.Build(res).
// Target 5 Migration: Eliminates Fields + Relations double iteration by iterating plan.Fields in a single unified loop.
func BuildFormFields(res *resource.Resource, currentValues map[string]any, isUpdate bool) []FieldWidget {
	p := plan.Build(res)
	if p == nil {
		return nil
	}

	if currentValues == nil {
		currentValues = make(map[string]any)
	}

	// Lookup map for belongs_to relation targets by foreign key name
	relTargetMap := make(map[string]string)
	for _, rel := range p.Relations {
		if rel.Kind == resource.KindBelongsTo && rel.ForeignKey != "" {
			relTargetMap[rel.ForeignKey] = rel.Target
		}
	}

	var widgets []FieldWidget

	// Single unified loop over plan.Fields (explicit + derived FK fields)
	for _, f := range p.Fields {
		if f.Deprecated {
			continue
		}

		val := currentValues[f.Name]

		// Check if field is a belongs_to foreign key
		if target, isFK := relTargetMap[f.Name]; isFK || f.IsDerivedFK {
			if target == "" {
				target = "Resource"
			}
			w := FieldWidget{
				Name:        f.Name,
				Label:       fmt.Sprintf("%s (%s ID)", f.Name, target),
				Type:        "number",
				Kind:        WidgetInput,
				Value:       val,
				Required:    !isUpdate,
				Description: fmt.Sprintf("Foreign key referencing %s", target),
			}
			widgets = append(widgets, w)
			continue
		}

		w := FieldWidget{
			Name:      f.Name,
			Label:     f.Name,
			Value:     val,
			Required:  !f.Nullable && !isUpdate && f.Default == nil,
			MinLength: f.Constraints.MinLength,
			MaxLength: f.Constraints.MaxLength,
			Min:       f.Constraints.Min,
			Max:       f.Constraints.Max,
			Pattern:   f.Constraints.Pattern,
		}

		// Widget selection owned by view package
		switch f.Type {
		case resource.TypeString:
			w.Kind = WidgetInput
			w.Type = "text"
		case resource.TypeText:
			w.Kind = WidgetTextarea
		case resource.TypeMarkdown:
			w.Kind = WidgetTextarea
			w.Type = "markdown"
			w.Description = "Markdown supported"
		case resource.TypeInt:
			w.Kind = WidgetInput
			w.Type = "number"
		case resource.TypeFloat:
			w.Kind = WidgetInput
			w.Type = "number"
		case resource.TypeBool:
			w.Kind = WidgetCheckbox
			w.Type = "checkbox"
		case resource.TypeDateTime:
			w.Kind = WidgetInput
			w.Type = "datetime-local"
		case resource.TypeEnum:
			w.Kind = WidgetSelect
			w.Options = f.Constraints.Values
		case resource.TypeEmail:
			w.Kind = WidgetInput
			w.Type = "email"
		case resource.TypeURL:
			w.Kind = WidgetInput
			w.Type = "url"
		default:
			w.Kind = WidgetInput
			w.Type = "text"
		}

		widgets = append(widgets, w)
	}

	return widgets
}
