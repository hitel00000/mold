package transport

import (
	"context"
	"fmt"
	"strings"

	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

// ErrInvalidInclude represents an invalid relation specified in ?include=.
type ErrInvalidInclude struct {
	Relation string
}

func (e ErrInvalidInclude) Error() string {
	return fmt.Sprintf("invalid relation '%s' for include", e.Relation)
}

// ProcessIncludes parses includeParam, validates requested relations against resource IR,
// batch queries embedded targets (WHERE id IN (...)), evaluates individual ActionRead permissions,
// filters soft-deleted target records, and embeds sanitized objects (or null) directly into records.
func ProcessIncludes(ctx context.Context, reg *Registry, res *resource.Resource, records []storage.Record, includeParam string, sess *auth.Session) error {
	if includeParam == "" || res == nil {
		return nil
	}

	p := plan.Build(res)
	if p == nil {
		return nil
	}

	relMap := make(map[string]plan.RelationPlan, len(p.Relations))
	for _, rel := range p.Relations {
		relMap[rel.Name] = rel
	}

	rawItems := strings.Split(includeParam, ",")
	var requestedRels []plan.RelationPlan

	for _, raw := range rawItems {
		relName := strings.TrimSpace(raw)
		if relName == "" {
			continue
		}

		relPlan, exists := relMap[relName]
		if !exists || relPlan.Kind != resource.KindBelongsTo {
			return ErrInvalidInclude{Relation: relName}
		}

		requestedRels = append(requestedRels, relPlan)
	}

	if len(records) == 0 || len(requestedRels) == 0 {
		return nil
	}

	for _, rel := range requestedRels {
		fkField := rel.ForeignKey

		// 1. Collect unique non-null FK values across records
		var fkVals []any
		seenFK := make(map[any]bool)

		for _, rec := range records {
			if rec == nil {
				continue
			}
			val := rec[fkField]
			if val != nil && val != "" && !seenFK[val] {
				seenFK[val] = true
				fkVals = append(fkVals, val)
			}
		}

		// Initialize all records with null for this relation key
		for _, rec := range records {
			if rec != nil {
				rec[rel.Name] = nil
			}
		}

		if len(fkVals) == 0 {
			continue
		}

		// 2. Lookup target resource entry in Registry
		targetEntry, found := reg.LookupResource(rel.Target)
		if !found || targetEntry.Resource == nil || targetEntry.Store == nil {
			continue
		}

		// 3. Batch query target resource: WHERE id IN (...)
		targetRecords, err := targetEntry.Store.List(ctx, targetEntry.Resource, storage.Query{
			IDs: fkVals,
		})
		if err != nil {
			continue
		}

		// 4. Map target records by ID after evaluating ActionRead permissions & soft-delete
		targetMap := make(map[any]storage.Record)
		for _, tRec := range targetRecords {
			idVal := tRec["id"]
			if idVal == nil {
				continue
			}

			// Check soft delete
			if tRec["deleted_at"] != nil && tRec["deleted_at"] != "" {
				continue
			}

			// Evaluate ActionRead auth for individual embedded record
			_, allowed, _ := auth.Evaluate(sess, targetEntry.Resource, auth.ActionRead, tRec, nil)
			if !allowed {
				continue
			}

			sanitized := SanitizeRecord(targetEntry.Resource, tRec)
			targetMap[idVal] = sanitized
		}

		// 5. Attach embedded target object (or null) to each parent record
		for _, rec := range records {
			if rec == nil {
				continue
			}
			fkVal := rec[fkField]
			if fkVal != nil {
				if embedded, ok := targetMap[fkVal]; ok && embedded != nil {
					rec[rel.Name] = embedded
				} else {
					rec[rel.Name] = nil
				}
			} else {
				rec[rel.Name] = nil
			}
		}
	}

	return nil
}
