package sqlite_test

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	_ "modernc.org/sqlite"
)

func TestCRUD_Operations(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_crud?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)

	path := filepath.Join("..", "..", "examples", "post.yaml")
	postRes, err := resource.LoadFromFile(path)
	if err != nil {
		t.Fatalf("failed to load post.yaml: %v", err)
	}

	if err := store.EnsureSchema(ctx, postRes); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	// 1. Create
	createdRecord, err := store.Create(ctx, postRes, storage.Record{
		"title": "First Post",
		"body":  "Hello World!",
	})
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	id, ok := createdRecord["id"]
	if !ok || id == nil {
		t.Fatalf("expected created record to have id, got: %v", createdRecord)
	}

	// 2. Get
	gotRecord, err := store.Get(ctx, postRes, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if gotRecord["title"] != "First Post" {
		t.Errorf("expected title 'First Post', got '%v'", gotRecord["title"])
	}

	// 3. Update
	updatedRecord, err := store.Update(ctx, postRes, id, storage.Record{
		"title": "Updated First Post",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updatedRecord["title"] != "Updated First Post" {
		t.Errorf("expected updated title 'Updated First Post', got '%v'", updatedRecord["title"])
	}

	// 4. List
	list, err := store.List(ctx, postRes, storage.Query{})
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected list length 1, got %d", len(list))
	}

	// 5. SoftDelete
	err = store.SoftDelete(ctx, postRes, id)
	if err != nil {
		t.Fatalf("SoftDelete failed: %v", err)
	}

	// 6. Get after SoftDelete should return ErrNotFound
	_, err = store.Get(ctx, postRes, id)
	if err != storage.ErrNotFound {
		t.Errorf("expected ErrNotFound after SoftDelete, got: %v", err)
	}

	// 7. List after SoftDelete should return empty list
	listAfterDelete, err := store.List(ctx, postRes, storage.Query{})
	if err != nil {
		t.Fatalf("List after SoftDelete failed: %v", err)
	}
	if len(listAfterDelete) != 0 {
		t.Errorf("expected empty list after SoftDelete, got %d items", len(listAfterDelete))
	}
}
