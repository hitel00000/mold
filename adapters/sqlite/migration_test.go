package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
	_ "modernc.org/sqlite"
)

func TestEnsureSchema_DestructiveMigration(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_migration?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)

	path := filepath.Join("..", "..", "examples", "post.yaml")
	postRes, err := resource.LoadFromFile(path)
	if err != nil {
		t.Fatalf("failed to load post.yaml: %v", err)
	}

	// 1. Initial schema application (version 1)
	err = store.EnsureSchema(ctx, postRes)
	if err != nil {
		t.Fatalf("EnsureSchema version 1 failed: %v", err)
	}

	var version int
	err = db.QueryRow("SELECT version FROM _mold_schema_versions WHERE resource_name = ?", postRes.Name).Scan(&version)
	if err != nil {
		t.Fatalf("failed to query version: %v", err)
	}
	if version != 1 {
		t.Errorf("expected schema version 1, got %d", version)
	}

	// Insert dummy record to verify table is recreated upon version change
	_, err = db.Exec(`INSERT INTO posts (title, body) VALUES ('Test Title', 'Test Body')`)
	if err != nil {
		t.Fatalf("failed to insert dummy row: %v", err)
	}

	// 2. Change version to 2 and run EnsureSchema (destructive migration expected)
	postRes.SchemaVersion = 2
	err = store.EnsureSchema(ctx, postRes)
	if err != nil {
		t.Fatalf("EnsureSchema version 2 failed: %v", err)
	}

	err = db.QueryRow("SELECT version FROM _mold_schema_versions WHERE resource_name = ?", postRes.Name).Scan(&version)
	if err != nil {
		t.Fatalf("failed to query updated version: %v", err)
	}
	if version != 2 {
		t.Errorf("expected schema version 2, got %d", version)
	}

	// Recreated table should be empty
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM posts").Scan(&count)
	if err != nil {
		t.Fatalf("failed to count rows in posts: %v", err)
	}
	if count != 0 {
		t.Errorf("expected table to be dropped and empty (count 0), got %d", count)
	}
}
