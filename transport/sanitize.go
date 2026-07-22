package transport

import (
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

// SanitizeRecord removes fields marked with deprecated: true in the Resource IR from output records.
func SanitizeRecord(res *resource.Resource, rec storage.Record) storage.Record {
	if rec == nil || res == nil {
		return rec
	}

	sanitized := make(storage.Record)
	deprecatedFields := make(map[string]bool)

	for _, f := range res.Fields {
		if f.Deprecated || f.Type == resource.TypePassword {
			deprecatedFields[f.Name] = true
		}
	}

	for k, v := range rec {
		if !deprecatedFields[k] {
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
