package transport

import (
	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

// SanitizeRecord removes fields marked with deprecated: true or password type using plan.Build(res).
// Target 7 Migration: Consumes target-agnostic plan.Plan and plan.FieldPlan.
func SanitizeRecord(res *resource.Resource, rec storage.Record) storage.Record {
	if rec == nil || res == nil {
		return rec
	}

	p := plan.Build(res)
	if p == nil {
		return rec
	}

	sanitized := make(storage.Record)
	omittedFields := make(map[string]bool)

	for _, f := range p.Fields {
		if f.Deprecated || f.Type == resource.TypePassword {
			omittedFields[f.Name] = true
		}
	}

	for k, v := range rec {
		if !omittedFields[k] {
			sanitized[k] = v
		}
	}

	return sanitized
}

// SanitizeRecordList applies SanitizeRecord to a list of records.
func SanitizeRecordList(res *resource.Resource, records []storage.Record) []storage.Record {
	if records == nil {
		return nil
	}
	sanitized := make([]storage.Record, len(records))
	for i, rec := range records {
		sanitized[i] = SanitizeRecord(res, rec)
	}
	return sanitized
}
