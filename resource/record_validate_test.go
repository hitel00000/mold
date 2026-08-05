package resource_test

import (
	"errors"
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestValidateRecord_UnknownDeprecatedAndPKFields(t *testing.T) {
	depSince := 2
	res := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "legacy_slug", Type: resource.TypeString, Deprecated: true, DeprecatedSince: &depSince, ClientWritable: true},
		},
	}

	// 1. Reject explicit PK 'id' on Create
	err := resource.ValidateRecord(res, map[string]any{"title": "Test Title", "id": 1}, false)
	if err == nil {
		t.Errorf("expected error when providing explicit PK 'id' on Create, got nil")
	}

	// 1-b. Reject explicit PK 'id' on Update
	err = resource.ValidateRecord(res, map[string]any{"title": "Test Title", "id": 1}, true)
	if err == nil {
		t.Errorf("expected error when providing explicit PK 'id' on Update, got nil")
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

// TestValidateRecord_GoldenExecutionOrderSnapshot verifies the exact pre-migration execution order:
// System column rejection -> Type Validation FIRST -> Constraint Validation SECOND.
func TestValidateRecord_GoldenExecutionOrderSnapshot(t *testing.T) {
	minLen := 5
	res := &resource.Resource{
		Name:       "User",
		Timestamps: true,
		SoftDelete: true,
		Fields: []resource.Field{
			{Name: "username", Type: resource.TypeString, Nullable: false, ClientWritable: true, Constraints: resource.Constraints{MinLength: &minLen}},
		},
		Relations: []resource.Relation{
			{Name: "org", Kind: resource.KindBelongsTo, Target: "Org", ForeignKey: "org_id"},
		},
	}

	// 1. System Column Rejection (created_at)
	err := resource.ValidateRecord(res, map[string]any{"username": "alice", "created_at": "2026-01-01"}, false)
	if err == nil || err.Error() != "resource 'User': system column 'created_at' cannot be explicitly provided in write payload" {
		t.Errorf("unexpected error for system column created_at: %v", err)
	}

	// 2. Type Validation FIRST: sending integer to string field (with min_length 5 constraint)
	// Must return type error FIRST, NOT min_length error SECOND
	err = resource.ValidateRecord(res, map[string]any{"username": 12345}, false)
	if err == nil || err.Error() != "resource 'User': field 'username' expects string, got int" {
		t.Errorf("expected type validation error FIRST ('expects string, got int'), got: %v", err)
	}

	// 3. Constraint Validation SECOND: sending short string to field with min_length 5
	err = resource.ValidateRecord(res, map[string]any{"username": "bob"}, false)
	if err == nil || err.Error() != "resource 'User': field 'username' length 3 is less than min_length 5" {
		t.Errorf("expected constraint validation error SECOND ('length 3 is less than min_length 5'), got: %v", err)
	}

	// 4. Pre-migration behavior: derived FK fields in relations were in validFields, but Section 3 only iterated r.Fields,
	// so derived FKs were not type-validated in pre-migration code. Plan migration will unify this.
	err = resource.ValidateRecord(res, map[string]any{"username": "alice", "org_id": 42}, false)
	if err != nil {
		t.Errorf("unexpected error for valid org_id: %v", err)
	}
}

func TestValidateRecord_SystemColumnRejection(t *testing.T) {
	// Resource with Timestamps & SoftDelete true
	resWithSys := &resource.Resource{
		Name:       "Post",
		Timestamps: true,
		SoftDelete: true,
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	// 1. Reject created_at as system column
	err := resource.ValidateRecord(resWithSys, map[string]any{"title": "T", "created_at": "2026-01-01T00:00:00Z"}, false)
	if err == nil || err.Error() != "resource 'Post': system column 'created_at' cannot be explicitly provided in write payload" {
		t.Errorf("expected system column error for created_at, got: %v", err)
	}

	// 2. Reject deleted_at as system column
	err = resource.ValidateRecord(resWithSys, map[string]any{"title": "T", "deleted_at": "2026-01-01T00:00:00Z"}, false)
	if err == nil || err.Error() != "resource 'Post': system column 'deleted_at' cannot be explicitly provided in write payload" {
		t.Errorf("expected system column error for deleted_at, got: %v", err)
	}

	// Resource without Timestamps & SoftDelete (false)
	resWithoutSys := &resource.Resource{
		Name:       "PostNoSys",
		Timestamps: false,
		SoftDelete: false,
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	// 3. Reject deleted_at as unknown field when SoftDelete is false
	err = resource.ValidateRecord(resWithoutSys, map[string]any{"title": "T", "deleted_at": "2026-01-01T00:00:00Z"}, false)
	if err == nil || err.Error() != "resource 'PostNoSys': unknown field 'deleted_at'" {
		t.Errorf("expected unknown field error for deleted_at when SoftDelete is false, got: %v", err)
	}
}

func TestValidateRecord_FieldTypeMismatch(t *testing.T) {
	res := &resource.Resource{
		Name: "Post",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: true, ClientWritable: true},
			{Name: "view_count", Type: resource.TypeInt, Nullable: true, ClientWritable: true},
			{Name: "rating", Type: resource.TypeFloat, Nullable: true, ClientWritable: true},
			{Name: "is_published", Type: resource.TypeBool, Nullable: true, ClientWritable: true},
			{Name: "published_at", Type: resource.TypeDateTime, Nullable: true, ClientWritable: true},
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
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
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
				Name:           "title",
				Type:           resource.TypeString,
				Nullable:       false,
				ClientWritable: true,
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
				Name:           "email",
				Type:           resource.TypeEmail,
				Nullable:       false,
				ClientWritable: true,
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
				Name:           "price",
				Type:           resource.TypeFloat,
				Nullable:       false,
				ClientWritable: true,
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

func TestValidateRecord_ClientWritable(t *testing.T) {
	res := &resource.Resource{
		Name: "User",
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false, ClientWritable: true},
			{Name: "role", Type: resource.TypeEnum, Nullable: false, Default: "user", ClientWritable: false, Constraints: resource.Constraints{Values: []string{"admin", "user"}}},
		},
	}

	// 1. Reject payload containing client_writable: false field (with string value)
	err := resource.ValidateRecord(res, map[string]any{"email": "user@example.com", "role": "admin"}, false)
	if err == nil || !errors.Is(err, resource.ErrClientWriteForbidden) {
		t.Errorf("expected ErrClientWriteForbidden for role: admin, got: %v", err)
	}
	t.Logf("[RAW PROOF LOG 1 - Go ValidateRecord String Payload Rejection]: %v", err)

	// 2. Reject payload containing client_writable: false field (with explicit null value: {"role": null})
	errNull := resource.ValidateRecord(res, map[string]any{"email": "user@example.com", "role": nil}, false)
	if errNull == nil || !errors.Is(errNull, resource.ErrClientWriteForbidden) {
		t.Errorf("expected ErrClientWriteForbidden for role: nil, got: %v", errNull)
	}
	t.Logf("[RAW PROOF LOG 2 - Go ValidateRecord Explicit Null Key Rejection (role: null)]: %v", errNull)

	// 3. Reject on Update as well if field key is present
	errUpd := resource.ValidateRecord(res, map[string]any{"role": "user"}, true)
	if errUpd == nil || !errors.Is(errUpd, resource.ErrClientWriteForbidden) {
		t.Errorf("expected ErrClientWriteForbidden on Update, got: %v", errUpd)
	}
	t.Logf("[RAW PROOF LOG 3 - Go ValidateRecord Update Rejection]: %v", errUpd)

	// 4. Accept payload omitting client_writable: false field
	errOk := resource.ValidateRecord(res, map[string]any{"email": "user@example.com"}, false)
	if errOk != nil {
		t.Errorf("unexpected error when omitting client_writable: false field: %v", errOk)
	}
	t.Logf("[RAW PROOF LOG 4 - Go ValidateRecord Accept Payload Omitting Non-Writable Field]: err = nil")
}
