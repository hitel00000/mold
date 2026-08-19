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

func TestCRUD_ValidationRejection(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_crud_val?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)

	minLen := 3
	res := &resource.Resource{
		Name:  "Article",
		Table: "articles",
		Fields: []resource.Field{
			{
				Name:           "title",
				Type:           resource.TypeString,
				Nullable:       false,
				ClientWritable: true,
				Constraints: resource.Constraints{
					MinLength: &minLen,
				},
			},
		},
	}

	if err := store.EnsureSchema(ctx, res); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	// 1. Reject missing required field
	_, err = store.Create(ctx, res, storage.Record{})
	if err == nil {
		t.Errorf("expected error for missing required title field, got nil")
	}

	// 2. Reject min_length constraint violation on Create
	_, err = store.Create(ctx, res, storage.Record{"title": "ab"})
	if err == nil {
		t.Errorf("expected error for min_length violation on create, got nil")
	}

	// Valid Create
	record, err := store.Create(ctx, res, storage.Record{"title": "Valid Title"})
	if err != nil {
		t.Fatalf("valid Create failed: %v", err)
	}

	// 3. Reject min_length constraint violation on Update
	_, err = store.Update(ctx, res, record["id"], storage.Record{"title": "x"})
	if err == nil {
		t.Errorf("expected error for min_length violation on update, got nil")
	}
}

func TestList_IDsBatchQuery(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_crud_ids?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	res := &resource.Resource{
		Name:  "Item",
		Table: "items",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}
	if err := store.EnsureSchema(ctx, res); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	rec1, _ := store.Create(ctx, res, storage.Record{"name": "Item 1"})
	rec2, _ := store.Create(ctx, res, storage.Record{"name": "Item 2"})
	_, _ = store.Create(ctx, res, storage.Record{"name": "Item 3"})

	list, err := store.List(ctx, res, storage.Query{
		IDs: []any{rec1["id"], rec2["id"]},
	})
	if err != nil {
		t.Fatalf("List with IDs failed: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 records, got %d", len(list))
	}
}

func TestList_FilterSliceINQuery(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_crud_filter_in?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	childRes := &resource.Resource{
		Name:  "ChildItem",
		Table: "child_items",
		Fields: []resource.Field{
			{Name: "parent_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}
	if err := store.EnsureSchema(ctx, childRes); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	_, _ = store.Create(ctx, childRes, storage.Record{"parent_id": 10, "name": "Child 10-A"})
	_, _ = store.Create(ctx, childRes, storage.Record{"parent_id": 10, "name": "Child 10-B"})
	_, _ = store.Create(ctx, childRes, storage.Record{"parent_id": 20, "name": "Child 20-A"})
	_, _ = store.Create(ctx, childRes, storage.Record{"parent_id": 30, "name": "Child 30-A"})

	// 1. Query with []any slice in Filter
	list, err := store.List(ctx, childRes, storage.Query{
		Filter: map[string]any{
			"parent_id": []any{10, 20},
		},
	})
	if err != nil {
		t.Fatalf("List with slice filter failed: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 records for parent_id IN (10, 20), got %d", len(list))
	}

	// 2. Query with empty slice in Filter -> 0 results
	emptyList, err := store.List(ctx, childRes, storage.Query{
		Filter: map[string]any{
			"parent_id": []any{},
		},
	})
	if err != nil {
		t.Fatalf("List with empty slice filter failed: %v", err)
	}
	if len(emptyList) != 0 {
		t.Fatalf("expected 0 records for empty slice filter, got %d", len(emptyList))
	}
}

func TestCRUD_StringPrimaryKey_UUIDAndCustomID(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_crud_str_pk?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)
	tagRes := &resource.Resource{
		Name:       "Tag",
		Table:      "tags",
		Timestamps: true,
		SoftDelete: false,
		Fields: []resource.Field{
			{Name: "id", Type: resource.TypeString, Nullable: true, ClientWritable: true},
			{Name: "label", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	if err := store.EnsureSchema(ctx, tagRes); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	// 1. Create with omitted ID -> should auto-generate UUID string
	createdUUID, err := store.Create(ctx, tagRes, storage.Record{
		"label": "Auto UUID Tag",
	})
	if err != nil {
		t.Fatalf("Create with omitted id failed: %v", err)
	}

	uuidID, ok := createdUUID["id"].(string)
	if !ok || len(uuidID) != 36 {
		t.Fatalf("expected 36-char UUID string id, got: %v", createdUUID["id"])
	}

	// Verify Get by generated UUID
	gotUUID, err := store.Get(ctx, tagRes, uuidID)
	if err != nil {
		t.Fatalf("Get by UUID failed: %v", err)
	}
	if gotUUID["label"] != "Auto UUID Tag" {
		t.Errorf("expected label 'Auto UUID Tag', got '%v'", gotUUID["label"])
	}

	// 2. Create with custom ID -> should preserve provided string
	customID := "tag_taste_fresh"
	createdCustom, err := store.Create(ctx, tagRes, storage.Record{
		"id":    customID,
		"label": "Fresh Taste Tag",
	})
	if err != nil {
		t.Fatalf("Create with custom id failed: %v", err)
	}
	if createdCustom["id"] != customID {
		t.Errorf("expected custom id '%s', got '%v'", customID, createdCustom["id"])
	}

	// Verify Get by custom ID
	gotCustom, err := store.Get(ctx, tagRes, customID)
	if err != nil {
		t.Fatalf("Get by custom ID failed: %v", err)
	}
	if gotCustom["label"] != "Fresh Taste Tag" {
		t.Errorf("expected label 'Fresh Taste Tag', got '%v'", gotCustom["label"])
	}

	// 3. Update should succeed without altering PK
	updated, err := store.Update(ctx, tagRes, customID, storage.Record{
		"label": "Updated Fresh Taste Tag",
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated["label"] != "Updated Fresh Taste Tag" || updated["id"] != customID {
		t.Errorf("unexpected updated record: %v", updated)
	}
}
