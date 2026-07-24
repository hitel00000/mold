package main_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
)

func TestRuntime_E2E(t *testing.T) {
	resourceDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "runtime_e2e.db")

	// 1. Initial Resources setup: User.yaml & Post.yaml
	userYAML := `
resource:
  name: User
  timestamps: true
  soft_delete: true
fields:
  - name: email
    type: email
    nullable: false
    constraints:
      unique: true
  - name: password
    type: password
    nullable: false
    constraints:
      min_length: 8
  - name: name
    type: string
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
`

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
`

	if err := os.WriteFile(filepath.Join(resourceDir, "User.yaml"), []byte(userYAML), 0644); err != nil {
		t.Fatalf("failed to write User.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed to write Post.yaml: %v", err)
	}

	// -------------------------------------------------------------------------
	// Main Runtime Bootstrap Assembly (Goal: <= 10 lines)
	// Line 1: Define runtime configuration
	cfg := runtime.Config{ResourceDir: resourceDir, DBPath: dbPath}
	// Line 2: Create runtime App container
	app, err := runtime.New(cfg)
	// Line 3: Check bootstrap error
	if err != nil {
		t.Fatalf("failed to build runtime app: %v", err)
	}
	// Line 4: Ensure cleanup
	defer app.Close()
	// Line 5: Start HTTP test server using app as http.Handler
	ts := httptest.NewServer(app)
	// Line 6: Ensure HTTP test server cleanup
	defer ts.Close()
	// -------------------------------------------------------------------------

	ctx := t.Context()
	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// 2. Seed Admin User & Login
	resReg, err := resource.LoadAll(resourceDir)
	if err != nil {
		t.Fatalf("failed to load resources for seeding: %v", err)
	}
	userRes, _ := resReg.Get("User")
	_, err = app.Store().Create(ctx, userRes, map[string]any{
		"email":    "admin@mold.dev",
		"password": "adminpassword123",
		"name":     "Admin User",
		"role":     "admin",
	})
	if err != nil {
		t.Fatalf("failed to seed admin user: %v", err)
	}

	loginForm := url.Values{}
	loginForm.Set("username", "admin@mold.dev")
	loginForm.Set("password", "adminpassword123")

	resp, err := client.PostForm(ts.URL+"/login", loginForm)
	if err != nil {
		t.Fatalf("failed to login: %v", err)
	}
	if resp.StatusCode != http.StatusSeeOther && resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 303 or 200 for login submit, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. REST API CRUD Verification on Post (using runtime.SuccessEnvelope type alias)
	createPostPayload := map[string]any{
		"title": "Runtime Package E2E Post",
		"body":  "# Hello Runtime\nTesting unified runtime package!",
	}
	postBody, _ := json.Marshal(createPostPayload)
	resp, err = client.Post(ts.URL+"/api/posts", "application/json", bytes.NewReader(postBody))
	if err != nil {
		t.Fatalf("failed to create post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}
	var postEnvelope runtime.SuccessEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&postEnvelope)
	resp.Body.Close()

	postMap, ok := postEnvelope.Data.(map[string]any)
	if !ok || postMap["id"] == nil {
		t.Fatalf("expected created post to have valid data, got %v", postEnvelope.Data)
	}

	// 4. HTML View Verification
	resp, err = client.Get(ts.URL + "/view/posts")
	if err != nil {
		t.Fatalf("failed HTML view GET /view/posts: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for HTML view /view/posts, got %d", resp.StatusCode)
	}
	htmlBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(htmlBytes), "Runtime Package E2E Post") {
		t.Errorf("expected HTML view to render title, got:\n%s", string(htmlBytes))
	}

	// HTML Detail View (markdown rendering check)
	resp, err = client.Get(ts.URL + "/view/posts/1")
	if err != nil {
		t.Fatalf("failed HTML detail view GET /view/posts/1: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for detail view, got %d", resp.StatusCode)
	}
	detailHTML, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(detailHTML), "<h1>Hello Runtime</h1>") {
		t.Errorf("expected markdown <h1> tag in detail view, got:\n%s", string(detailHTML))
	}

	// 5. AI Workflow Reload (Dynamic Tag.yaml addition)
	tagYAML := `
resource:
  name: Tag
  timestamps: true
  soft_delete: true
fields:
  - name: name
    type: string
    nullable: false
    constraints:
      unique: true
auth:
  permissions:
    create: authenticated
    read: public
    update: role:admin
    delete: role:admin
`
	if err := os.WriteFile(filepath.Join(resourceDir, "Tag.yaml"), []byte(tagYAML), 0644); err != nil {
		t.Fatalf("failed to write Tag.yaml: %v", err)
	}

	// Call POST /_mold/reload
	resp, err = client.Post(ts.URL+"/_mold/reload", "application/json", nil)
	if err != nil {
		t.Fatalf("failed POST /_mold/reload: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for reload, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify new Tag resource API & View immediately work
	createTagPayload := map[string]any{"name": "golang"}
	tagBody, _ := json.Marshal(createTagPayload)
	resp, err = client.Post(ts.URL+"/api/tags", "application/json", bytes.NewReader(tagBody))
	if err != nil {
		t.Fatalf("failed POST /api/tags: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for Tag after reload, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp, err = client.Get(ts.URL + "/view/tags")
	if err != nil {
		t.Fatalf("failed GET /view/tags: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for Tag HTML view after reload, got %d", resp.StatusCode)
	}
	tagHTMLBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(tagHTMLBytes), "golang") {
		t.Errorf("expected Tag HTML view to contain 'golang', got:\n%s", string(tagHTMLBytes))
	}
}
