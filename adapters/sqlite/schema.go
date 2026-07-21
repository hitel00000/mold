package sqlite

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hitel00000/mold/resource"
)

// GenerateCreateTableSQL generates the DDL for creating a SQLite table from a Resource IR.
func GenerateCreateTableSQL(res *resource.Resource) string {
	var columns []string
	var constraints []string

	// Primary key 'id' is automatically included
	columns = append(columns, `"id" INTEGER PRIMARY KEY AUTOINCREMENT`)

	// Track explicit field names to avoid duplicates
	fieldMap := make(map[string]bool)
	fieldMap["id"] = true

	// Build columns from IR fields
	for _, f := range res.Fields {
		if fieldMap[f.Name] {
			continue
		}
		fieldMap[f.Name] = true

		colDef := fmt.Sprintf(`"%s" %s`, f.Name, mapToSQLiteType(f.Type))

		if !f.Nullable {
			colDef += " NOT NULL"
		}

		if f.Default != nil {
			colDef += fmt.Sprintf(" DEFAULT %s", formatDefaultValue(f.Default))
		}

		if f.Constraints.Unique {
			colDef += " UNIQUE"
		}

		// Field-level CHECK constraints for enum, min, max
		// Note: Constraints such as pattern, min_length, and max_length are skipped
		// at the SQLite DDL level due to dialect limitations/complexity, and are
		// enforced at the application level via resource.Validate.
		if f.Type == resource.TypeEnum && len(f.Constraints.Values) > 0 {
			var vals []string
			for _, v := range f.Constraints.Values {
				vals = append(vals, fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''")))
			}
			colDef += fmt.Sprintf(` CHECK ("%s" IN (%s))`, f.Name, strings.Join(vals, ", "))
		}

		if f.Constraints.Min != nil {
			colDef += fmt.Sprintf(` CHECK ("%s" >= %g)`, f.Name, *f.Constraints.Min)
		}
		if f.Constraints.Max != nil {
			colDef += fmt.Sprintf(` CHECK ("%s" <= %g)`, f.Name, *f.Constraints.Max)
		}

		columns = append(columns, colDef)
	}

	// Foreign key columns from belongs_to relations
	for _, rel := range res.Relations {
		if rel.Kind == resource.KindBelongsTo && rel.ForeignKey != "" {
			if !fieldMap[rel.ForeignKey] {
				fieldMap[rel.ForeignKey] = true
				columns = append(columns, fmt.Sprintf(`"%s" INTEGER`, rel.ForeignKey))
			}
			targetTable := toSnakeCase(rel.Target) + "s"
			constraints = append(constraints, fmt.Sprintf(`FOREIGN KEY ("%s") REFERENCES "%s"("id")`, rel.ForeignKey, targetTable))
		}
	}

	// Automatic timestamp columns
	if res.Timestamps {
		if !fieldMap["created_at"] {
			columns = append(columns, `"created_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`)
		}
		if !fieldMap["updated_at"] {
			columns = append(columns, `"updated_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`)
		}
	}

	// Automatic soft delete column
	if res.SoftDelete {
		if !fieldMap["deleted_at"] {
			columns = append(columns, `"deleted_at" TEXT NULL`)
		}
	}

	allDefs := append(columns, constraints...)

	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (%s);`, res.Table, strings.Join(allDefs, ", "))
}

func mapToSQLiteType(t resource.FieldType) string {
	switch t {
	case resource.TypeInt, resource.TypeBool:
		return "INTEGER"
	case resource.TypeFloat:
		return "REAL"
	default:
		// string, text, markdown, datetime, enum, email, url
		return "TEXT"
	}
}

func formatDefaultValue(val any) string {
	switch v := val.(type) {
	case string:
		return fmt.Sprintf("'%s'", strings.ReplaceAll(v, "'", "''"))
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return fmt.Sprintf("%v", v)
	}
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func toSnakeCase(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
