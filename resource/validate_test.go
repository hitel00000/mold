package resource_test

import (
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestValidate_MissingResourceName(t *testing.T) {
	r := &resource.Resource{
		Name: "",
	}
	err := resource.Validate(r)
	if err == nil {
		t.Errorf("expected error for missing resource name, got nil")
	}
}

func TestValidate_MissingFieldType(t *testing.T) {
	r := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{Name: "title", Type: ""},
		},
	}
	err := resource.Validate(r)
	if err == nil {
		t.Errorf("expected error for missing field type, got nil")
	}
}

func TestValidate_UnsupportedFieldType(t *testing.T) {
	r := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{Name: "title", Type: "invalid_type"},
		},
	}
	err := resource.Validate(r)
	if err == nil {
		t.Errorf("expected error for unsupported field type, got nil")
	}
}

func TestValidate_InvalidConstraintMinMaxForString(t *testing.T) {
	minVal := 10.0
	r := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{
				Name: "title",
				Type: resource.TypeString,
				Constraints: resource.Constraints{
					Min: &minVal,
				},
			},
		},
	}
	err := resource.Validate(r)
	if err == nil {
		t.Errorf("expected error for min constraint on string field, got nil")
	}
}

func TestValidate_InvalidConstraintMinLengthForInt(t *testing.T) {
	minLen := 5
	r := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{
				Name: "view_count",
				Type: resource.TypeInt,
				Constraints: resource.Constraints{
					MinLength: &minLen,
				},
			},
		},
	}
	err := resource.Validate(r)
	if err == nil {
		t.Errorf("expected error for min_length constraint on int field, got nil")
	}
}

func TestValidate_EnumWithoutValues(t *testing.T) {
	r := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{
				Name: "status",
				Type: resource.TypeEnum,
			},
		},
	}
	err := resource.Validate(r)
	if err == nil {
		t.Errorf("expected error for enum field without values, got nil")
	}
}

func TestValidateTargetResources_MissingTarget(t *testing.T) {
	r := &resource.Resource{
		Name: "Comment",
		Relations: []resource.Relation{
			{
				Name:       "post",
				Kind:       resource.KindBelongsTo,
				Target:     "Post",
				ForeignKey: "post_id",
			},
		},
	}
	exists := func(target string) bool {
		return false // target Post does not exist
	}
	err := resource.ValidateTargetResources(r, exists)
	if err == nil {
		t.Errorf("expected error for missing relation target, got nil")
	}
}
