package sqlite_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
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
