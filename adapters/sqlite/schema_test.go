package sqlite_test

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
)

// TestSQLiteSchema_GoldenParity verifies that GenerateCreateTableSQL derived via plan.Plan
// matches expected SQLite DDL for Post and Comment resources byte-for-byte.
func TestSQLiteSchema_GoldenParity(t *testing.T) {
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	postRes, _ := reg.Get("Post")
	commentRes, _ := reg.Get("Comment")

	expectedPostDDL := `CREATE TABLE IF NOT EXISTS "posts" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "title" TEXT NOT NULL, "body" TEXT NOT NULL, "author_id" INTEGER NOT NULL, "created_at" TEXT NOT NULL DEFAULT (DATETIME('now')), "updated_at" TEXT NOT NULL DEFAULT (DATETIME('now')), "deleted_at" TEXT NULL, FOREIGN KEY ("author_id") REFERENCES "users"("id"));`
	expectedCommentDDL := `CREATE TABLE IF NOT EXISTS "comments" ("id" INTEGER PRIMARY KEY AUTOINCREMENT, "body" TEXT NOT NULL, "post_id" INTEGER NOT NULL, "author_id" INTEGER NOT NULL, "created_at" TEXT NOT NULL DEFAULT (DATETIME('now')), "updated_at" TEXT NOT NULL DEFAULT (DATETIME('now')), "deleted_at" TEXT NULL, FOREIGN KEY ("post_id") REFERENCES "posts"("id"), FOREIGN KEY ("author_id") REFERENCES "users"("id"));`

	postDDL := sqlite.GenerateCreateTableSQL(postRes)
	if postDDL != expectedPostDDL {
		t.Errorf("Post DDL mismatch!\nExpected:\n%s\nGot:\n%s", expectedPostDDL, postDDL)
	}

	commentDDL := sqlite.GenerateCreateTableSQL(commentRes)
	if commentDDL != expectedCommentDDL {
		t.Errorf("Comment DDL mismatch!\nExpected:\n%s\nGot:\n%s", expectedCommentDDL, commentDDL)
	}
}

// TestSQLiteSchema_RealDBExecution verifies that GenerateCreateTableSQL executes cleanly on a real SQLite DB.
func TestSQLiteSchema_RealDBExecution(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_real.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite db: %v", err)
	}
	defer db.Close()

	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	for _, res := range reg.List() {
		ddl := sqlite.GenerateCreateTableSQL(res)
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("failed executing DDL on real DB for %s: %v\nDDL:\n%s", res.Name, err, ddl)
		}
	}
}
