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

	runnerJS := fmt.Sprintf(`
import { Miniflare } from "miniflare";
import fs from "node:fs";

async function run() {
  const mf = new Miniflare({
    modules: true,
    scriptPath: "./src/index.ts",
    d1Databases: ["DB"],
    d1Persist: false,
    compatibilityFlags: ["nodejs_compat"]
  });

  const db = await mf.getD1Database("DB");
  const schemaSQL = fs.readFileSync("./schema.sql", "utf8");
  for (const stmt of schemaSQL.split(";").map(s => s.trim()).filter(Boolean)) {
    await db.exec(stmt);
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

  // 3. Test has_many relation: GET /api/tags?include=record_tags -> 400
  const resHasMany = await mf.dispatchFetch("http://localhost/api/tags?include=record_tags");
  if (resHasMany.status !== 400) {
    console.error("Expected 400 for has_many include, got", resHasMany.status);
    process.exit(1);
  }
  const jsonHasMany = await resHasMany.json();
  console.log("Miniflare HasMany Include Response:", JSON.stringify(jsonHasMany, null, 2));

  // 4. Test SSR View GET /view/record_tags?include=tag
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
`)

	if err := os.WriteFile(filepath.Join(tmpDir, "test_runner.mjs"), []byte(runnerJS), 0644); err != nil {
		t.Fatalf("failed writing test_runner.mjs: %v", err)
	}

	cmd := exec.Command("npx", "--package=miniflare", "node", "test_runner.mjs")
	cmd.Dir = tmpDir
	outputBytes, err := cmd.CombinedOutput()
	t.Logf("Miniflare Raw Log Output:\n%s", string(outputBytes))
	if err != nil {
		t.Fatalf("Miniflare test runner failed: %v", err)
	}
}



