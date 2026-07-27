package sqlite

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
)

// GenerateCreateTableSQL generates the DDL for creating a SQLite table from a Resource IR using plan.Build(res).
// Target 4 Migration: Consumes target-agnostic plan.Plan and plan.FieldPlan.
func GenerateCreateTableSQL(res *resource.Resource) string {
	p := plan.Build(res)
	if p == nil {
		return ""
	}

	var columns []string
	var constraints []string

	// Primary key 'id' is automatically included
	columns = append(columns, `"id" INTEGER PRIMARY KEY AUTOINCREMENT`)

	// Track explicit field names to avoid duplicates
	fieldMap := make(map[string]bool)
	fieldMap["id"] = true

	// Build columns from plan fields (explicit + derived FKs)
	for _, f := range p.Fields {
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

		// Column-level UNIQUE constraint only if soft_delete is false.
		// If soft_delete is true, a partial unique index (WHERE deleted_at IS NULL) is generated instead.
		if f.Constraints.Unique && !p.SoftDelete {
			colDef += " UNIQUE"
		}

		// Field-level CHECK constraints for enum, min, max
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

	// Foreign key constraints from belongs_to relations in plan
	for _, rel := range p.Relations {
		if rel.Kind == resource.KindBelongsTo && rel.ForeignKey != "" {
			targetTable := toSnakeCase(rel.Target) + "s"
			constraints = append(constraints, fmt.Sprintf(`FOREIGN KEY ("%s") REFERENCES "%s"("id")`, rel.ForeignKey, targetTable))
		}
	}

	// Automatic timestamp columns
	if p.Timestamps {
		if !fieldMap["created_at"] {
			columns = append(columns, `"created_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`)
		}
		if !fieldMap["updated_at"] {
			columns = append(columns, `"updated_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`)
		}
	}

	// Automatic soft delete column
	if p.SoftDelete {
		if !fieldMap["deleted_at"] {
			columns = append(columns, `"deleted_at" TEXT NULL`)
		}
	}

	allDefs := append(columns, constraints...)

	return fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (%s);`, p.Table, strings.Join(allDefs, ", "))
}

// GenerateIndexesSQL generates index DDLs for a Resource using plan.Build(res).
func GenerateIndexesSQL(res *resource.Resource) []string {
	p := plan.Build(res)
	var indexes []string

	if p == nil {
		return indexes
	}

	if p.SoftDelete {
		for _, f := range p.Fields {
			if f.Constraints.Unique {
				idxName := fmt.Sprintf("idx_%s_%s_unique", p.Table, f.Name)
				idxSQL := fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS "%s" ON "%s"("%s") WHERE "deleted_at" IS NULL;`,
					idxName, p.Table, f.Name)
				indexes = append(indexes, idxSQL)
			}
		}
	}

	for _, group := range p.UniqueTogether {
		if len(group) == 0 {
			continue
		}
		quotedCols := make([]string, 0, len(group))
		for _, col := range group {
			quotedCols = append(quotedCols, fmt.Sprintf(`"%s"`, col))
		}
		idxName := fmt.Sprintf("idx_%s_unique_%s", p.Table, strings.Join(group, "_"))
		var idxSQL string
		if p.SoftDelete {
			idxSQL = fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS "%s" ON "%s"(%s) WHERE "deleted_at" IS NULL;`,
				idxName, p.Table, strings.Join(quotedCols, ", "))
		} else {
			idxSQL = fmt.Sprintf(`CREATE UNIQUE INDEX IF NOT EXISTS "%s" ON "%s"(%s);`,
				idxName, p.Table, strings.Join(quotedCols, ", "))
		}
		indexes = append(indexes, idxSQL)
	}

	return indexes
}

func mapToSQLiteType(t resource.FieldType) string {
	switch t {
	case resource.TypeInt, resource.TypeBool:
		return "INTEGER"
	case resource.TypeFloat:
		return "REAL"
	default:
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
