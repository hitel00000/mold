package resource_test

import (
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestValidateRecord_UnknownDeprecatedAndPKFields(t *testing.T) {
	depSince := 2
	res := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false},
			{Name: "legacy_slug", Type: resource.TypeString, Deprecated: true, DeprecatedSince: &depSince},
		},
	}

	// 1. Reject explicit PK 'id' on Create
	err := resource.ValidateRecord(res, map[string]any{"title": "Test Title", "id": 1}, false)
	if err == nil {
		t.Errorf("expected error when providing explicit PK 'id' on Create, got nil")
	}

	// 2. Reject unknown field
	err = resource.ValidateRecord(res, map[string]any{"title": "Test Title", "titel": "typo"}, false)
	if err == nil {
		t.Errorf("expected error for unknown field 'titel', got nil")
	}

	// 3. Reject deprecated field write
	err = resource.ValidateRecord(res, map[string]any{"title": "Test Title", "legacy_slug": "old-slug"}, false)
	if err == nil {
		t.Errorf("expected error for writing deprecated field 'legacy_slug', got nil")
	}

	// 4. Valid record write
	err = resource.ValidateRecord(res, map[string]any{"title": "Test Title"}, false)
	if err != nil {
		t.Errorf("unexpected error for valid record: %v", err)
	}
}

func TestValidateRecord_FieldTypeMismatch(t *testing.T) {
	res := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: true},
			{Name: "view_count", Type: resource.TypeInt, Nullable: true},
			{Name: "rating", Type: resource.TypeFloat, Nullable: true},
			{Name: "is_published", Type: resource.TypeBool, Nullable: true},
			{Name: "published_at", Type: resource.TypeDateTime, Nullable: true},
		},
	}

	// 1. String expects string, got int
	err := resource.ValidateRecord(res, map[string]any{"title": 123}, false)
	if err == nil {
		t.Errorf("expected error for string field getting int, got nil")
	}

	// 2. Int expects int, got string
	err = resource.ValidateRecord(res, map[string]any{"view_count": "100"}, false)
	if err == nil {
		t.Errorf("expected error for int field getting string, got nil")
	}

	// 3. Int with decimal float should be rejected
	err = resource.ValidateRecord(res, map[string]any{"view_count": 10.5}, false)
	if err == nil {
		t.Errorf("expected error for int field getting float with decimal 10.5, got nil")
	}

	// 4. Int with integer float (e.g. 10.0) should be accepted
	err = resource.ValidateRecord(res, map[string]any{"view_count": 10.0}, false)
	if err != nil {
		t.Errorf("unexpected error for int field getting 10.0: %v", err)
	}

	// 5. Float accepts int or float
	err = resource.ValidateRecord(res, map[string]any{"rating": 5}, false)
	if err != nil {
		t.Errorf("unexpected error for float field getting int 5: %v", err)
	}

	// 6. Bool expects bool, got int
	err = resource.ValidateRecord(res, map[string]any{"is_published": 1}, false)
	if err == nil {
		t.Errorf("expected error for bool field getting int 1, got nil")
	}

	// 7. DateTime invalid format
	err = resource.ValidateRecord(res, map[string]any{"published_at": "invalid-date"}, false)
	if err == nil {
		t.Errorf("expected error for invalid datetime format, got nil")
	}
}

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
