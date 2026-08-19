package main

import (
	"context"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMoldDev_E2EFileWatchReload(t *testing.T) {
	resourceDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "dev_test.db")

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
		t.Fatalf("failed writing User.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml: %v", err)
	}

	cfg := DevConfig{
		ResourceDir: resourceDir,
		DBPath:      dbPath,
		Addr:        "127.0.0.1:0",
		AdminEmail:  "admin@mold.dev",
		AdminPass:   "adminpassword123",
		DebounceMs:  100,
		PollMs:      50,
	}

	ds, err := NewDevServer(cfg)
	if err != nil {
		t.Fatalf("failed constructing DevServer: %v", err)
	}
	defer ds.Close()

	// Seed Admin User into database before login so Option A login succeeds
	ctx := context.Background()
	_, err = ds.App().CreateRecord(ctx, "User", map[string]any{
		"email":    "admin@mold.dev",
		"password": "adminpassword123",
		"name":     "Admin User",
		"role":     "admin",
	})
	if err != nil {
		t.Fatalf("failed seeding admin user: %v", err)
	}

	// Start Dev Server
	if err := ds.Start(); err != nil {
		t.Fatalf("failed starting DevServer: %v", err)
	}

	baseURL := ds.BaseURL()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// Verify initial endpoint GET /api/posts works
	resp, err := client.Get(baseURL + "/api/posts")
	if err != nil {
		t.Fatalf("initial GET /api/posts failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/posts, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify /api/comments does NOT exist yet (should be 404)
	resp, err = client.Get(baseURL + "/api/comments")
	if err != nil {
		t.Fatalf("GET /api/comments failed: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 Not Found for /api/comments before reload, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Trigger File Change: Write Comment.yaml
	commentYAML := `
resource:
  name: Comment
  timestamps: true
fields:
  - name: content
    type: string
    nullable: false
`
	if err := os.WriteFile(filepath.Join(resourceDir, "Comment.yaml"), []byte(commentYAML), 0644); err != nil {
		t.Fatalf("failed writing Comment.yaml: %v", err)
	}

	// Wait for watcher poll + debounce (50ms poll + 100ms debounce + safety margin)
	time.Sleep(600 * time.Millisecond)

	// Verify /api/comments NOW exists and returns 200 OK (reloaded automatically!)
	resp, err = client.Get(baseURL + "/api/comments")
	if err != nil {
		t.Fatalf("post-reload GET /api/comments failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/comments after dev watcher reload, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// HTML View test post-reload
	resp, err = client.Get(baseURL + "/view/comments")
	if err != nil {
		t.Fatalf("post-reload GET /view/comments failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /view/comments after dev watcher reload, got %d", resp.StatusCode)
	}
	htmlBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if !strings.Contains(string(htmlBytes), "Comment") {
		t.Errorf("expected HTML view to render Comment page, got:\n%s", string(htmlBytes))
	}
}

func TestMoldDev_InvalidYAMLReloadFailureIsolation(t *testing.T) {
	resourceDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "dev_invalid_test.db")

	postYAML := `
resource:
  name: Post
fields:
  - name: title
    type: string
`
	if err := os.WriteFile(filepath.Join(resourceDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml: %v", err)
	}

	cfg := DevConfig{
		ResourceDir: resourceDir,
		DBPath:      dbPath,
		Addr:        "127.0.0.1:0",
		DebounceMs:  100,
		PollMs:      50,
	}

	ds, err := NewDevServer(cfg)
	if err != nil {
		t.Fatalf("failed constructing DevServer: %v", err)
	}
	defer ds.Close()

	if err := ds.Start(); err != nil {
		t.Fatalf("failed starting DevServer: %v", err)
	}

	baseURL := ds.BaseURL()
	client := &http.Client{}

	// Verify /api/posts works initially
	resp, err := client.Get(baseURL + "/api/posts")
	if err != nil {
		t.Fatalf("initial GET /api/posts failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for /api/posts, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Write corrupted invalid YAML
	corruptedYAML := `
resource:
  name: Corrupted
  invalid_yaml_syntax: ::: [unclosed
`
	if err := os.WriteFile(filepath.Join(resourceDir, "Corrupted.yaml"), []byte(corruptedYAML), 0644); err != nil {
		t.Fatalf("failed writing Corrupted.yaml: %v", err)
	}

	// Wait for watcher poll + debounce
	time.Sleep(600 * time.Millisecond)

	// Existing IR must remain active (200 OK for /api/posts)
	resp, err = client.Get(baseURL + "/api/posts")
	if err != nil {
		t.Fatalf("post-corrupted-reload GET /api/posts failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("CRITICAL: expected 200 OK for /api/posts after invalid YAML reload failure, but got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
