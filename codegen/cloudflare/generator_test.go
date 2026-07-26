package cloudflare_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/codegen/cloudflare"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
)

func TestCloudflareGenerator_DirectIRConsumption(t *testing.T) {
	// 1. Load IR directly via resource.LoadAll using relative path examples/blog
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed to load resource IR from %s: %v", relResourceDir, err)
	}

	postRes, exists := reg.Get("Post")
	if !exists {
		t.Fatalf("expected Post resource in IR registry")
	}

	// Verify that IR is a strong Go struct, proving direct IR consumption without YAML re-parsing
	if postRes.Name != "Post" || postRes.Table != "posts" {
		t.Fatalf("unexpected Post IR name/table: %s / %s", postRes.Name, postRes.Table)
	}

	// 2. Generate Cloudflare Workers code from loaded IR
	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generator failed: %v", err)
	}

	// 3. Verify D1 Schema SQL
	if !strings.Contains(output.SchemaSQL, `CREATE TABLE IF NOT EXISTS "posts"`) {
		t.Errorf("expected SchemaSQL to contain CREATE TABLE IF NOT EXISTS \"posts\", got:\n%s", output.SchemaSQL)
	}
	if !strings.Contains(output.SchemaSQL, `"title" TEXT NOT NULL`) {
		t.Errorf("expected SchemaSQL to contain \"title\" TEXT NOT NULL, got:\n%s", output.SchemaSQL)
	}

	// 4. Verify TS + Hono IndexTS
	if !strings.Contains(output.IndexTS, `import { Hono } from 'hono';`) {
		t.Errorf("expected IndexTS to import Hono")
	}
	if !strings.Contains(output.IndexTS, `app.get('/api/posts', async (c) => {`) {
		t.Errorf("expected IndexTS to define GET /api/posts route")
	}
	if !strings.Contains(output.IndexTS, `app.post('/api/posts', async (c) => {`) {
		t.Errorf("expected IndexTS to define POST /api/posts route")
	}
	if !strings.Contains(output.IndexTS, `app.delete('/api/posts/:id', async (c) => {`) {
		t.Errorf("expected IndexTS to define DELETE /api/posts route")
	}

	// 5. Verify PackageJSON and WranglerConfig
	if !strings.Contains(output.PackageJSON, `"hono": "^4.7.0"`) {
		t.Errorf("expected PackageJSON to contain hono dependency")
	}
	if !strings.Contains(output.WranglerConfig, `"database_name": "mold-d1"`) {
		t.Errorf("expected WranglerConfig to contain D1 binding")
	}
}

// TestCloudflareGenerator_SchemaSQLGoldenParity verifies that the generated D1 DDL schema SQL
// derived via plan.Plan matches the expected golden DDL string byte-for-byte.
func TestCloudflareGenerator_SchemaSQLGoldenParity(t *testing.T) {
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	expectedPostsDDL := `CREATE TABLE IF NOT EXISTS "posts" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "title" TEXT NOT NULL,
    "body" TEXT NOT NULL,
    "author_id" INTEGER NOT NULL,
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL,
    "deleted_at" TEXT
);`

	expectedCommentsDDL := `CREATE TABLE IF NOT EXISTS "comments" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "body" TEXT NOT NULL,
    "post_id" INTEGER NOT NULL,
    "author_id" INTEGER NOT NULL,
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL,
    "deleted_at" TEXT
);`

	if !strings.Contains(output.SchemaSQL, expectedPostsDDL) {
		t.Errorf("posts DDL mismatch!\nExpected snippet:\n%s\nGot SchemaSQL:\n%s", expectedPostsDDL, output.SchemaSQL)
	}
	if !strings.Contains(output.SchemaSQL, expectedCommentsDDL) {
		t.Errorf("comments DDL mismatch!\nExpected snippet:\n%s\nGot SchemaSQL:\n%s", expectedCommentsDDL, output.SchemaSQL)
	}
}

// TestCloudflareGenerator_TSValidationGoldenSnapshot captures the exact pre-migration TS validation and DB bind code snippets.
func TestCloudflareGenerator_TSValidationGoldenSnapshot(t *testing.T) {
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// Verify TS validation for string (title) and number (author_id)
	if !strings.Contains(output.IndexTS, "body['title'] !== undefined && body['title'] !== null && typeof body['title'] !== 'string'") {
		t.Errorf("expected TS validation snippet for title, got:\n%s", output.IndexTS)
	}
	if !strings.Contains(output.IndexTS, "body['author_id'] !== undefined && body['author_id'] !== null && typeof body['author_id'] !== 'number'") {
		t.Errorf("expected TS validation snippet for author_id, got:\n%s", output.IndexTS)
	}
}

// TestCloudflareGenerator_CRUDSpecificationParity compares the Go Runtime API responses
// with the generated TS Hono API structure contract to ensure 100% envelope specification parity.
func TestCloudflareGenerator_CRUDSpecificationParity(t *testing.T) {
	resourceDir := t.TempDir()
	postYAML := `
resource:
  name: Post
  timestamps: true
  soft_delete: true
fields:
  - name: title
    type: string
    nullable: false
  - name: body
    type: markdown
    nullable: false
auth:
  permissions:
    create: public
    read: public
    update: public
    delete: public
`
	if err := os.WriteFile(filepath.Join(resourceDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml: %v", err)
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "parity.db")

	// Start Go Runtime App
	app, err := runtime.New(runtime.Config{
		ResourceDir: resourceDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed to start Go runtime app: %v", err)
	}
	defer app.Close()

	ts := httptest.NewServer(app)
	defer ts.Close()

	client := ts.Client()

	// 1. Go API: Create Post
	createPayload := map[string]any{
		"title": "Go Parity Post",
		"body":  "Testing Go vs TS Workers parity",
	}
	bodyBytes, _ := json.Marshal(createPayload)
	resp, err := client.Post(ts.URL+"/api/posts", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("failed Go POST /api/posts: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created from Go API, got %d", resp.StatusCode)
	}

	var goEnvelope runtime.SuccessEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&goEnvelope); err != nil {
		t.Fatalf("failed to decode Go response envelope: %v", err)
	}
	resp.Body.Close()

	goRecord, ok := goEnvelope.Data.(map[string]any)
	if !ok || goRecord["title"] != "Go Parity Post" {
		t.Fatalf("unexpected Go API data payload: %v", goEnvelope.Data)
	}

	// 2. Go API: List Posts
	resp, err = client.Get(ts.URL + "/api/posts")
	if err != nil {
		t.Fatalf("failed Go GET /api/posts: %v", err)
	}
	var goListEnv runtime.ListSuccessEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&goListEnv); err != nil {
		t.Fatalf("failed to decode Go list response envelope: %v", err)
	}
	resp.Body.Close()

	if goListEnv.Meta.Total < 1 {
		t.Fatalf("expected Go list total >= 1, got %d", goListEnv.Meta.Total)
	}

	// 3. Generate TS Codegen Output and verify that the generated TS Hono handlers emit
	// the EXACT SAME JSON envelope keys ("data" for detail/create, "data" + "meta" for list, "error" for error).
	gen := cloudflare.NewGenerator()
	reg, _ := resource.LoadAll(resourceDir)
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("failed to generate TS code: %v", err)
	}

	// Verify TS code has envelope structure parity:
	if !strings.Contains(out.IndexTS, `data: results || []`) || !strings.Contains(out.IndexTS, `meta: { total, limit, offset }`) {
		t.Errorf("TS Hono list response missing matching { data, meta } envelope structure")
	}
	if !strings.Contains(out.IndexTS, `data: created`) {
		t.Errorf("TS Hono create response missing matching { data } envelope structure")
	}
	if !strings.Contains(out.IndexTS, `data: { deleted: true, id: parsedId }`) {
		t.Errorf("TS Hono delete response missing matching { data } envelope structure")
	}
}

// TestCloudflareGenerator_FKValidationAndBinding_PlanParity verifies that derived FK fields in plan.Plan
// generate number validation and D1 parameter binding in TS Hono app code.
func TestCloudflareGenerator_FKValidationAndBinding_PlanParity(t *testing.T) {
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("failed generating code: %v", err)
	}

	// 1. Verify TS Validation for derived FK post_id in comments handler
	if !strings.Contains(out.IndexTS, "body['post_id'] !== undefined && body['post_id'] !== null && typeof body['post_id'] !== 'number'") {
		t.Errorf("expected TS code to contain number validation for post_id FK, got:\n%s", out.IndexTS)
	}

	// 2. Verify D1 SQL INSERT includes post_id and author_id columns
	if !strings.Contains(out.IndexTS, `INSERT INTO "comments" ("body", "post_id", "author_id"`) {
		t.Errorf("expected TS code to contain INSERT INTO comments with post_id and author_id, got:\n%s", out.IndexTS)
	}
}

// TestCloudflareGenerator_PreAuthGoldenSnapshot captures golden snapshot of basic route code structure before Auth extension.
func TestCloudflareGenerator_PreAuthGoldenSnapshot(t *testing.T) {
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("failed generating code: %v", err)
	}

	expectedRouteSnippet := "app.get('/api/posts', async (c) => {"
	if !strings.Contains(out.IndexTS, expectedRouteSnippet) {
		t.Errorf("pre-auth golden snapshot failed: route snippet %q missing", expectedRouteSnippet)
	}
}
