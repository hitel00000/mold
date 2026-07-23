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

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/transport"
	"github.com/hitel00000/mold/view"
)

func TestMVPSuccessCriteria_E2E(t *testing.T) {
	resourceDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "mvp_success.db")

	// 1. Initial State: Write User.yaml (Auth) & Post.yaml (MVP Success Target)
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

	if err := osWriteFile(filepath.Join(resourceDir, "User.yaml"), []byte(userYAML)); err != nil {
		t.Fatalf("failed to write User.yaml: %v", err)
	}
	if err := osWriteFile(filepath.Join(resourceDir, "Post.yaml"), []byte(postYAML)); err != nil {
		t.Fatalf("failed to write Post.yaml: %v", err)
	}

	// 2. Storage Setup
	store, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := t.Context()

	sm, err := auth.NewSessionManager(store.DB())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	// Helper to build transport registry and view handler
	buildRuntime := func() (*resource.Registry, *transport.Registry, *view.ViewHandler, error) {
		resReg, err := resource.LoadAll(resourceDir)
		if err != nil {
			return nil, nil, nil, err
		}

		transReg := transport.NewRegistry()
		for _, r := range resReg.List() {
			if err := store.EnsureSchema(ctx, r); err != nil {
				return nil, nil, nil, err
			}
			transReg.Register(r, store)
		}

		// Initialize router and view handler
		dummyRouter := transport.NewRouter(transReg)
		dummyRouter.SetSessionManager(sm)
		vh, err := view.NewViewHandler(dummyRouter, nil)
		if err != nil {
			return nil, nil, nil, err
		}

		return resReg, transReg, vh, nil
	}

	resReg, transReg, vh, err := buildRuntime()
	if err != nil {
		t.Fatalf("failed to build runtime: %v", err)
	}

	router := transport.NewRouter(transReg)
	router.SetSessionManager(sm)

	vh, err = view.NewViewHandler(router, nil)
	if err != nil {
		t.Fatalf("failed to create view handler: %v", err)
	}

	router.SetReloadFunc(func() (*transport.Registry, error) {
		_, newTransReg, newVh, err := buildRuntime()
		if err != nil {
			return nil, err
		}
		vh = newVh
		return newTransReg, nil
	})

	mainHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api") || strings.HasPrefix(r.URL.Path, "/_mold") {
			router.ServeHTTP(w, r)
		} else {
			vh.ServeHTTP(w, r)
		}
	})

	ts := httptest.NewServer(mainHandler)
	defer ts.Close()

	jar, _ := cookiejar.New(nil)
	client := ts.Client()
	client.Jar = jar
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}

	// 3. Step A: Seed Admin User & Obtain Session via /login (Auth Integration)
	userRes, _ := resReg.Get("User")
	_, err = store.Create(ctx, userRes, map[string]any{
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
		t.Fatalf("expected 303 SeeOther or 200 OK for login form submit, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 4. Step B: REST API CRUD Verification on Post
	createPostPayload := map[string]any{
		"title": "Mold MVP Launch Post",
		"body":  "# Hello Mold\nWelcome to **Resource-Driven** Runtime!",
	}
	postBody, _ := json.Marshal(createPostPayload)
	resp, err = client.Post(ts.URL+"/api/posts", "application/json", bytes.NewReader(postBody))
	if err != nil {
		t.Fatalf("failed to create post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for Post, got %d", resp.StatusCode)
	}
	var postEnvelope transport.SuccessEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&postEnvelope)
	resp.Body.Close()

	postMap := postEnvelope.Data.(map[string]any)
	postID := postMap["id"]
	if postID == nil {
		t.Fatalf("expected created post to have id, got nil")
	}

	// GET List API
	resp, err = client.Get(ts.URL + "/api/posts")
	if err != nil {
		t.Fatalf("failed GET /api/posts: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /api/posts, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 5. Step C: HTML Default View Verification
	resp, err = client.Get(ts.URL + "/view/posts")
	if err != nil {
		t.Fatalf("failed HTML view GET /view/posts: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for HTML view /view/posts, got %d", resp.StatusCode)
	}
	htmlBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	htmlStr := string(htmlBytes)
	if !strings.Contains(htmlStr, "Mold MVP Launch Post") {
		t.Errorf("expected HTML view to render post title, got:\n%s", htmlStr)
	}

	// HTML Detail View (markdown rendering)
	resp, err = client.Get(ts.URL + "/view/posts/1")
	if err != nil {
		t.Fatalf("failed HTML detail view GET /view/posts/1: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for HTML detail view /view/posts/1, got %d", resp.StatusCode)
	}
	detailHTML, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(detailHTML), "<h1>Hello Mold</h1>") {
		t.Errorf("expected rendered markdown <h1> tag in detail HTML, got:\n%s", string(detailHTML))
	}

	// 6. Step D: AI Workflow Reload (Resource Addition without Go code modification)
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
	if err := osWriteFile(filepath.Join(resourceDir, "Tag.yaml"), []byte(tagYAML)); err != nil {
		t.Fatalf("failed to write Tag.yaml: %v", err)
	}

	// Call POST /_mold/reload using admin session cookie
	resp, err = client.Post(ts.URL+"/_mold/reload", "application/json", nil)
	if err != nil {
		t.Fatalf("failed to trigger reload: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for reload API, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify Tag API and View immediately work!
	createTagPayload := map[string]any{"name": "go"}
	tagBody, _ := json.Marshal(createTagPayload)
	resp, err = client.Post(ts.URL+"/api/tags", "application/json", bytes.NewReader(tagBody))
	if err != nil {
		t.Fatalf("failed POST /api/tags: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for new Tag resource after reload, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Check Tag HTML View
	resp, err = client.Get(ts.URL + "/view/tags")
	if err != nil {
		t.Fatalf("failed HTML view GET /view/tags: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for new Tag HTML view, got %d", resp.StatusCode)
	}
	tagHTMLBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !strings.Contains(string(tagHTMLBytes), "go") {
		t.Errorf("expected Tag HTML view to contain 'go', got:\n%s", string(tagHTMLBytes))
	}
}

func osWriteFile(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}
