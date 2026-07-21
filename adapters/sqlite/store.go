package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	_ "modernc.org/sqlite"
)

const metaTableName = "_mold_schema_versions"

// Store implements storage.Store for SQLite databases.
type Store struct {
	db *sql.DB
	mu sync.Mutex
}

// Ensure interface compliance
var _ storage.Store = (*Store)(nil)

// NewStore initializes a Store with an existing sql.DB handle.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// Open connects to a SQLite database by DSN and returns a Store.
func Open(dsn string) (*Store, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite database: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping sqlite database: %w", err)
	}
	return NewStore(db), nil
}

// Close closes the database connection.
func (s *Store) Close() error {
	return s.db.Close()
}

// DB returns the underlying sql.DB instance for testing or direct access.
func (s *Store) DB() *sql.DB {
	return s.db
}

// EnsureSchema creates or migrates a table based on the Resource IR.
func (s *Store) EnsureSchema(ctx context.Context, res *resource.Resource) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Ensure meta table exists
	initMetaSQL := fmt.Sprintf(`CREATE TABLE IF NOT EXISTS "%s" (
		"resource_name" TEXT PRIMARY KEY,
		"version" INTEGER NOT NULL,
		"updated_at" TEXT NOT NULL DEFAULT (DATETIME('now'))
	);`, metaTableName)

	if _, err := s.db.ExecContext(ctx, initMetaSQL); err != nil {
		return fmt.Errorf("failed to initialize schema meta table: %w", err)
	}

	// 2. Query current applied version
	var currentVersion int
	querySQL := fmt.Sprintf(`SELECT "version" FROM "%s" WHERE "resource_name" = ?;`, metaTableName)
	err := s.db.QueryRowContext(ctx, querySQL, res.Name).Scan(&currentVersion)
	if err != nil && err != sql.ErrNoRows {
		return fmt.Errorf("failed to query schema version for %s: %w", res.Name, err)
	}

	// If version is already applied, do nothing
	if err == nil && currentVersion == res.SchemaVersion {
		return nil
	}

	// 3. Destructive migration: DROP existing table if version is different
	// Note: Destructive migration is an intended MVP constraint.
	// If real user data preservation is required, this will be replaced with a diff-based migration strategy.
	dropSQL := fmt.Sprintf(`DROP TABLE IF EXISTS "%s";`, res.Table)
	if _, err := s.db.ExecContext(ctx, dropSQL); err != nil {
		return fmt.Errorf("failed to drop table %s for destructive migration: %w", res.Table, err)
	}

	// 4. CREATE TABLE
	createSQL := GenerateCreateTableSQL(res)
	if _, err := s.db.ExecContext(ctx, createSQL); err != nil {
		return fmt.Errorf("failed to create table %s: %w", res.Table, err)
	}

	// 5. Update meta table
	upsertMetaSQL := fmt.Sprintf(`INSERT INTO "%s" ("resource_name", "version", "updated_at") VALUES (?, ?, DATETIME('now'))
		ON CONFLICT("resource_name") DO UPDATE SET "version" = excluded."version", "updated_at" = excluded."updated_at";`, metaTableName)
	if _, err := s.db.ExecContext(ctx, upsertMetaSQL, res.Name, res.SchemaVersion); err != nil {
		return fmt.Errorf("failed to update schema version for %s: %w", res.Name, err)
	}

	return nil
}
