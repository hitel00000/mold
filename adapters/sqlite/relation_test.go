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

func TestRelation_PostAndCommentsIntegration(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_rel?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open sqlite in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)

	examplesDir := filepath.Join("..", "..", "examples")
	reg, err := resource.LoadAll(examplesDir)
	if err != nil {
		t.Fatalf("failed to LoadAll from examples: %v", err)
	}

	postRes, _ := reg.Get("Post")
	commentRes, _ := reg.Get("Comment")

	// Ensure schemas for both resources
	if err := store.EnsureSchema(ctx, postRes); err != nil {
		t.Fatalf("EnsureSchema for Post failed: %v", err)
	}
	if err := store.EnsureSchema(ctx, commentRes); err != nil {
		t.Fatalf("EnsureSchema for Comment failed: %v", err)
	}

	// 1. Create a Post
	postRecord, err := store.Create(ctx, postRes, storage.Record{
		"title": "Relation Test Post",
		"body":  "Testing HasMany and BelongsTo relations",
	})
	if err != nil {
		t.Fatalf("Create Post failed: %v", err)
	}

	postID := postRecord["id"]
	if postID == nil {
		t.Fatalf("expected Post ID to be present")
	}

	// 2. Create multiple Comments linked via post_id
	comment1, err := store.Create(ctx, commentRes, storage.Record{
		"body":    "First comment on post",
		"post_id": postID,
	})
	if err != nil {
		t.Fatalf("Create Comment 1 failed: %v", err)
	}

	comment2, err := store.Create(ctx, commentRes, storage.Record{
		"body":    "Second comment on post",
		"post_id": postID,
	})
	if err != nil {
		t.Fatalf("Create Comment 2 failed: %v", err)
	}

	// 3. Query Comments linked to the Post using FK filter
	comments, err := store.List(ctx, commentRes, storage.Query{
		Filter: map[string]any{
			"post_id": postID,
		},
	})
	if err != nil {
		t.Fatalf("List Comments by post_id failed: %v", err)
	}
	if len(comments) != 2 {
		t.Fatalf("expected 2 comments for post_id %v, got %d", postID, len(comments))
	}

	// 4. SoftDelete the parent Post
	err = store.SoftDelete(ctx, postRes, postID)
	if err != nil {
		t.Fatalf("SoftDelete Post failed: %v", err)
	}

	// Verify parent Post is soft deleted
	_, err = store.Get(ctx, postRes, postID)
	if err != storage.ErrNotFound {
		t.Errorf("expected parent Post to be soft deleted (ErrNotFound), got: %v", err)
	}

	// 5. Verify child Comments still exist independently
	// (Note: Automatic soft_cascade handling is deferred to Milestone 3+;
	// currently parent soft delete does not impact existing child records)
	c1Record, err := store.Get(ctx, commentRes, comment1["id"])
	if err != nil {
		t.Errorf("expected child comment 1 to remain intact, got error: %v", err)
	} else if c1Record["body"] != "First comment on post" {
		t.Errorf("unexpected comment body: %v", c1Record["body"])
	}

	c2Record, err := store.Get(ctx, commentRes, comment2["id"])
	if err != nil {
		t.Errorf("expected child comment 2 to remain intact, got error: %v", err)
	} else if c2Record["body"] != "Second comment on post" {
		t.Errorf("unexpected comment body: %v", c2Record["body"])
	}
}
