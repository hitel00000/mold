package resource

import (
	"fmt"
	"regexp"
	"time"
	"unicode/utf8"
)

// ValidateRecord verifies that input record data satisfies type safety and all constraints defined in the Resource IR.
// Primary key 'id' is strictly rejected in both Create and Update payloads to prevent PK mutation.
func ValidateRecord(r *Resource, record map[string]any, isUpdate bool) error {
	if r == nil {
		return fmt.Errorf("resource is nil")
	}
	if record == nil {
		record = make(map[string]any)
	}

	// 1. Build sets of valid, system, and deprecated fields based on IR flags
	validFields := make(map[string]bool)
	systemFields := make(map[string]bool)
	deprecatedFields := make(map[string]bool)

	for _, f := range r.Fields {
		if f.Deprecated {
			deprecatedFields[f.Name] = true
		} else {
			validFields[f.Name] = true
		}
	}
	for _, rel := range r.Relations {
		if rel.Kind == KindBelongsTo && rel.ForeignKey != "" {
			validFields[rel.ForeignKey] = true
		}
	}

	// System columns dynamically determined by IR settings
	if r.Timestamps {
		systemFields["created_at"] = true
		systemFields["updated_at"] = true
	}
	if r.SoftDelete {
		systemFields["deleted_at"] = true
	}

	// 2. Validate input keys: PK 'id', system columns, deprecated fields, and unknown fields
	for k := range record {
		if k == "id" {
			if !isUpdate {
				return fmt.Errorf("resource '%s': primary key 'id' cannot be explicitly provided in create payload", r.Name)
			}
			return fmt.Errorf("resource '%s': primary key 'id' cannot be included in update payload; pass it as the target id parameter instead", r.Name)
		}
		if systemFields[k] {
			return fmt.Errorf("resource '%s': system column '%s' cannot be explicitly provided in write payload", r.Name, k)
		}
		if deprecatedFields[k] {
			return fmt.Errorf("resource '%s': field '%s' is deprecated and cannot be written", r.Name, k)
		}
		if !validFields[k] {
			return fmt.Errorf("resource '%s': unknown field '%s'", r.Name, k)
		}
	}

	// 3. Field level checks for fields in Resource IR
	for _, f := range r.Fields {
		if f.Deprecated {
			continue
		}

		val, exists := record[f.Name]

		// Required check for non-nullable fields without default values (only during Create)
		if !isUpdate && !f.Nullable && f.Default == nil {
			if !exists || val == nil {
				return fmt.Errorf("resource '%s': field '%s' is required", r.Name, f.Name)
			}
		}

		if !exists || val == nil {
			if isUpdate && exists && val == nil && !f.Nullable {
				return fmt.Errorf("resource '%s': field '%s' cannot be null", r.Name, f.Name)
			}
			continue
		}

		// Field Type Validation
		if err := validateFieldType(r.Name, f, val); err != nil {
			return err
		}

		// Constraints Validation
		if err := validateFieldConstraints(r.Name, f, val); err != nil {
			return err
		}
	}

	return nil
}

func validateFieldType(resName string, f Field, val any) error {
	switch f.Type {
	case TypeString, TypeText, TypeMarkdown, TypeEmail, TypeURL, TypeBlob:
		if _, ok := val.(string); !ok {
			return fmt.Errorf("resource '%s': field '%s' expects %s, got %s", resName, f.Name, f.Type, typeNameOf(val))
		}
	case TypePassword:
		strVal, ok := val.(string)
		if !ok {
			return fmt.Errorf("resource '%s': field '%s' expects password string, got %s", resName, f.Name, typeNameOf(val))
		}
		if len([]byte(strVal)) > 72 {
			return fmt.Errorf("resource '%s': password field '%s' value exceeds maximum allowed bcrypt length of 72 bytes", resName, f.Name)
		}
	case TypeInt:
		switch v := val.(type) {
		case int, int64, int32:
			// valid integer
		case float64:
			if v != float64(int64(v)) {
				return fmt.Errorf("resource '%s': field '%s' expects int, got float with decimal (%g)", resName, f.Name, v)
			}
		case float32:
			if float64(v) != float64(int64(v)) {
				return fmt.Errorf("resource '%s': field '%s' expects int, got float with decimal (%g)", resName, f.Name, v)
			}
		default:
			return fmt.Errorf("resource '%s': field '%s' expects int, got %s", resName, f.Name, typeNameOf(val))
		}
	case TypeFloat:
		switch val.(type) {
		case float64, float32, int, int64, int32:
			// valid numeric
		default:
			return fmt.Errorf("resource '%s': field '%s' expects float, got %s", resName, f.Name, typeNameOf(val))
		}
	case TypeBool:
		if _, ok := val.(bool); !ok {
			return fmt.Errorf("resource '%s': field '%s' expects bool, got %s", resName, f.Name, typeNameOf(val))
		}
	case TypeDateTime:
		strVal, ok := val.(string)
		if !ok {
			return fmt.Errorf("resource '%s': field '%s' expects datetime (ISO8601 string), got %s", resName, f.Name, typeNameOf(val))
		}
		if !isValidDateTime(strVal) {
			return fmt.Errorf("resource '%s': field '%s' value '%s' is not a valid ISO8601 datetime", resName, f.Name, strVal)
		}
	case TypeEnum:
		strVal, ok := val.(string)
		if !ok {
			return fmt.Errorf("resource '%s': field '%s' expects string enum, got %s", resName, f.Name, typeNameOf(val))
		}
		if len(f.Constraints.Values) > 0 {
			valid := false
			for _, allowed := range f.Constraints.Values {
				if strVal == allowed {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("resource '%s': field '%s' value '%s' is not in allowed enum values %v", resName, f.Name, strVal, f.Constraints.Values)
			}
		}
	}

	return nil
}

func validateFieldConstraints(resName string, f Field, val any) error {
	if strVal, isString := val.(string); isString {
		runeCount := utf8.RuneCountInString(strVal)

		if f.Constraints.MinLength != nil && runeCount < *f.Constraints.MinLength {
			return fmt.Errorf("resource '%s': field '%s' length %d is less than min_length %d", resName, f.Name, runeCount, *f.Constraints.MinLength)
		}
		if f.Constraints.MaxLength != nil && runeCount > *f.Constraints.MaxLength {
			return fmt.Errorf("resource '%s': field '%s' length %d is greater than max_length %d", resName, f.Name, runeCount, *f.Constraints.MaxLength)
		}
		if f.Constraints.Pattern != "" {
			matched, err := regexp.MatchString(f.Constraints.Pattern, strVal)
			if err != nil {
				return fmt.Errorf("resource '%s': field '%s' invalid regex pattern '%s': %w", resName, f.Name, f.Constraints.Pattern, err)
			}
			if !matched {
				return fmt.Errorf("resource '%s': field '%s' value '%s' does not match pattern '%s'", resName, f.Name, strVal, f.Constraints.Pattern)
			}
		}
	}

	if numVal, isNum := toFloat64(val); isNum {
		if f.Constraints.Min != nil && numVal < *f.Constraints.Min {
			return fmt.Errorf("resource '%s': field '%s' value %g is less than min %g", resName, f.Name, numVal, *f.Constraints.Min)
		}
		if f.Constraints.Max != nil && numVal > *f.Constraints.Max {
			return fmt.Errorf("resource '%s': field '%s' value %g is greater than max %g", resName, f.Name, numVal, *f.Constraints.Max)
		}
	}

	return nil
}

func typeNameOf(val any) string {
	if val == nil {
		return "nil"
	}
	return fmt.Sprintf("%T", val)
}

func isValidDateTime(s string) bool {
	formats := []string{
		time.RFC3339,
		time.RFC3339Nano,
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		"2006-01-02",
	}
	for _, fmtStr := range formats {
		if _, err := time.Parse(fmtStr, s); err == nil {
			return true
		}
	}
	return false
}

func toFloat64(val any) (float64, bool) {
	switch v := val.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	default:
		return 0, false
	}
}
