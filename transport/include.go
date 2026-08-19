package transport

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

const MaxNestedRecordsPerParent = 50

// ErrInvalidInclude represents an invalid relation specified in ?include=.
type ErrInvalidInclude struct {
	Relation string
}

func (e ErrInvalidInclude) Error() string {
	return fmt.Sprintf("invalid relation '%s' for include", e.Relation)
}

// ErrIncludeTooLarge represents exceeding the maximum nested records allowed per parent.
type ErrIncludeTooLarge struct {
	Relation string
	Limit    int
}

func (e ErrIncludeTooLarge) Error() string {
	return fmt.Sprintf("nested records for relation '%s' exceed limit of %d", e.Relation, e.Limit)
}

// ProcessIncludes parses includeParam, validates requested relations against resource IR,
// batch queries embedded targets (belongs_to: WHERE id IN (...), has_many: WHERE foreign_key IN (...)),
// evaluates individual ActionRead permissions, filters soft-deleted target records,
// enforces the per-parent limit of 50 child records on has_many, and embeds sanitized objects (or arrays) into records.
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

		// Reject 2-depth dot-chaining (e.g. record_tags.tag)
		if strings.Contains(relName, ".") {
			return ErrInvalidInclude{Relation: relName}
		}

		relPlan, exists := relMap[relName]
		if !exists || (relPlan.Kind != resource.KindBelongsTo && relPlan.Kind != resource.KindHasMany) {
			return ErrInvalidInclude{Relation: relName}
		}

		requestedRels = append(requestedRels, relPlan)
	}

	if len(records) == 0 || len(requestedRels) == 0 {
		return nil
	}

	for _, rel := range requestedRels {
		switch rel.Kind {
		case resource.KindBelongsTo:
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

			// Initialize all records with null for this belongs_to relation key
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
				targetMap[normalizeIDKey(idVal)] = sanitized
			}

			// 5. Attach embedded target object (or null) to each parent record
			for _, rec := range records {
				if rec == nil {
					continue
				}
				fkVal := normalizeIDKey(rec[fkField])
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

		case resource.KindHasMany:
			fkField := rel.ForeignKey

			// 1. Collect unique non-null PK (id) values across parent records
			var parentIDs []any
			seenParentID := make(map[any]bool)

			for _, rec := range records {
				if rec == nil {
					continue
				}
				idVal := rec["id"]
				if idVal != nil && idVal != "" && !seenParentID[idVal] {
					seenParentID[idVal] = true
					parentIDs = append(parentIDs, idVal)
				}
			}

			// Initialize all parent records with empty slice [] for this has_many relation
			for _, rec := range records {
				if rec != nil {
					rec[rel.Name] = []storage.Record{}
				}
			}

			if len(parentIDs) == 0 || fkField == "" {
				continue
			}

			// 2. Lookup target child resource entry in Registry
			targetEntry, found := reg.LookupResource(rel.Target)
			if !found || targetEntry.Resource == nil || targetEntry.Store == nil {
				continue
			}

			// 3. Batch query child target resource: WHERE {fkField} IN (...)
			childRecords, err := targetEntry.Store.List(ctx, targetEntry.Resource, storage.Query{
				Filter: map[string]any{
					fkField: parentIDs,
				},
			})
			if err != nil {
				continue
			}

			// 4. Group child records by parent FK ID, enforcing 50 records limit per parent and auth
			childMap := make(map[any][]storage.Record)
			for _, cRec := range childRecords {
				if cRec == nil {
					continue
				}

				// Check soft delete
				if cRec["deleted_at"] != nil && cRec["deleted_at"] != "" {
					continue
				}

				parentFK := normalizeIDKey(cRec[fkField])
				if parentFK == nil {
					continue
				}

				// Evaluate ActionRead auth for individual child record
				_, allowed, _ := auth.Evaluate(sess, targetEntry.Resource, auth.ActionRead, cRec, nil)
				if !allowed {
					continue
				}

				sanitizedChild := SanitizeRecord(targetEntry.Resource, cRec)
				childMap[parentFK] = append(childMap[parentFK], sanitizedChild)
				if len(childMap[parentFK]) > MaxNestedRecordsPerParent {
					return ErrIncludeTooLarge{
						Relation: rel.Name,
						Limit:    MaxNestedRecordsPerParent,
					}
				}
			}

			// 5. Attach embedded children array to each parent record
			for _, rec := range records {
				if rec == nil {
					continue
				}
				pID := normalizeIDKey(rec["id"])
				if pID != nil && childMap[pID] != nil {
					rec[rel.Name] = childMap[pID]
				} else {
					rec[rel.Name] = []storage.Record{}
				}
			}
		}
	}

	return nil
}

func normalizeIDKey(v any) any {
	if v == nil {
		return nil
	}
	switch val := v.(type) {
	case int:
		return int64(val)
	case int32:
		return int64(val)
	case int64:
		return val
	case float64:
		return int64(val)
	case string:
		if n, err := strconv.ParseInt(val, 10, 64); err == nil {
			return n
		}
		return val
	default:
		return v
	}
}

