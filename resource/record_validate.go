package resource

import (
	"fmt"
	"regexp"
	"unicode/utf8"
)

// ValidateRecord verifies that input record data satisfies all constraints defined in the Resource IR.
// If isUpdate is true, missing required fields are skipped since partial updates are allowed.
func ValidateRecord(r *Resource, record map[string]any, isUpdate bool) error {
	if r == nil {
		return fmt.Errorf("resource is nil")
	}
	if record == nil {
		record = make(map[string]any)
	}

	for _, f := range r.Fields {
		val, exists := record[f.Name]

		// 1. Required check for non-nullable fields without default values (only during Create)
		if !isUpdate && !f.Nullable && f.Default == nil {
			if !exists || val == nil {
				return fmt.Errorf("resource '%s': field '%s' is required", r.Name, f.Name)
			}
		}

		if !exists || val == nil {
			// If explicitly updating a non-nullable field with nil
			if isUpdate && exists && val == nil && !f.Nullable {
				return fmt.Errorf("resource '%s': field '%s' cannot be null", r.Name, f.Name)
			}
			continue
		}

		// 2. Validate string constraints (min_length, max_length, pattern)
		if strVal, isString := val.(string); isString {
			runeCount := utf8.RuneCountInString(strVal)

			if f.Constraints.MinLength != nil && runeCount < *f.Constraints.MinLength {
				return fmt.Errorf("resource '%s': field '%s' length %d is less than min_length %d", r.Name, f.Name, runeCount, *f.Constraints.MinLength)
			}
			if f.Constraints.MaxLength != nil && runeCount > *f.Constraints.MaxLength {
				return fmt.Errorf("resource '%s': field '%s' length %d is greater than max_length %d", r.Name, f.Name, runeCount, *f.Constraints.MaxLength)
			}
			if f.Constraints.Pattern != "" {
				matched, err := regexp.MatchString(f.Constraints.Pattern, strVal)
				if err != nil {
					return fmt.Errorf("resource '%s': field '%s' invalid regex pattern '%s': %w", r.Name, f.Name, f.Constraints.Pattern, err)
				}
				if !matched {
					return fmt.Errorf("resource '%s': field '%s' value '%s' does not match pattern '%s'", r.Name, f.Name, strVal, f.Constraints.Pattern)
				}
			}
		}

		// 3. Validate numeric min / max constraints
		if numVal, isNum := toFloat64(val); isNum {
			if f.Constraints.Min != nil && numVal < *f.Constraints.Min {
				return fmt.Errorf("resource '%s': field '%s' value %g is less than min %g", r.Name, f.Name, numVal, *f.Constraints.Min)
			}
			if f.Constraints.Max != nil && numVal > *f.Constraints.Max {
				return fmt.Errorf("resource '%s': field '%s' value %g is greater than max %g", r.Name, f.Name, numVal, *f.Constraints.Max)
			}
		}

		// 4. Validate Enum values
		if f.Type == TypeEnum && len(f.Constraints.Values) > 0 {
			strVal, ok := val.(string)
			if !ok {
				return fmt.Errorf("resource '%s': field '%s' enum value must be a string", r.Name, f.Name)
			}
			valid := false
			for _, allowed := range f.Constraints.Values {
				if strVal == allowed {
					valid = true
					break
				}
			}
			if !valid {
				return fmt.Errorf("resource '%s': field '%s' value '%s' is not in allowed enum values %v", r.Name, f.Name, strVal, f.Constraints.Values)
			}
		}
	}

	return nil
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
