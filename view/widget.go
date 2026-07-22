package view

import (
	"fmt"

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

// BuildFormFields constructs the list of form input widgets for Create and Edit forms.
// It iterates over both res.Fields (excluding deprecated fields) and res.Relations (belongs_to foreign keys).
// System columns (id, created_at, updated_at, deleted_at) are excluded from form fields.
func BuildFormFields(res *resource.Resource, currentValues map[string]any, isUpdate bool) []FieldWidget {
	if res == nil {
		return nil
	}

	if currentValues == nil {
		currentValues = make(map[string]any)
	}

	var widgets []FieldWidget

	// 1. Process regular IR fields
	for _, f := range res.Fields {
		if f.Deprecated {
			continue
		}

		val := currentValues[f.Name]

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

		switch f.Type {
		case resource.TypeString:
			w.Kind = WidgetInput
			w.Type = "text"
		case resource.TypeText:
			w.Kind = WidgetTextarea
		case resource.TypeMarkdown:
			w.Kind = WidgetTextarea
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

	// 2. Process belongs_to Foreign Keys (e.g. post_id)
	for _, rel := range res.Relations {
		if rel.Kind == resource.KindBelongsTo && rel.ForeignKey != "" {
			val := currentValues[rel.ForeignKey]
			w := FieldWidget{
				Name:        rel.ForeignKey,
				Label:       fmt.Sprintf("%s (%s ID)", rel.ForeignKey, rel.Target),
				Type:        "number",
				Kind:        WidgetInput,
				Value:       val,
				Required:    !isUpdate,
				Description: fmt.Sprintf("Foreign key referencing %s", rel.Target),
			}
			widgets = append(widgets, w)
		}
	}

	return widgets
}
