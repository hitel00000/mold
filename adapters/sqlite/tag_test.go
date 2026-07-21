package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	_ "modernc.org/sqlite"
)

func TestNonSoftDeleteResource_TagE2E(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_tag?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)

	path := filepath.Join("..", "..", "examples", "tag.yaml")
	tagRes, err := resource.LoadFromFile(path)
	if err != nil {
		t.Fatalf("failed to load tag.yaml: %v", err)
	}

	// 1. Ensure Schema
	if err := store.EnsureSchema(ctx, tagRes); err != nil {
		t.Fatalf("EnsureSchema for Tag failed: %v", err)
	}

	// Verify DDL does NOT contain deleted_at column
	ddl := sqlite.GenerateCreateTableSQL(tagRes)
	if strings.Contains(ddl, "deleted_at") {
		t.Errorf("expected DDL for non-soft-delete Tag to not contain deleted_at, got: %s", ddl)
	}

	// 2. Create
	tagRecord, err := store.Create(ctx, tagRes, storage.Record{
		"name": "golang",
	})
	if err != nil {
		t.Fatalf("Create Tag failed: %v", err)
	}

	tagID := tagRecord["id"]
	if tagID == nil {
		t.Fatalf("expected Tag ID to be non-nil")
	}

	// 3. Get
	gotTag, err := store.Get(ctx, tagRes, tagID)
	if err != nil {
		t.Fatalf("Get Tag failed: %v", err)
	}
	if gotTag["name"] != "golang" {
		t.Errorf("expected Tag name 'golang', got '%v'", gotTag["name"])
	}

	// 4. List
	tags, err := store.List(ctx, tagRes, storage.Query{})
	if err != nil {
		t.Fatalf("List Tags failed: %v", err)
	}
	if len(tags) != 1 {
		t.Fatalf("expected 1 tag in list, got %d", len(tags))
	}

	// 5. Update
	updatedTag, err := store.Update(ctx, tagRes, tagID, storage.Record{
		"name": "mold-framework",
	})
	if err != nil {
		t.Fatalf("Update Tag failed: %v", err)
	}
	if updatedTag["name"] != "mold-framework" {
		t.Errorf("expected updated Tag name 'mold-framework', got '%v'", updatedTag["name"])
	}

	// 6. Hard Delete (SoftDelete call on soft_delete: false resource)
	err = store.SoftDelete(ctx, tagRes, tagID)
	if err != nil {
		t.Fatalf("SoftDelete on non-soft-delete resource failed: %v", err)
	}

	// 7. Verify hard deletion
	_, err = store.Get(ctx, tagRes, tagID)
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound after deletion, got: %v", err)
	}

	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM tags").Scan(&count)
	if err != nil {
		t.Fatalf("failed to query row count: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 rows in tags table after hard delete, got %d", count)
	}
}
