package resource_test

import (
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestValidateRecord_RequiredFieldMissing(t *testing.T) {
	res := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false},
		},
	}

	record := map[string]any{}
	err := resource.ValidateRecord(res, record, false)
	if err == nil {
		t.Errorf("expected error for missing required field 'title', got nil")
	}

	// Should pass during update if not included
	err = resource.ValidateRecord(res, record, true)
	if err != nil {
		t.Errorf("unexpected error for missing required field during update: %v", err)
	}
}

func TestValidateRecord_MinMaxLength(t *testing.T) {
	minLen := 3
	maxLen := 10
	res := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{
				Name:     "title",
				Type:     resource.TypeString,
				Nullable: false,
				Constraints: resource.Constraints{
					MinLength: &minLen,
					MaxLength: &maxLen,
				},
			},
		},
	}

	// Too short
	err := resource.ValidateRecord(res, map[string]any{"title": "ab"}, false)
	if err == nil {
		t.Errorf("expected error for title shorter than min_length, got nil")
	}

	// Too long
	err = resource.ValidateRecord(res, map[string]any{"title": "this title is way too long"}, false)
	if err == nil {
		t.Errorf("expected error for title longer than max_length, got nil")
	}

	// Valid length
	err = resource.ValidateRecord(res, map[string]any{"title": "valid"}, false)
	if err != nil {
		t.Errorf("unexpected error for valid title: %v", err)
	}
}

func TestValidateRecord_Pattern(t *testing.T) {
	res := &resource.Resource{
		Name: "User",
		Fields: []resource.Field{
			{
				Name:     "email",
				Type:     resource.TypeEmail,
				Nullable: false,
				Constraints: resource.Constraints{
					Pattern: `^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`,
				},
			},
		},
	}

	// Invalid pattern
	err := resource.ValidateRecord(res, map[string]any{"email": "not-an-email"}, false)
	if err == nil {
		t.Errorf("expected error for pattern mismatch, got nil")
	}

	// Valid pattern
	err = resource.ValidateRecord(res, map[string]any{"email": "user@example.com"}, false)
	if err != nil {
		t.Errorf("unexpected error for valid email pattern: %v", err)
	}
}

func TestValidateRecord_MinMax(t *testing.T) {
	minVal := 1.0
	maxVal := 100.0
	res := &resource.Resource{
		Name: "Product",
		Fields: []resource.Field{
			{
				Name:     "price",
				Type:     resource.TypeFloat,
				Nullable: false,
				Constraints: resource.Constraints{
					Min: &minVal,
					Max: &maxVal,
				},
			},
		},
	}

	// Out of range (lower)
	err := resource.ValidateRecord(res, map[string]any{"price": 0.5}, false)
	if err == nil {
		t.Errorf("expected error for price lower than min, got nil")
	}

	// Out of range (higher)
	err = resource.ValidateRecord(res, map[string]any{"price": 150.0}, false)
	if err == nil {
		t.Errorf("expected error for price higher than max, got nil")
	}

	// Valid
	err = resource.ValidateRecord(res, map[string]any{"price": 49.99}, false)
	if err != nil {
		t.Errorf("unexpected error for valid price: %v", err)
	}
}
