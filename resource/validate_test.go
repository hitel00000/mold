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

func TestValidate_UniqueTogether(t *testing.T) {
	validRes := &resource.Resource{
		Name: "RecordTag",
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt},
			{Name: "tag_id", Type: resource.TypeInt},
		},
		Constraints: &resource.ResourceConstraints{
			UniqueTogether: [][]string{
				{"sake_record_id", "tag_id"},
			},
		},
	}
	if err := resource.Validate(validRes); err != nil {
		t.Errorf("expected valid unique_together to pass validation, got: %v", err)
	}

	nonExistentFieldRes := &resource.Resource{
		Name: "RecordTag",
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt},
		},
		Constraints: &resource.ResourceConstraints{
			UniqueTogether: [][]string{
				{"sake_record_id", "unknown_field"},
			},
		},
	}
	if err := resource.Validate(nonExistentFieldRes); err == nil {
		t.Errorf("expected error for non-existent field in unique_together, got nil")
	}

	singleFieldGroupRes := &resource.Resource{
		Name: "RecordTag",
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt},
		},
		Constraints: &resource.ResourceConstraints{
			UniqueTogether: [][]string{
				{"sake_record_id"},
			},
		},
	}
	if err := resource.Validate(singleFieldGroupRes); err == nil {
		t.Errorf("expected error for single field group in unique_together, got nil")
	}

	duplicateInGroupRes := &resource.Resource{
		Name: "RecordTag",
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt},
		},
		Constraints: &resource.ResourceConstraints{
			UniqueTogether: [][]string{
				{"sake_record_id", "sake_record_id"},
			},
		},
	}
	if err := resource.Validate(duplicateInGroupRes); err == nil {
		t.Errorf("expected error for duplicate field in unique_together group, got nil")
	}
}
