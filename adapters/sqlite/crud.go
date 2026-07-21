package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
)

// Create inserts a new record into the resource table.
func (s *Store) Create(ctx context.Context, res *resource.Resource, record storage.Record) (storage.Record, error) {
	if record == nil {
		record = make(storage.Record)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	if res.Timestamps {
		if _, exists := record["created_at"]; !exists {
			record["created_at"] = nowStr
		}
		if _, exists := record["updated_at"]; !exists {
			record["updated_at"] = nowStr
		}
	}

	var cols []string
	var placeholders []string
	var args []any

	for k, v := range record {
		cols = append(cols, fmt.Sprintf(`"%s"`, k))
		placeholders = append(placeholders, "?")
		args = append(args, v)
	}

	insertSQL := fmt.Sprintf(`INSERT INTO "%s" (%s) VALUES (%s);`,
		res.Table,
		strings.Join(cols, ", "),
		strings.Join(placeholders, ", "),
	)

	result, err := s.db.ExecContext(ctx, insertSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to insert record into %s: %w", res.Table, err)
	}

	id, err := result.LastInsertId()
	if err == nil && id > 0 {
		if _, exists := record["id"]; !exists {
			record["id"] = id
		}
	}

	return record, nil
}

// Get fetches a single record by ID.
func (s *Store) Get(ctx context.Context, res *resource.Resource, id any) (storage.Record, error) {
	querySQL := fmt.Sprintf(`SELECT * FROM "%s" WHERE "id" = ?`, res.Table)
	args := []any{id}

	if res.SoftDelete {
		querySQL += ` AND "deleted_at" IS NULL`
	}
	querySQL += `;`

	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query record from %s: %w", res.Table, err)
	}
	defer rows.Close()

	records, err := scanRows(rows)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, storage.ErrNotFound
	}

	return records[0], nil
}

// List queries records matching filters, pagination, and soft delete constraints.
func (s *Store) List(ctx context.Context, res *resource.Resource, query storage.Query) ([]storage.Record, error) {
	querySQL := fmt.Sprintf(`SELECT * FROM "%s" WHERE 1=1`, res.Table)
	var args []any

	if res.SoftDelete {
		querySQL += ` AND "deleted_at" IS NULL`
	}

	for k, v := range query.Filter {
		querySQL += fmt.Sprintf(` AND "%s" = ?`, k)
		args = append(args, v)
	}

	if query.Limit > 0 {
		querySQL += fmt.Sprintf(" LIMIT %d", query.Limit)
	}
	if query.Offset > 0 {
		querySQL += fmt.Sprintf(" OFFSET %d", query.Offset)
	}
	querySQL += `;`

	rows, err := s.db.QueryContext(ctx, querySQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list records from %s: %w", res.Table, err)
	}
	defer rows.Close()

	return scanRows(rows)
}

// Update modifies an existing record by ID.
func (s *Store) Update(ctx context.Context, res *resource.Resource, id any, record storage.Record) (storage.Record, error) {
	if len(record) == 0 {
		return s.Get(ctx, res, id)
	}

	nowStr := time.Now().UTC().Format(time.RFC3339)
	if res.Timestamps {
		record["updated_at"] = nowStr
	}

	var setClauses []string
	var args []any

	for k, v := range record {
		if k == "id" {
			continue
		}
		setClauses = append(setClauses, fmt.Sprintf(`"%s" = ?`, k))
		args = append(args, v)
	}

	if len(setClauses) == 0 {
		return s.Get(ctx, res, id)
	}

	updateSQL := fmt.Sprintf(`UPDATE "%s" SET %s WHERE "id" = ?`, res.Table, strings.Join(setClauses, ", "))
	args = append(args, id)

	if res.SoftDelete {
		updateSQL += ` AND "deleted_at" IS NULL`
	}
	updateSQL += `;`

	result, err := s.db.ExecContext(ctx, updateSQL, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to update record in %s: %w", res.Table, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return nil, err
	}
	if rowsAffected == 0 {
		return nil, storage.ErrNotFound
	}

	return s.Get(ctx, res, id)
}

// SoftDelete marks a record as soft-deleted or permanently removes it.
func (s *Store) SoftDelete(ctx context.Context, res *resource.Resource, id any) error {
	var deleteSQL string
	args := []any{id}

	if res.SoftDelete {
		nowStr := time.Now().UTC().Format(time.RFC3339)
		deleteSQL = fmt.Sprintf(`UPDATE "%s" SET "deleted_at" = ? WHERE "id" = ? AND "deleted_at" IS NULL;`, res.Table)
		args = []any{nowStr, id}
	} else {
		deleteSQL = fmt.Sprintf(`DELETE FROM "%s" WHERE "id" = ?;`, res.Table)
	}

	result, err := s.db.ExecContext(ctx, deleteSQL, args...)
	if err != nil {
		return fmt.Errorf("failed to delete record from %s: %w", res.Table, err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return storage.ErrNotFound
	}

	return nil
}

func scanRows(rows *sql.Rows) ([]storage.Record, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	var results []storage.Record

	for rows.Next() {
		columns := make([]any, len(cols))
		columnPointers := make([]any, len(cols))
		for i := range columns {
			columnPointers[i] = &columns[i]
		}

		if err := rows.Scan(columnPointers...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		rec := make(storage.Record)
		for i, colName := range cols {
			val := columnPointers[i].(*any)
			if *val != nil {
				if b, ok := (*val).([]byte); ok {
					rec[colName] = string(b)
				} else {
					rec[colName] = *val
				}
			} else {
				rec[colName] = nil
			}
		}
		results = append(results, rec)
	}

	return results, rows.Err()
}
