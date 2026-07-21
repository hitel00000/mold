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

func TestGenerateCreateTableSQL_Post(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "post.yaml")
	postRes, err := resource.LoadFromFile(path)
	if err != nil {
		t.Fatalf("failed to load post.yaml: %v", err)
	}

	ddl := sqlite.GenerateCreateTableSQL(postRes)

	expectedParts := []string{
		`CREATE TABLE IF NOT EXISTS "posts"`,
		`"id" INTEGER PRIMARY KEY AUTOINCREMENT`,
		`"title" TEXT NOT NULL`,
		`"body" TEXT NOT NULL`,
		`"created_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`,
		`"updated_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`,
		`"deleted_at" TEXT NULL`,
	}

	for _, part := range expectedParts {
		if !strings.Contains(ddl, part) {
			t.Errorf("expected DDL to contain '%s', got: %s", part, ddl)
		}
	}
}

func TestGenerateCreateTableSQL_Comment(t *testing.T) {
	path := filepath.Join("..", "..", "examples", "comment.yaml")
	commentRes, err := resource.LoadFromFile(path)
	if err != nil {
		t.Fatalf("failed to load comment.yaml: %v", err)
	}

	ddl := sqlite.GenerateCreateTableSQL(commentRes)

	expectedParts := []string{
		`CREATE TABLE IF NOT EXISTS "comments"`,
		`"id" INTEGER PRIMARY KEY AUTOINCREMENT`,
		`"body" TEXT NOT NULL`,
		`"post_id" INTEGER`,
		`FOREIGN KEY ("post_id") REFERENCES "posts"("id")`,
		`"created_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`,
		`"updated_at" TEXT NOT NULL DEFAULT (DATETIME('now'))`,
		`"deleted_at" TEXT NULL`,
	}

	for _, part := range expectedParts {
		if !strings.Contains(ddl, part) {
			t.Errorf("expected DDL to contain '%s', got: %s", part, ddl)
		}
	}
}

func TestPartialUniqueIndex_WithSoftDelete(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", "file:mem_partial_unique?mode=memory&cache=shared")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}
	defer db.Close()

	store := sqlite.NewStore(db)

	res := &resource.Resource{
		Name:       "User",
		Table:      "users",
		SoftDelete: true,
		Timestamps: true,
		Fields: []resource.Field{
			{
				Name:     "username",
				Type:     resource.TypeString,
				Nullable: false,
				Constraints: resource.Constraints{
					Unique: true,
				},
			},
		},
	}

	if err := store.EnsureSchema(ctx, res); err != nil {
		t.Fatalf("EnsureSchema failed: %v", err)
	}

	// 1. Create first user with unique username 'alice'
	user1, err := store.Create(ctx, res, storage.Record{"username": "alice"})
	if err != nil {
		t.Fatalf("Create user1 failed: %v", err)
	}

	// 2. Creating duplicate active user should fail
	_, err = store.Create(ctx, res, storage.Record{"username": "alice"})
	if err == nil {
		t.Errorf("expected error for duplicate active username, got nil")
	}

	// 3. Soft-delete user1
	if err := store.SoftDelete(ctx, res, user1["id"]); err != nil {
		t.Fatalf("SoftDelete user1 failed: %v", err)
	}

	// 4. Create new user with same username 'alice' after soft-delete should succeed
	_, err = store.Create(ctx, res, storage.Record{"username": "alice"})
	if err != nil {
		t.Errorf("expected Create with same username after soft-delete to succeed, got error: %v", err)
	}
}
