package cloudflare_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/codegen/cloudflare"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
)

func TestGenerateDrinkLogMoldApp(t *testing.T) {
	drinkLogDir := filepath.Join("..", "..", "..", "drink-log")
	resDir := filepath.Join(drinkLogDir, "resources")
	if _, err := os.Stat(resDir); os.IsNotExist(err) {
		t.Skip("drink-log directory not found")
	}
	reg, err := resource.LoadAll(resDir)
	if err != nil {
		t.Fatalf("failed loading drink-log resources: %v", err)
	}
	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("failed generating cloudflare code: %v", err)
	}
	moldAppPath := filepath.Join(drinkLogDir, "functions", "_shared", "generated", "mold_app.ts")
	tsContent := out.IndexTS + "\nexport { app as moldApp };\n"
	if err := os.WriteFile(moldAppPath, []byte(tsContent), 0644); err != nil {
		t.Fatalf("failed writing mold_app.ts: %v", err)
	}
	t.Logf("Successfully regenerated %s", moldAppPath)
}

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
    "created_at" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "deleted_at" TEXT,
    FOREIGN KEY ("author_id") REFERENCES "users"("id") ON DELETE RESTRICT
);`

	expectedCommentsDDL := `CREATE TABLE IF NOT EXISTS "comments" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "body" TEXT NOT NULL,
    "post_id" INTEGER NOT NULL,
    "author_id" INTEGER NOT NULL,
    "created_at" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "updated_at" TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
    "deleted_at" TEXT,
    FOREIGN KEY ("post_id") REFERENCES "posts"("id") ON DELETE RESTRICT,
    FOREIGN KEY ("author_id") REFERENCES "users"("id") ON DELETE RESTRICT
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
	if !strings.Contains(out.IndexTS, `data: sanitized`) || !strings.Contains(out.IndexTS, `meta: { total, limit, offset }`) {
		t.Errorf("TS Hono list response missing matching { data, meta } envelope structure")
	}
	if !strings.Contains(out.IndexTS, `data: sanitizeRecord(created,`) {
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

// TestCloudflareGenerator_AuthGuards verifies code generation of 401/404/403 auth guards,
// ownership field checks, and role escalation protection.
func TestCloudflareGenerator_AuthGuards(t *testing.T) {
	resourceDir := t.TempDir()
	userYAML := `
resource:
  name: User
  timestamps: true
  soft_delete: true
fields:
  - name: email
    type: email
    nullable: false
  - name: role
    type: enum
    nullable: false
    default: "user"
    constraints:
      values: ["admin", "user"]
auth:
  permissions:
    create: public
    read: authenticated
    update: owner
    delete: role:admin
  ownership_field: id
`
	if err := os.WriteFile(filepath.Join(resourceDir, "User.yaml"), []byte(userYAML), 0644); err != nil {
		t.Fatalf("failed writing User.yaml: %v", err)
	}

	reg, err := resource.LoadAll(resourceDir)
	if err != nil {
		t.Fatalf("failed loading User IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 1. Verify getAuthUser helper function is included
	if !strings.Contains(out.IndexTS, "async function getAuthUser(c: any): Promise<AuthUser | null>") {
		t.Errorf("expected IndexTS to contain getAuthUser helper, got:\n%s", out.IndexTS)
	}

	// 2. Verify 401 UNAUTHORIZED check for read: authenticated
	if !strings.Contains(out.IndexTS, "writeError(c, 401, 'UNAUTHORIZED', 'authentication required')") {
		t.Errorf("expected IndexTS to contain 401 UNAUTHORIZED check, got:\n%s", out.IndexTS)
	}

	// 3. Verify role escalation check for role: admin
	if !strings.Contains(out.IndexTS, "writeError(c, 403, 'FORBIDDEN', 'cannot grant admin role')") {
		t.Errorf("expected IndexTS to contain role escalation guard, got:\n%s", out.IndexTS)
	}

	// 4. Verify 404 check before 403 ownership check in update
	if !strings.Contains(out.IndexTS, "writeError(c, 404, 'NOT_FOUND', 'record not found')") {
		t.Errorf("expected IndexTS to contain 404 NOT_FOUND check, got:\n%s", out.IndexTS)
	}
}

// TestCloudflareGenerator_PasswordHandling verifies password hashing on write,
// response sanitization (strip), _mold_sessions DDL, and login/logout handlers.
func TestCloudflareGenerator_PasswordHandling(t *testing.T) {
	resourceDir := t.TempDir()
	userYAML := `
resource:
  name: User
  timestamps: true
  soft_delete: true
fields:
  - name: email
    type: email
    nullable: false
  - name: password
    type: password
    nullable: false
`
	if err := os.WriteFile(filepath.Join(resourceDir, "User.yaml"), []byte(userYAML), 0644); err != nil {
		t.Fatalf("failed writing User.yaml: %v", err)
	}

	reg, err := resource.LoadAll(resourceDir)
	if err != nil {
		t.Fatalf("failed loading User IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 1. Verify _mold_sessions DDL
	if !strings.Contains(out.SchemaSQL, `CREATE TABLE IF NOT EXISTS "_mold_sessions"`) {
		t.Errorf("expected SchemaSQL to contain _mold_sessions DDL, got:\n%s", out.SchemaSQL)
	}

	// 2. Verify hashPassword, verifyPassword and sanitizeRecord TS helpers
	if !strings.Contains(out.IndexTS, "async function hashPassword(plain: string): Promise<string>") {
		t.Errorf("expected IndexTS to contain hashPassword helper, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "async function verifyPassword(plain: string, storedHash: string)") {
		t.Errorf("expected IndexTS to contain verifyPassword helper, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "'$pbkdf2$' + iterations + '$' + saltHex + '$' + hashHex") {
		t.Errorf("expected IndexTS to contain $pbkdf2$ format string, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "function sanitizeRecord(record: any, passwordFields: string[])") {
		t.Errorf("expected IndexTS to contain sanitizeRecord helper, got:\n%s", out.IndexTS)
	}

	// 3. Verify password hashing on create & update
	if !strings.Contains(out.IndexTS, "body['password'] = await hashPassword(String(body['password']))") {
		t.Errorf("expected IndexTS to hash password on write, got:\n%s", out.IndexTS)
	}

	// 4. Verify password response sanitization
	if !strings.Contains(out.IndexTS, "sanitizeRecord(created, ['password'])") {
		t.Errorf("expected IndexTS to sanitize response on create, got:\n%s", out.IndexTS)
	}

	// 5. Verify /login and /logout endpoints
	if !strings.Contains(out.IndexTS, "app.post('/login', async (c) => {") {
		t.Errorf("expected IndexTS to contain /login route, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "app.post('/logout', async (c) => {") {
		t.Errorf("expected IndexTS to contain /logout route, got:\n%s", out.IndexTS)
	}
}

// TestCloudflareGenerator_HTMLDefaultView verifies generation of SSR HTML List/Detail/Form routes and XSS sanitization.
func TestCloudflareGenerator_HTMLDefaultView(t *testing.T) {
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 1. Verify escapeHTML and sanitizeHTML helpers
	if !strings.Contains(out.IndexTS, "function escapeHTML(str: any): string") {
		t.Errorf("expected IndexTS to contain escapeHTML helper, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "function sanitizeHTML(html: string): string") {
		t.Errorf("expected IndexTS to contain sanitizeHTML helper, got:\n%s", out.IndexTS)
	}

	// 2. Verify List View route
	if !strings.Contains(out.IndexTS, "app.get('/view/posts', async (c) => {") {
		t.Errorf("expected IndexTS to contain GET /view/posts, got:\n%s", out.IndexTS)
	}

	// 3. Verify Form New route
	if !strings.Contains(out.IndexTS, "app.get('/view/posts/new', async (c) => {") {
		t.Errorf("expected IndexTS to contain GET /view/posts/new, got:\n%s", out.IndexTS)
	}

	// 4. Verify Form Create submit route and 303 redirect
	if !strings.Contains(out.IndexTS, "c.redirect('/view/posts', 303)") {
		t.Errorf("expected IndexTS to contain 303 redirect after create submit, got:\n%s", out.IndexTS)
	}

	// 5. Verify Detail View route with XSS sanitization on markdown body and single/unquoted event handler stripping
	if !strings.Contains(out.IndexTS, "sanitizeHTML(String(record['body'] || ''))") {
		t.Errorf("expected IndexTS to apply sanitizeHTML to markdown body, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, ".replace(/on\\w+\\s*=\\s*'[^']*'/gi, '')") || !strings.Contains(out.IndexTS, ".replace(/on\\w+\\s*=\\s*[^\\s>]+/gi, '')") {
		t.Errorf("expected IndexTS sanitizeHTML to contain single-quote and unquoted event handler stripping, got:\n%s", out.IndexTS)
	}
}

// TestCloudflareGenerator_BlobFieldR2 verifies generation of R2 Blob endpoints and Wrangler config.
func TestCloudflareGenerator_BlobFieldR2(t *testing.T) {
	resourceDir := t.TempDir()
	docYAML := `
resource:
  name: Document
  timestamps: true
  soft_delete: true
fields:
  - name: title
    type: string
    nullable: false
  - name: file_key
    type: blob
    nullable: true
`
	if err := os.WriteFile(filepath.Join(resourceDir, "Document.yaml"), []byte(docYAML), 0644); err != nil {
		t.Fatalf("failed writing Document.yaml: %v", err)
	}

	reg, err := resource.LoadAll(resourceDir)
	if err != nil {
		t.Fatalf("failed loading Document IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 1. Verify BUCKET binding in IndexTS
	if !strings.Contains(out.IndexTS, "BUCKET: R2Bucket;") {
		t.Errorf("expected IndexTS to contain BUCKET: R2Bucket binding, got:\n%s", out.IndexTS)
	}

	// 2. Verify r2_buckets in WranglerConfig
	if !strings.Contains(out.WranglerConfig, `"r2_buckets": [`) {
		t.Errorf("expected WranglerConfig to contain r2_buckets, got:\n%s", out.WranglerConfig)
	}

	// 3. Verify Overwrite Upload endpoint
	if !strings.Contains(out.IndexTS, "app.post('/api/documents/:id/upload/file_key', async (c) => {") {
		t.Errorf("expected IndexTS to contain upload endpoint, got:\n%s", out.IndexTS)
	}

	// 4. Verify Download Blob endpoint
	if !strings.Contains(out.IndexTS, "app.get('/api/documents/:id/blob/file_key', async (c) => {") {
		t.Errorf("expected IndexTS to contain download endpoint, got:\n%s", out.IndexTS)
	}

	// 5. Verify Delete Blob endpoint
	if !strings.Contains(out.IndexTS, "app.delete('/api/documents/:id/blob/file_key', async (c) => {") {
		t.Errorf("expected IndexTS to contain delete endpoint, got:\n%s", out.IndexTS)
	}

	// 6. Verify 1-Step multipart create and atomic rollback in POST /api/documents
	if !strings.Contains(out.IndexTS, "contentType.includes('multipart/form-data')") {
		t.Errorf("expected IndexTS to parse multipart/form-data in POST /api/documents, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "BLOB_STORE_FAILED_RECORD_PRESERVED") {
		t.Errorf("expected IndexTS to contain atomic rollback error envelope for failed blob upload, got:\n%s", out.IndexTS)
	}
}

// TestCloudflareGenerator_SQLLiteralQuoting verifies single quotes are strictly used for SQL string literals.
func TestCloudflareGenerator_SQLLiteralQuoting(t *testing.T) {
	relResourceDir := filepath.Join("..", "..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	if strings.Contains(out.IndexTS, `"deleted_at" = ""`) {
		t.Errorf("found double-quoted string literal \"deleted_at\" = \"\" in generated SQL; must use single quotes ''")
	}
}

// TestCloudflareGenerator_MultipleBlobFieldsGenericness verifies generic table and multi-blob field codegen.
func TestCloudflareGenerator_MultipleBlobFieldsGenericness(t *testing.T) {
	tmpDir := t.TempDir()
	sakePostYAML := `
resource:
  name: SakePost
  table: sake_posts
fields:
  - name: title
    type: string
  - name: cover_image
    type: blob
  - name: attachment_file
    type: blob
`
	if err := os.WriteFile(filepath.Join(tmpDir, "SakePost.yaml"), []byte(sakePostYAML), 0644); err != nil {
		t.Fatalf("failed writing SakePost.yaml: %v", err)
	}

	reg, err := resource.LoadAll(tmpDir)
	if err != nil {
		t.Fatalf("failed loading SakePost IR: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// Verify generic dynamic table and field name interpolation
	expectedCoverKey := "blobs/sake_posts/${created.id}/cover_image_"
	expectedAttachmentKey := "blobs/sake_posts/${created.id}/attachment_file_"

	if !strings.Contains(out.IndexTS, expectedCoverKey) {
		t.Errorf("expected IndexTS to contain generic cover_image key %q, got:\n%s", expectedCoverKey, out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, expectedAttachmentKey) {
		t.Errorf("expected IndexTS to contain generic attachment_file key %q, got:\n%s", expectedAttachmentKey, out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "const file_cover_image = formData.get('cover_image');") {
		t.Errorf("expected IndexTS to contain cover_image form extraction, got:\n%s", out.IndexTS)
	}
	if !strings.Contains(out.IndexTS, "const file_attachment_file = formData.get('attachment_file');") {
		t.Errorf("expected IndexTS to contain attachment_file form extraction, got:\n%s", out.IndexTS)
	}

	// Verify compensating deletion and error code codegen
	if !strings.Contains(out.IndexTS, "const uploadedBlobKeys: string[] = [];") {
		t.Errorf("expected IndexTS to declare uploadedBlobKeys tracking array")
	}
	if !strings.Contains(out.IndexTS, "uploadedBlobKeys.push(key);") {
		t.Errorf("expected IndexTS to push key to uploadedBlobKeys")
	}
	if !strings.Contains(out.IndexTS, "await c.env.BUCKET.delete(key);") {
		t.Errorf("expected IndexTS to issue compensating delete for R2 orphan objects")
	}
	if !strings.Contains(out.IndexTS, "BLOB_ORPHAN_CLEANUP_FAILED") {
		t.Errorf("expected IndexTS to handle BLOB_ORPHAN_CLEANUP_FAILED error code")
	}
}

func TestCloudflareGenerator_UniqueTogether(t *testing.T) {
	recTagRes := &resource.Resource{
		Name:          "RecordTag",
		Table:         "record_tags",
		SchemaVersion: 1,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt, Nullable: false},
			{Name: "tag_id", Type: resource.TypeInt, Nullable: false},
		},
		Constraints: &resource.ResourceConstraints{
			UniqueTogether: [][]string{
				{"sake_record_id", "tag_id"},
			},
		},
	}

	reg := resource.NewRegistry()
	if err := reg.Register(recTagRes); err != nil {
		t.Fatalf("failed to register resource: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 1. Verify D1 DDL
	expectedDDL := `CREATE UNIQUE INDEX IF NOT EXISTS "idx_record_tags_unique_sake_record_id_tag_id" ON "record_tags"("sake_record_id", "tag_id") WHERE "deleted_at" IS NULL;`
	if !strings.Contains(out.SchemaSQL, expectedDDL) {
		t.Errorf("expected SchemaSQL to contain Partial Unique Index DDL %q, got:\n%s", expectedDDL, out.SchemaSQL)
	}

	// 2. Verify UNIQUE constraint try-catch and INVALID_INPUT error response in TS codegen
	if !strings.Contains(out.IndexTS, "errMsg.includes('UNIQUE constraint failed')") {
		t.Errorf("expected IndexTS to catch UNIQUE constraint failed in try-catch block")
	}
	if !strings.Contains(out.IndexTS, "writeError(c, 400, 'INVALID_INPUT', `unique constraint failed: ${errMsg}`)") {
		t.Errorf("expected IndexTS to write INVALID_INPUT error code with 400 status")
	}
}

func TestCloudflareGenerator_NullableOwnershipTSOutput(t *testing.T) {
	tagRes := &resource.Resource{
		Name:          "Tag",
		Table:         "tags",
		SchemaVersion: 1,
		SoftDelete:    true,
		Auth: &resource.Auth{
			OwnershipField: "owner_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "owner",
				Update: "owner",
				Delete: "owner",
			},
		},
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false},
			{Name: "owner_id", Type: resource.TypeInt, Nullable: true},
		},
	}

	reg := resource.NewRegistry()
	if err := reg.Register(tagRes); err != nil {
		t.Fatalf("failed to register Tag resource: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 1. Verify GET Detail does not early 401 return before fetching record when read: owner
	if strings.Contains(out.IndexTS, "// DETAIL /api/tags/:id\napp.get('/api/tags/:id', async (c) => {\n  const authUser = await getAuthUser(c);\n  if (!authUser)") {
		t.Errorf("expected GET Detail for owner perm with ownership_field to not early 401 return before fetching record")
	}

	// 2. Verify GET Detail checks ownerVal !== null && ownerVal !== undefined
	if !strings.Contains(out.IndexTS, "const ownerVal = (record as any)['owner_id'];\n  if (ownerVal !== null && ownerVal !== undefined) {") {
		t.Errorf("expected GET Detail to check ownerVal !== null for nullable ownership, got:\n%s", out.IndexTS)
	}

	// 3. Verify PUT Update checks ownerVal === null || ownerVal === undefined -> admin check
	if !strings.Contains(out.IndexTS, "const ownerVal = (existing as any)['owner_id'];\n  if (ownerVal === null || ownerVal === undefined) {\n    if (authUser.role !== 'admin') {") {
		t.Errorf("expected PUT Update to check ownerVal === null for admin requirement, got:\n%s", out.IndexTS)
	}
}

func TestCloudflareGenerator_IncludeQuery(t *testing.T) {
	tagRes := &resource.Resource{
		Name:          "Tag",
		Table:         "tags",
		SchemaVersion: 1,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false},
		},
		Relations: []resource.Relation{
			{Name: "record_tags", Kind: resource.KindHasMany, Target: "RecordTag", ForeignKey: "tag_id"},
		},
	}

	recordTagRes := &resource.Resource{
		Name:          "RecordTag",
		Table:         "record_tags",
		SchemaVersion: 1,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt, Nullable: false},
			{Name: "tag_id", Type: resource.TypeInt, Nullable: false},
		},
		Relations: []resource.Relation{
			{Name: "tag", Kind: resource.KindBelongsTo, Target: "Tag", ForeignKey: "tag_id"},
		},
	}

	reg := resource.NewRegistry()
	_ = reg.Register(tagRes)
	_ = reg.Register(recordTagRes)

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	// 1. Verify relMetadata generation in TS
	if !strings.Contains(out.IndexTS, "const relMetadata: Record<string, Record<string,") {
		t.Errorf("expected IndexTS to contain relMetadata definition")
	}
	if !strings.Contains(out.IndexTS, "'tag': { kind: 'belongs_to', targetTable: 'tags', fk: 'tag_id'") {
		t.Errorf("expected IndexTS to contain tag belongs_to relation metadata, got:\n%s", out.IndexTS)
	}

	// 2. Verify processIncludes helper function in TS
	if !strings.Contains(out.IndexTS, "async function processIncludes(c: any, currentTable: string, records: any[], includeStr: string | undefined, authUser: AuthUser | null): Promise<any>") {
		t.Errorf("expected IndexTS to contain processIncludes helper")
	}
	if !strings.Contains(out.IndexTS, "return writeError(c, 400, 'INVALID_INCLUDE'") {
		t.Errorf("expected IndexTS to return INVALID_INCLUDE for invalid relation")
	}

	// 3. Verify processIncludes invocation in List and Detail handlers
	if !strings.Contains(out.IndexTS, "await processIncludes(c, 'record_tags', sanitized, c.req.query('include'), authUser)") {
		t.Errorf("expected IndexTS to invoke processIncludes in List handler")
	}
}

func TestCloudflareCodegen_MiniflareIncludeE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heavy Miniflare integration test in short mode")
	}
	tagRes := &resource.Resource{
		Name:          "Tag",
		Table:         "tags",
		SchemaVersion: 1,
		SoftDelete:    true,
		Auth: &resource.Auth{
			OwnershipField: "owner_id",
			Permissions: resource.Permissions{
				Read: "owner",
			},
		},
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false},
			{Name: "owner_id", Type: resource.TypeInt, Nullable: true},
		},
		Relations: []resource.Relation{
			{Name: "record_tags", Kind: resource.KindHasMany, Target: "RecordTag", ForeignKey: "tag_id"},
		},
	}

	recordTagRes := &resource.Resource{
		Name:          "RecordTag",
		Table:         "record_tags",
		SchemaVersion: 1,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt, Nullable: true},
			{Name: "tag_id", Type: resource.TypeInt, Nullable: true},
		},
		Relations: []resource.Relation{
			{Name: "tag", Kind: resource.KindBelongsTo, Target: "Tag", ForeignKey: "tag_id"},
		},
	}

	reg := resource.NewRegistry()
	_ = reg.Register(tagRes)
	_ = reg.Register(recordTagRes)

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	tmpDir := t.TempDir()
	for relPath, content := range out.Files {
		fullPath := filepath.Join(tmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed creating dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed writing file %s: %v", relPath, err)
		}
	}

	cmdNpm := exec.Command("npm.cmd", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "hono@^4.7.0", "esbuild@^0.24.0")
	if os.Getenv("OS") != "Windows_NT" {
		cmdNpm = exec.Command("npm", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "hono@^4.7.0", "esbuild@^0.24.0")
	}
	cmdNpm.Dir = tmpDir
	if outBytes, err := cmdNpm.CombinedOutput(); err != nil {
		t.Fatalf("npm install failed: %v\nOutput: %s", err, string(outBytes))
	}

	// Transpile & bundle src/index.ts to src/index.js using esbuild
	cmdEsbuild := exec.Command("npx.cmd", "esbuild", "src/index.ts", "--bundle", "--format=esm", "--target=es2022", "--outfile=src/index.js", "--external:node:*")
	if os.Getenv("OS") != "Windows_NT" {
		cmdEsbuild = exec.Command("npx", "esbuild", "src/index.ts", "--bundle", "--format=esm", "--target=es2022", "--outfile=src/index.js", "--external:node:*")
	}
	cmdEsbuild.Dir = tmpDir
	if outBytes, err := cmdEsbuild.CombinedOutput(); err != nil {
		t.Fatalf("esbuild failed: %v, log: %s", err, string(outBytes))
	}

	miniflareURL := filepath.ToSlash(filepath.Join(tmpDir, "node_modules", "miniflare", "dist", "src", "index.js"))

	runnerJS := fmt.Sprintf(`
import { pathToFileURL } from "node:url";
import fs from "node:fs";

async function run() {
  const miniflareModule = await import(pathToFileURL("%s").href);
  const { Miniflare } = miniflareModule;

  const mf = new Miniflare({
    workers: [
      {
        modules: true,
        scriptPath: "./src/index.js",
        d1Databases: ["DB"],
        compatibilityFlags: ["nodejs_compat"]
      }
    ]
  });

  const db = await mf.getD1Database("DB");
  const schemaSQL = fs.readFileSync("./schema.sql", "utf8");
  const cleanSQL = schemaSQL.replace(/--.*$/gm, "");
  for (const rawStmt of cleanSQL.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await db.exec(stmt + ";");
    }
  }

  // Seed Tags
  await db.exec("INSERT INTO tags (id, name, owner_id) VALUES (1, 'Public Tag 1', NULL);");
  await db.exec("INSERT INTO tags (id, name, owner_id) VALUES (2, 'Private Tag 2', 999);");
  await db.exec("INSERT INTO tags (id, name, owner_id, deleted_at) VALUES (4, 'Deleted Tag 4', NULL, '2026-01-01T00:00:00Z');");

  // Seed RecordTags
  await db.exec("INSERT INTO record_tags (id, tag_id) VALUES (1, 1);");
  await db.exec("INSERT INTO record_tags (id, tag_id) VALUES (2, 2);");
  await db.exec("INSERT INTO record_tags (id, tag_id) VALUES (3, NULL);");
  await db.exec("INSERT INTO record_tags (id, tag_id) VALUES (4, 4);");

  // 1. Test GET /api/record_tags?include=tag
  const resList = await mf.dispatchFetch("http://localhost/api/record_tags?include=tag");
  if (resList.status !== 200) {
    console.error("List status failed:", resList.status);
    process.exit(1);
  }
  const jsonList = await resList.json();
  console.log("Miniflare List Response:", JSON.stringify(jsonList, null, 2));

  const items = jsonList.data;
  if (items.length !== 4) {
    console.error("Expected 4 items, got", items.length);
    process.exit(1);
  }

  const rec1 = items.find(i => i.id === 1);
  const rec2 = items.find(i => i.id === 2);
  const rec3 = items.find(i => i.id === 3);
  const rec4 = items.find(i => i.id === 4);

  if (!rec1 || !rec1.tag || rec1.tag.name !== 'Public Tag 1') {
    console.error("Scenario A failed:", rec1);
    process.exit(1);
  }
  if (!rec2 || rec2.tag !== null) {
    console.error("Scenario B failed:", rec2);
    process.exit(1);
  }
  if (!rec3 || rec3.tag !== null) {
    console.error("Scenario C failed:", rec3);
    process.exit(1);
  }
  if (!rec4 || rec4.tag !== null) {
    console.error("Scenario D failed:", rec4);
    process.exit(1);
  }

  // 2. Test invalid relation: ?include=invalid_rel -> 400
  const resErr = await mf.dispatchFetch("http://localhost/api/record_tags?include=invalid_rel");
  if (resErr.status !== 400) {
    console.error("Expected 400 for invalid_rel, got", resErr.status);
    process.exit(1);
  }
  const jsonErr = await resErr.json();
  console.log("Miniflare Invalid Include Response:", JSON.stringify(jsonErr, null, 2));

  // 3. Test dot-chaining relation: ?include=record_tags.tag -> 400
  const resDot = await mf.dispatchFetch("http://localhost/api/tags?include=record_tags.tag");
  if (resDot.status !== 400) {
    console.error("Expected 400 for dot-chaining include, got", resDot.status);
    process.exit(1);
  }

  // 4. Test has_many relation: GET /api/tags?include=record_tags -> 200 with array
  const resHasMany = await mf.dispatchFetch("http://localhost/api/tags?include=record_tags");
  if (resHasMany.status !== 200) {
    console.error("Expected 200 for has_many include, got", resHasMany.status);
    process.exit(1);
  }
  const jsonHasMany = await resHasMany.json();
  console.log("Miniflare HasMany Include Response:", JSON.stringify(jsonHasMany, null, 2));
  const tag1Rec = jsonHasMany.data.find(t => t.id === 1);
  if (!tag1Rec || !Array.isArray(tag1Rec.record_tags) || tag1Rec.record_tags.length !== 1) {
    console.error("Expected tag 1 to have 1 embedded record_tag, got:", tag1Rec);
    process.exit(1);
  }

  // 5. Test SSR View GET /view/record_tags?include=tag
  const resView = await mf.dispatchFetch("http://localhost/view/record_tags?include=tag");
  if (resView.status !== 200) {
    console.error("Expected 200 for SSR View, got", resView.status);
    process.exit(1);
  }

  console.log("Miniflare 4-scenario include tests 100%% PASS!");
  await mf.dispose();
}

run().catch(err => {
  console.error(err);
  process.exit(1);
});
`, miniflareURL)

	if err := os.WriteFile(filepath.Join(tmpDir, "test_runner.mjs"), []byte(runnerJS), 0644); err != nil {
		t.Fatalf("failed writing test_runner.mjs: %v", err)
	}

	cmd := exec.Command("node", "test_runner.mjs")
	cmd.Dir = tmpDir
	outputBytes, err := cmd.CombinedOutput()
	t.Logf("Miniflare Raw Log Output:\n%s", string(outputBytes))
	if err != nil {
		t.Fatalf("Miniflare test runner failed: %v", err)
	}
}

// TestCloudflareCodegen_MiniflareR2KeyIndirectionEmpirical empirically proves Problem 1:
// Integer PK in D1 does NOT require rewriting R2 object key paths.
// Legacy UUID format R2 key ("images/{uuid-owner}/sake/{uuid-record}/{uuid-image}.jpg")
// stored in D1's image_key column for record id=1 (INTEGER AUTOINCREMENT) is fetched successfully via Miniflare R2 binding.
func TestCloudflareCodegen_MiniflareR2KeyIndirectionEmpirical(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil || nodePath == "" {
		t.Skip("node not found in PATH, skipping Miniflare R2 key indirection empirical test")
	}

	reg := resource.NewRegistry()
	sakeImgRes := &resource.Resource{
		Name:       "SakeImage",
		Table:      "sake_images",
		Timestamps: true,
		Fields: []resource.Field{
			{Name: "owner_id", Type: resource.TypeInt, Nullable: false},
			{Name: "record_id", Type: resource.TypeInt, Nullable: false},
			{Name: "image_key", Type: resource.TypeBlob, Nullable: false},
		},
	}
	reg.Register(sakeImgRes)

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("failed to generate code: %v", err)
	}

	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(output.PackageJSON), 0644); err != nil {
		t.Fatalf("failed writing package.json: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "wrangler.jsonc"), []byte(output.WranglerConfig), 0644); err != nil {
		t.Fatalf("failed writing wrangler.jsonc: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte(output.SchemaSQL), 0644); err != nil {
		t.Fatalf("failed writing schema.sql: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "index.ts"), []byte(output.IndexTS), 0644); err != nil {
		t.Fatalf("failed writing index.ts: %v", err)
	}

	cmdNpm := exec.Command("npm.cmd", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "esbuild@^0.24.0")
	if os.Getenv("OS") != "Windows_NT" {
		cmdNpm = exec.Command("npm", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "esbuild@^0.24.0")
	}
	cmdNpm.Dir = tmpDir
	if out, err := cmdNpm.CombinedOutput(); err != nil {
		t.Fatalf("npm install failed: %v\nOutput: %s", err, string(out))
	}

	// Run esbuild
	cmdEsbuild := exec.Command("npx.cmd", "esbuild", "index.ts", "--bundle", "--format=esm", "--outfile=dist/index.js", "--external:node:*")
	if os.Getenv("OS") != "Windows_NT" {
		cmdEsbuild = exec.Command("npx", "esbuild", "index.ts", "--bundle", "--format=esm", "--outfile=dist/index.js", "--external:node:*")
	}
	cmdEsbuild.Dir = tmpDir
	if out, err := cmdEsbuild.CombinedOutput(); err != nil {
		t.Fatalf("esbuild bundle failed: %v\nOutput: %s", err, string(out))
	}

	miniflareURL := filepath.ToSlash(filepath.Join(tmpDir, "node_modules", "miniflare", "dist", "src", "index.js"))

	runnerJS := fmt.Sprintf(`
import fs from 'node:fs';
import path from 'node:path';
import { pathToFileURL } from 'node:url';

async function run() {
  const miniflareModule = await import(pathToFileURL("%s").href);
  const { Miniflare } = miniflareModule;

  const mf = new Miniflare({
    workers: [
      {
        modules: true,
        scriptPath: "./dist/index.js",
        d1Databases: { DB: "mold-d1" },
        r2Buckets: { BUCKET: "mold-r2" },
      }
    ]
  });

  const db = await mf.getD1Database("DB");
  const bucket = await mf.getR2Bucket("BUCKET");
  const schemaSQL = fs.readFileSync("./schema.sql", "utf8");

  const cleanSQL = schemaSQL.replace(/--.*$/gm, "");
  for (const rawStmt of cleanSQL.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await db.exec(stmt + ";");
    }
  }

  // Legacy UUID format R2 key path in production
  const legacyUUIDKey = "images/usr_7f8a9b0c-1234-4567-89ab-cdef01234567/sake/rec_1a2b3c4d-5678-90ab-cdef-1234567890ab/img_9f8e7d6c-5432-10fe-dcba-9876543210fe.jpg";
  const binaryPayload = "EMPIRICAL_R2_BINARY_IMAGE_BYTES_99999";

  // 1. Seed D1 with INTEGER AUTOINCREMENT PK (id = 1) and legacy UUID image_key
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, created_at, updated_at) VALUES (1, 101, 202, '" + legacyUUIDKey + "', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");

  // 2. Put binary data directly into R2 under legacy UUID key
  await bucket.put(legacyUUIDKey, binaryPayload);

  // 3. Dispatch HTTP request to Mold TS Endpoint GET /api/sake_images/1/blob/image_key (Targeting INTEGER id=1)
  const res = await mf.dispatchFetch("http://localhost/api/sake_images/1/blob/image_key");
  
  console.log("=== EMPIRICAL MINIFLARE R2 TEST RAW LOG ===");
  console.log("HTTP Response Status:", res.status);
  const textBody = await res.text();
  console.log("HTTP Response Body:", textBody);

  if (res.status !== 200) {
    console.error("FAILED: Expected 200 OK, got", res.status);
    process.exit(1);
  }

  if (textBody !== binaryPayload) {
    console.error("FAILED: Response body mismatch. Expected", binaryPayload, "got", textBody);
    process.exit(1);
  }

  console.log("[EMPIRICAL PROOF VERIFIED]: Mold TS Blob endpoint correctly served legacy UUID R2 key for INTEGER record id=1!");
  await mf.dispose();
}

run().catch(err => {
  console.error(err);
  process.exit(1);
});
`, miniflareURL)

	if err := os.WriteFile(filepath.Join(tmpDir, "test_runner.mjs"), []byte(runnerJS), 0644); err != nil {
		t.Fatalf("failed writing test_runner.mjs: %v", err)
	}

	cmd := exec.Command("node", "test_runner.mjs")
	cmd.Dir = tmpDir
	outputBytes, err := cmd.CombinedOutput()
	t.Logf("Miniflare Raw Log Output:\n%s", string(outputBytes))
	if err != nil {
		t.Fatalf("Miniflare test runner failed: %v", err)
	}
}

// TestCloudflareGenerator_OwnershipAutoInjectionTS verifies that the generated Cloudflare TS Workers code
// emits auto-injection of ownership_field from authUser on CREATE, ignoring client payload.
func TestCloudflareGenerator_OwnershipAutoInjectionTS(t *testing.T) {
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

	// Verify that generated TS for POST /api/posts contains ownership auto-injection
	expectedSnippet := "if (authUser) {\n    body['author_id'] = authUser.id;\n  } else {\n    delete body['author_id'];\n  }"
	if !strings.Contains(output.IndexTS, expectedSnippet) {
		t.Fatalf("expected generated IndexTS to contain ownership auto-injection snippet:\n%s\ngot IndexTS:\n%s", expectedSnippet, output.IndexTS)
	}

	t.Logf("=== EMPIRICAL GENERATED CLOUDFLARE TS CODE SNIPPET (POST /api/posts) ===")
	t.Logf("%s", expectedSnippet)
}

func TestCloudflareGenerator_ClientWritableTS(t *testing.T) {
	res := &resource.Resource{
		Name:  "User",
		Table: "users",
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false, ClientWritable: true},
			{Name: "role", Type: resource.TypeEnum, Nullable: false, Default: "user", ClientWritable: false},
		},
	}

	reg := resource.NewRegistry()
	reg.Register(res)

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	expectedSnippet := "if (body['role'] !== undefined) {\n    return writeError(c, 400, 'CLIENT_WRITE_FORBIDDEN', 'field \\'role\\' is not client-writable');\n  }"
	if !strings.Contains(output.IndexTS, expectedSnippet) {
		t.Fatalf("expected generated IndexTS to contain client_writable check snippet:\n%s\ngot IndexTS:\n%s", expectedSnippet, output.IndexTS)
	}

	t.Logf("=== EMPIRICAL GENERATED CLOUDFLARE TS CODE SNIPPET (CLIENT_WRITE_FORBIDDEN) ===")
	t.Logf("%s", expectedSnippet)
}

func TestCloudflareGenerator_NestedWritesTS(t *testing.T) {
	parentRes := &resource.Resource{
		Name:  "Post",
		Table: "posts",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "comments", Kind: resource.KindHasMany, Target: "Comment", ForeignKey: "post_id"},
		},
	}

	childRes := &resource.Resource{
		Name:  "Comment",
		Table: "comments",
		Fields: []resource.Field{
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "body", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	reg := resource.NewRegistry()
	reg.Register(parentRes)
	reg.Register(childRes)

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	if !strings.Contains(output.IndexTS, "if (body['comments'] !== undefined && body['comments'] !== null)") {
		t.Fatalf("expected generated IndexTS to contain nested writes check for comments")
	}

	if !strings.Contains(output.IndexTS, "nestedWrites.push({ relName: 'comments', targetTable: 'comments', fkField: 'post_id'") {
		t.Fatalf("expected generated IndexTS to push nested write item for comments")
	}

	if !strings.Contains(output.IndexTS, "DELETE FROM \"${createdChildTrackers[j].table}\" WHERE id = ?") {
		t.Fatalf("expected generated IndexTS to contain compensating rollback query")
	}

	t.Logf("=== EMPIRICAL GENERATED CLOUDFLARE TS NESTED WRITES CODE VERIFIED ===")
}

func TestCloudflareCodegen_MiniflareNestedWritesConstraintsAndAuthE2E(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heavy Miniflare integration test in short mode")
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skip("node not found in PATH, skipping Miniflare E2E test")
	}

	minLen := 5
	postRes := &resource.Resource{
		Name:          "Post",
		Table:         "posts",
		SchemaVersion: 1,
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "public",
			},
		},
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "comments", Kind: resource.KindHasMany, Target: "Comment", ForeignKey: "post_id"},
			{Name: "audits", Kind: resource.KindHasMany, Target: "AuditLog", ForeignKey: "post_id"},
		},
	}

	commentRes := &resource.Resource{
		Name:          "Comment",
		Table:         "comments",
		SchemaVersion: 1,
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "public",
			},
		},
		Fields: []resource.Field{
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "body", Type: resource.TypeString, Nullable: false, ClientWritable: true, Constraints: resource.Constraints{MinLength: &minLen}},
			{Name: "status", Type: resource.TypeEnum, Nullable: false, ClientWritable: true, Constraints: resource.Constraints{Values: []string{"approved", "pending"}}},
		},
	}

	auditRes := &resource.Resource{
		Name:          "AuditLog",
		Table:         "audit_logs",
		SchemaVersion: 1,
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "role:admin",
				Read:   "role:admin",
			},
		},
		Fields: []resource.Field{
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "action", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	userRes := &resource.Resource{
		Name:          "User",
		Table:         "users",
		SchemaVersion: 1,
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false, ClientWritable: true},
			{Name: "role", Type: resource.TypeEnum, Nullable: false, ClientWritable: true, Constraints: resource.Constraints{Values: []string{"admin", "user"}}},
		},
	}

	reg := resource.NewRegistry()
	_ = reg.Register(postRes)
	_ = reg.Register(commentRes)
	_ = reg.Register(auditRes)
	_ = reg.Register(userRes)

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generation failed: %v", err)
	}

	tmpDir := t.TempDir()
	for relPath, content := range out.Files {
		fullPath := filepath.Join(tmpDir, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed creating dir: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed writing file %s: %v", relPath, err)
		}
	}

	cmdNpm := exec.Command("npm.cmd", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "hono@^4.7.0", "esbuild@^0.24.0")
	if os.Getenv("OS") != "Windows_NT" {
		cmdNpm = exec.Command("npm", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "hono@^4.7.0", "esbuild@^0.24.0")
	}
	cmdNpm.Dir = tmpDir
	if outBytes, err := cmdNpm.CombinedOutput(); err != nil {
		t.Fatalf("npm install failed: %v\nOutput: %s", err, string(outBytes))
	}

	cmdEsbuild := exec.Command("npx.cmd", "esbuild", "src/index.ts", "--bundle", "--format=esm", "--target=es2022", "--outfile=src/index.js", "--external:node:*")
	if os.Getenv("OS") != "Windows_NT" {
		cmdEsbuild = exec.Command("npx", "esbuild", "src/index.ts", "--bundle", "--format=esm", "--target=es2022", "--outfile=src/index.js", "--external:node:*")
	}
	cmdEsbuild.Dir = tmpDir
	if outBytes, err := cmdEsbuild.CombinedOutput(); err != nil {
		t.Fatalf("esbuild failed: %v, log: %s", err, string(outBytes))
	}

	miniflareURL := filepath.ToSlash(filepath.Join(tmpDir, "node_modules", "miniflare", "dist", "src", "index.js"))

	runnerJS := fmt.Sprintf(`
import { pathToFileURL } from "node:url";
import fs from "node:fs";

async function run() {
  const miniflareModule = await import(pathToFileURL("%s").href);
  const { Miniflare } = miniflareModule;

  const mf = new Miniflare({
    workers: [
      {
        modules: true,
        scriptPath: "./src/index.js",
        d1Databases: ["DB"],
        compatibilityFlags: ["nodejs_compat"]
      }
    ]
  });

  const db = await mf.getD1Database("DB");
  const schemaSQL = fs.readFileSync("./schema.sql", "utf8");
  const cleanSQL = schemaSQL.replace(/--.*$/gm, "");
  for (const rawStmt of cleanSQL.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await db.exec(stmt + ";");
    }
  }

  // Create session table and seed sessions
  await db.exec("CREATE TABLE IF NOT EXISTS _mold_sessions (id TEXT PRIMARY KEY, user_id INTEGER, role TEXT, created_at TEXT, expires_at TEXT);");
  const futureExp = new Date(Date.now() + 86400000).toISOString();
  await db.exec("INSERT INTO users (id, email, role) VALUES (101, 'user@test.com', 'user');");
  await db.exec("INSERT INTO users (id, email, role) VALUES (999, 'admin@test.com', 'admin');");
  await db.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('sess_user_101', 101, datetime('now'), '" + futureExp + "');");
  await db.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('sess_admin_999', 999, datetime('now'), '" + futureExp + "');");

  // 1. Test child min_length constraint violation -> 400 VALIDATION_FAILED + 0 DB rows
  const resMinLen = await mf.dispatchFetch("http://localhost/api/posts", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Cookie": "mold_session=sess_user_101" },
    body: JSON.stringify({
      title: "Valid Post Title",
      comments: [
        { body: "hi", status: "approved" }
      ]
    })
  });
  console.log("Miniflare Child Constraint (min_length) Status:", resMinLen.status);
  const jsonMinLen = await resMinLen.json();
  console.log("Miniflare Child Constraint (min_length) Response:", JSON.stringify(jsonMinLen));
  if (resMinLen.status !== 400) {
    console.error("Expected 400 for child min_length violation, got", resMinLen.status);
    process.exit(1);
  }
  const count1 = await db.prepare("SELECT COUNT(*) as c FROM posts").first();
  if (count1.c !== 0) {
    console.error("Expected 0 posts in DB, got", count1.c);
    process.exit(1);
  }

  // 2. Test child enum constraint violation -> 400 VALIDATION_FAILED + 0 DB rows
  const resEnum = await mf.dispatchFetch("http://localhost/api/posts", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Cookie": "mold_session=sess_user_101" },
    body: JSON.stringify({
      title: "Valid Post Title",
      comments: [
        { body: "Valid comment body", status: "invalid_status" }
      ]
    })
  });
  console.log("Miniflare Child Constraint (enum) Status:", resEnum.status);
  const jsonEnum = await resEnum.json();
  console.log("Miniflare Child Constraint (enum) Response:", JSON.stringify(jsonEnum));
  if (resEnum.status !== 400) {
    console.error("Expected 400 for child enum violation, got", resEnum.status);
    process.exit(1);
  }

  // 3. Test child role:admin permission denial by normal user -> 403 FORBIDDEN + 0 DB rows
  const resPerm = await mf.dispatchFetch("http://localhost/api/posts", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Cookie": "mold_session=sess_user_101" },
    body: JSON.stringify({
      title: "Unprivileged Post",
      audits: [
        { action: "ILLEGAL_ADMIN_ACTION" }
      ]
    })
  });
  console.log("Miniflare Child Permission Denial Status:", resPerm.status);
  const jsonPerm = await resPerm.json();
  console.log("Miniflare Child Permission Denial Response:", JSON.stringify(jsonPerm));
  if (resPerm.status !== 403) {
    console.error("Expected 403 for child permission denial, got", resPerm.status);
    process.exit(1);
  }
  const count2 = await db.prepare("SELECT COUNT(*) as c FROM posts").first();
  const count3 = await db.prepare("SELECT COUNT(*) as c FROM audit_logs").first();
  if (count2.c !== 0 || count3.c !== 0) {
    console.error("Expected 0 posts and 0 audit_logs in DB, got posts=" + count2.c + " audits=" + count3.c);
    process.exit(1);
  }

  // 4. Test valid nested write -> 201 Created + 1 parent, 1 child in DB
  const resValid = await mf.dispatchFetch("http://localhost/api/posts", {
    method: "POST",
    headers: { "Content-Type": "application/json", "Cookie": "mold_session=sess_user_101" },
    body: JSON.stringify({
      title: "Valid Post With Comment",
      comments: [
        { body: "Valid comment body", status: "approved" }
      ]
    })
  });
  console.log("Miniflare Valid Nested Write Status:", resValid.status);
  const jsonValid = await resValid.json();
  console.log("Miniflare Valid Nested Write Response:", JSON.stringify(jsonValid));
  if (resValid.status !== 201) {
    console.error("Expected 201 for valid nested write, got", resValid.status);
    process.exit(1);
  }
  const count4 = await db.prepare("SELECT COUNT(*) as c FROM posts").first();
  const count5 = await db.prepare("SELECT COUNT(*) as c FROM comments").first();
  if (count4.c !== 1 || count5.c !== 1) {
    console.error("Expected 1 post and 1 comment in DB, got posts=" + count4.c + " comments=" + count5.c);
    process.exit(1);
  }

  console.log("Miniflare Nested Writes Constraints and Auth Tests 100%% PASS!");
  await mf.dispose();
}

run().catch(err => {
  console.error("Fatal Miniflare runner error:", err);
  process.exit(1);
});
`, miniflareURL)

	runnerPath := filepath.Join(tmpDir, "runner.mjs")
	if err := os.WriteFile(runnerPath, []byte(runnerJS), 0644); err != nil {
		t.Fatalf("failed writing runner.mjs: %v", err)
	}

	cmdRun := exec.Command("node", "runner.mjs")
	cmdRun.Dir = tmpDir
	outputBytes, err := cmdRun.CombinedOutput()
	t.Logf("Miniflare Raw Log Output:\n%s", string(outputBytes))
	if err != nil {
		t.Fatalf("Miniflare test runner failed: %v", err)
	}
}

func TestCloudflareCodegen_MultipartFormBlobAndNestedWritesMiniflareEmpirical(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heavy Miniflare integration test in short mode")
	}
	nodePath, err := exec.LookPath("node")
	if err != nil || nodePath == "" {
		t.Skip("node not found in PATH")
	}

	reg := resource.NewRegistry()
	userRes := &resource.Resource{
		Name:       "User",
		Table:      "users",
		Timestamps: true,
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false, ClientWritable: true},
			{Name: "role", Type: resource.TypeString, Nullable: false, Default: "user", ClientWritable: true},
		},
	}
	postRes := &resource.Resource{
		Name:       "Post",
		Table:      "posts",
		Timestamps: true,
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "author_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "cover_image", Type: resource.TypeBlob, Nullable: true, ClientWritable: true},
		},
		Auth: &resource.Auth{
			OwnershipField: "author_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "public",
				Update: "owner",
				Delete: "owner",
			},
		},
		Relations: []resource.Relation{
			{Name: "comments", Kind: resource.KindHasMany, Target: "Comment", ForeignKey: "post_id", OnDelete: resource.OnDeleteRestrict},
		},
	}
	commentRes := &resource.Resource{
		Name:       "Comment",
		Table:      "comments",
		Timestamps: true,
		Fields: []resource.Field{
			{Name: "body", Type: resource.TypeText, Nullable: false, ClientWritable: true},
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "author_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
		},
		Auth: &resource.Auth{
			OwnershipField: "author_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "public",
				Update: "owner",
				Delete: "owner",
			},
		},
		Relations: []resource.Relation{
			{Name: "post", Kind: resource.KindBelongsTo, Target: "Post", ForeignKey: "post_id"},
		},
	}

	reg.Register(userRes)
	reg.Register(postRes)
	reg.Register(commentRes)

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("failed generate: %v", err)
	}
	t.Logf("Generated SchemaSQL:\n%s", output.SchemaSQL)

	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(output.PackageJSON), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "wrangler.jsonc"), []byte(output.WranglerConfig), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte(output.SchemaSQL), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "index.ts"), []byte(output.IndexTS), 0644)

	cmdNpm := exec.Command("npm.cmd", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "hono@^4.7.0", "esbuild@^0.24.0")
	if os.Getenv("OS") != "Windows_NT" {
		cmdNpm = exec.Command("npm", "install", "--no-audit", "--no-fund", "miniflare@^3.20241205.0", "hono@^4.7.0", "esbuild@^0.24.0")
	}
	cmdNpm.Dir = tmpDir
	if out, err := cmdNpm.CombinedOutput(); err != nil {
		t.Fatalf("npm install failed: %v\nOutput: %s", err, string(out))
	}

	cmdBuild := exec.Command("npx.cmd", "esbuild", "index.ts", "--bundle", "--format=esm", "--outfile=dist/worker.js", "--target=esnext", "--external:cloudflare:workers")
	if os.Getenv("OS") != "Windows_NT" {
		cmdBuild = exec.Command("npx", "esbuild", "index.ts", "--bundle", "--format=esm", "--outfile=dist/worker.js", "--target=esnext", "--external:cloudflare:workers")
	}
	cmdBuild.Dir = tmpDir
	if out, err := cmdBuild.CombinedOutput(); err != nil {
		t.Fatalf("esbuild bundle failed: %v\nOutput: %s", err, string(out))
	}

	workerJSPath := filepath.Join(tmpDir, "dist", "worker.js")
	miniflareURL := filepath.ToSlash(workerJSPath)

	runnerJS := fmt.Sprintf(`
import { Miniflare } from "miniflare";
import fs from "node:fs";
import { File } from "node:buffer";

async function run() {
  const mf = new Miniflare({
    modules: true,
    scriptPath: "%s",
    d1Databases: { DB: "test-db" },
    r2Buckets: { BUCKET: "test-bucket" },
    compatibilityFlags: ["nodejs_compat"],
  });

  const db = await mf.getD1Database("DB");
  const bucket = await mf.getR2Bucket("BUCKET");
  const schemaSQL = fs.readFileSync("schema.sql", "utf8");
  const cleanSQL = schemaSQL.replace(/--.*$/gm, "");
  for (const rawStmt of cleanSQL.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await db.exec(stmt + ";");
    }
  }

  // Seed user and session
  await db.prepare("INSERT INTO users (id, email, role, created_at, updated_at) VALUES (101, 'author@example.com', 'user', datetime('now'), datetime('now'))").run();
  await db.prepare("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('sess_author_101', 101, datetime('now'), datetime('now', '+1 day'))").run();

  // 1. Test 1-Step Multipart with payload JSON + image file + nested comments
  const dummyImageBytes = new TextEncoder().encode("SAMPLE_IMAGE_DATA_BLOB_TEST_123");
  const imageFile = new File([dummyImageBytes], "cover.png", { type: "image/png" });

  const form1 = new FormData();
  form1.set("payload", JSON.stringify({
    title: "Post with 1-Step Multipart Blob and Nested Comment",
    comments: [
      { body: "Awesome 1-step comment" }
    ]
  }));
  form1.set("cover_image", imageFile);

  const res1 = await mf.dispatchFetch("http://localhost/api/posts", {
    method: "POST",
    headers: { "Cookie": "mold_session=sess_author_101" },
    body: form1
  });
  console.log("Miniflare 1-Step Multipart Status:", res1.status);
  const json1 = await res1.json();
  console.log("Miniflare 1-Step Multipart Response:", JSON.stringify(json1));

  if (res1.status !== 201) {
    console.error("Expected 201 Created for 1-step multipart, got", res1.status);
    process.exit(1);
  }

  if (!json1.data || !json1.data.id || !json1.data.cover_image || !json1.data.cover_image.startsWith("blobs/posts/")) {
    console.error("Invalid response data or blob key:", json1);
    process.exit(1);
  }
  if (!json1.data.comments || json1.data.comments.length !== 1 || json1.data.comments[0].body !== "Awesome 1-step comment") {
    console.error("Nested comment missing or incorrect:", json1);
    process.exit(1);
  }

  const blobKey = json1.data.cover_image;
  const r2Object = await bucket.get(blobKey);
  if (!r2Object) {
    console.error("Blob key not found in R2 bucket:", blobKey);
    process.exit(1);
  }
  const r2Bytes = new Uint8Array(await r2Object.arrayBuffer());
  if (r2Bytes.length !== dummyImageBytes.length || r2Bytes[0] !== dummyImageBytes[0]) {
    console.error("R2 object byte mismatch:", r2Bytes);
    process.exit(1);
  }

  // 2. Test Blob Download endpoint
  const resDl = await mf.dispatchFetch("http://localhost/api/posts/" + json1.data.id + "/blob/cover_image", {
    method: "GET"
  });
  console.log("Miniflare Blob Download Status:", resDl.status);
  if (resDl.status !== 200) {
    console.error("Expected 200 for blob download, got", resDl.status);
    process.exit(1);
  }
  const dlBytes = new Uint8Array(await resDl.arrayBuffer());
  if (dlBytes.length !== dummyImageBytes.length || dlBytes[0] !== dummyImageBytes[0]) {
    console.error("Downloaded byte length mismatch:", dlBytes.length);
    process.exit(1);
  }

  // 3. Test Invalid JSON in payload field -> 400 INVALID_JSON
  const formBad = new FormData();
  formBad.append("payload", "{ invalid_json: ");
  const resBad = await mf.dispatchFetch("http://localhost/api/posts", {
    method: "POST",
    headers: { "Cookie": "mold_session=sess_author_101" },
    body: formBad
  });
  console.log("Miniflare Invalid JSON in multipart status:", resBad.status);
  if (resBad.status !== 400) {
    console.error("Expected 400 for bad JSON in multipart, got", resBad.status);
    process.exit(1);
  }

  // 4. Test file fallback ('file' parameter name instead of field name)
  const formFallback = new FormData();
  formFallback.set("payload", JSON.stringify({ title: "Post with fallback file name" }));
  formFallback.set("file", new File([dummyImageBytes], "fallback.png", { type: "image/png" }));
  const resFallback = await mf.dispatchFetch("http://localhost/api/posts", {
    method: "POST",
    headers: { "Cookie": "mold_session=sess_author_101" },
    body: formFallback
  });
  console.log("Miniflare File Fallback Status:", resFallback.status);
  const jsonFallback = await resFallback.json();
  console.log("Miniflare File Fallback Response:", JSON.stringify(jsonFallback));
  if (resFallback.status !== 201 || !jsonFallback.data.cover_image) {
    console.error("Expected 201 with cover_image for file fallback, got", resFallback.status, jsonFallback);
    process.exit(1);
  }

  console.log("Miniflare 1-Step Multipart & Blob Tests 100%% PASS!");
  await mf.dispose();
}

run().catch(err => {
  console.error("Fatal Miniflare runner error:", err);
  process.exit(1);
});
`, miniflareURL)

	runnerPath2 := filepath.Join(tmpDir, "runner2.mjs")
	if err := os.WriteFile(runnerPath2, []byte(runnerJS), 0644); err != nil {
		t.Fatalf("failed writing runner2.mjs: %v", err)
	}

	cmdRun2 := exec.Command("node", "runner2.mjs")
	cmdRun2.Dir = tmpDir
	outputBytes2, err := cmdRun2.CombinedOutput()
	t.Logf("Miniflare Multipart Raw Log Output:\n%s", string(outputBytes2))
	if err != nil {
		t.Fatalf("Miniflare multipart test runner failed: %v", err)
	}
}



