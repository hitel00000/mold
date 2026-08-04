package runtime_test

import (
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitel00000/mold/runtime"
	"github.com/hitel00000/mold/view"
)

func TestNew_ConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     runtime.Config
		wantErr string
	}{
		{
			name: "missing ResourceDir",
			cfg: runtime.Config{
				ResourceDir: "",
				DBPath:      "test.db",
			},
			wantErr: "Config.ResourceDir is required",
		},
		{
			name: "missing DBPath",
			cfg: runtime.Config{
				ResourceDir: t.TempDir(),
				DBPath:      "",
			},
			wantErr: "Config.DBPath is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := runtime.New(tt.cfg)
			if err == nil {
				if app != nil {
					_ = app.Close()
				}
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
		})
	}
}

func TestNew_InvalidResourceDirOrYAML(t *testing.T) {
	t.Run("non-existent resource dir", func(t *testing.T) {
		cfg := runtime.Config{
			ResourceDir: filepath.Join(t.TempDir(), "non_existent"),
			DBPath:      filepath.Join(t.TempDir(), "test.db"),
		}
		app, err := runtime.New(cfg)
		if err == nil {
			_ = app.Close()
			t.Fatal("expected error for non-existent resource dir, got nil")
		}
	})

	t.Run("invalid YAML in resource dir", func(t *testing.T) {
		resDir := t.TempDir()
		invalidYAML := `
resource:
  name: Invalid
fields:
  - name: title
    type: invalid_type
`
		if err := os.WriteFile(filepath.Join(resDir, "Invalid.yaml"), []byte(invalidYAML), 0644); err != nil {
			t.Fatalf("failed to write invalid YAML: %v", err)
		}

		cfg := runtime.Config{
			ResourceDir: resDir,
			DBPath:      filepath.Join(t.TempDir(), "test.db"),
		}
		app, err := runtime.New(cfg)
		if err == nil {
			_ = app.Close()
			t.Fatal("expected error for invalid YAML resource, got nil")
		}
	})
}

func TestNew_SuccessPaths(t *testing.T) {
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
	resDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(resDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed to write Post.yaml: %v", err)
	}

	t.Run("success without BlobDir and Overrides", func(t *testing.T) {
		cfg := runtime.Config{
			ResourceDir: resDir,
			DBPath:      filepath.Join(t.TempDir(), "app.db"),
		}
		app, err := runtime.New(cfg)
		if err != nil {
			t.Fatalf("expected New() success, got: %v", err)
		}
		if app.Store() == nil {
			t.Error("expected store to be non-nil")
		}
		if err := app.Close(); err != nil {
			t.Errorf("failed to close app: %v", err)
		}
	})

	t.Run("success with BlobDir and Overrides", func(t *testing.T) {
		blobDir := t.TempDir()
		overrides := view.NewTemplateOverrides()
		cfg := runtime.Config{
			ResourceDir: resDir,
			DBPath:      filepath.Join(t.TempDir(), "app_blob.db"),
			BlobDir:     blobDir,
			Overrides:   overrides,
		}
		app, err := runtime.New(cfg)
		if err != nil {
			t.Fatalf("expected New() success, got: %v", err)
		}
		if err := app.Close(); err != nil {
			t.Errorf("failed to close app: %v", err)
		}
	})
}

func TestApp_CreateRecord(t *testing.T) {
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
	resDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(resDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed to write Post.yaml: %v", err)
	}

	cfg := runtime.Config{
		ResourceDir: resDir,
		DBPath:      filepath.Join(t.TempDir(), "createrecord.db"),
	}
	app, err := runtime.New(cfg)
	if err != nil {
		t.Fatalf("failed to build runtime app: %v", err)
	}
	defer app.Close()

	ctx := t.Context()

	t.Run("success via resource name", func(t *testing.T) {
		record, err := app.CreateRecord(ctx, "Post", map[string]any{
			"title": "First Post",
			"body":  "Content of first post",
		})
		if err != nil {
			t.Fatalf("expected CreateRecord success, got error: %v", err)
		}
		if record["id"] == nil {
			t.Error("expected created record to have id")
		}
		if record["title"] != "First Post" {
			t.Errorf("expected title 'First Post', got %v", record["title"])
		}
	})

	t.Run("table name fallback rejected under single resource name contract", func(t *testing.T) {
		_, err := app.CreateRecord(ctx, "posts", map[string]any{
			"title": "Second Post",
			"body":  "Content of second post",
		})
		if err == nil {
			t.Fatal("expected error when passing table name 'posts' instead of resource name 'Post', got nil")
		}
		if !errors.Is(err, runtime.ErrResourceNotFound) {
			t.Errorf("expected errors.Is ErrResourceNotFound, got: %v", err)
		}
	})

	t.Run("non-existent resource error path (errors.Is sentinel check)", func(t *testing.T) {
		_, err := app.CreateRecord(ctx, "NonExistent", map[string]any{
			"name": "foo",
		})
		if err == nil {
			t.Fatal("expected error for non-existent resource, got nil")
		}
		if !errors.Is(err, runtime.ErrResourceNotFound) {
			t.Errorf("expected errors.Is ErrResourceNotFound, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"NonExistent"`) {
			t.Errorf("expected error message to contain resource name %q, got: %v", "NonExistent", err)
		}
	})

	t.Run("record validation failure path (not ErrResourceNotFound)", func(t *testing.T) {
		// Missing non-nullable "body" field
		_, err := app.CreateRecord(ctx, "Post", map[string]any{
			"title": "Incomplete Post",
		})
		if err == nil {
			t.Fatal("expected validation error for missing body, got nil")
		}
		if errors.Is(err, runtime.ErrResourceNotFound) {
			t.Errorf("expected data validation error, not ErrResourceNotFound, got: %v", err)
		}
	})
}

func TestApp_IssueSessionForUser(t *testing.T) {
	docYAML := `
resource:
  name: Document
  table: documents
fields:
  - name: title
    type: string
  - name: user_id
    type: int
auth:
  ownership_field: user_id
  permissions:
    create: authenticated
    read: owner
    update: owner
    delete: owner
`
	resDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(resDir, "Document.yaml"), []byte(docYAML), 0644); err != nil {
		t.Fatalf("failed writing Document.yaml: %v", err)
	}

	cfg := runtime.Config{
		ResourceDir: resDir,
		DBPath:      filepath.Join(t.TempDir(), "oauth_session.db"),
	}
	app, err := runtime.New(cfg)
	if err != nil {
		t.Fatalf("failed building app: %v", err)
	}
	defer app.Close()

	ctx := t.Context()

	// 1. Issue session cookie for user_id = 999
	cookieVal, exp, err := app.IssueSessionForUser(ctx, 999, "user")
	if err != nil {
		t.Fatalf("failed IssueSessionForUser: %v", err)
	}
	if !strings.HasPrefix(cookieVal, "_mold_session=") {
		t.Errorf("expected cookie value to start with '_mold_session=', got %s", cookieVal)
	}
	if !strings.Contains(cookieVal, "Secure") || !strings.Contains(cookieVal, "Expires=") || !strings.Contains(cookieVal, "Max-Age=") {
		t.Errorf("expected cookie value to contain Secure, Expires, and Max-Age attributes, got %s", cookieVal)
	}
	if exp.Before(time.Now()) {
		t.Errorf("invalid expiration time: %v", exp)
	}

	// 2. Create document owned by user_id = 999
	doc, err := app.CreateRecord(ctx, "Document", map[string]any{
		"title":   "OAuth Protected Doc",
		"user_id": 999,
	})
	if err != nil {
		t.Fatalf("failed CreateRecord: %v", err)
	}
	docID := doc["id"]

	// 3. Test HTTP request with issued session cookie -> 200 OK (Owner Access)
	req, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/documents/%v", docID), nil)
	req.Header.Set("Cookie", cookieVal)

	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for owner read with issued session cookie, got %d: %s", w.Code, w.Body.String())
	}

	// 4. Test HTTP request for another user_id = 888 session -> 403 Forbidden
	cookieVal888, _, err := app.IssueSessionForUser(ctx, 888, "user")
	if err != nil {
		t.Fatalf("failed issuing session for 888: %v", err)
	}

	req403, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/documents/%v", docID), nil)
	req403.Header.Set("Cookie", cookieVal888)

	w403 := httptest.NewRecorder()
	app.ServeHTTP(w403, req403)

	if w403.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-owner user, got %d: %s", w403.Code, w403.Body.String())
	}
}

func TestApp_SessionUser(t *testing.T) {
	docYAML := `
resource:
  name: Post
  table: posts
fields:
  - name: title
    type: string
  - name: author_id
    type: int
auth:
  ownership_field: author_id
  permissions:
    create: authenticated
    read: public
    update: owner
    delete: owner
`
	resDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(resDir, "Post.yaml"), []byte(docYAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml: %v", err)
	}

	cfg := runtime.Config{
		ResourceDir: resDir,
		DBPath:      filepath.Join(t.TempDir(), "session_user_test.db"),
	}
	app, err := runtime.New(cfg)
	if err != nil {
		t.Fatalf("failed building app: %v", err)
	}
	defer app.Close()

	ctx := t.Context()

	// 1. Issue session cookie for user 501 with role "user"
	cookieVal501, _, err := app.IssueSessionForUser(ctx, 501, "user")
	if err != nil {
		t.Fatalf("failed IssueSessionForUser: %v", err)
	}

	// 2. Issue session cookie for admin user 999 with role "admin"
	cookieVal999, _, err := app.IssueSessionForUser(ctx, 999, "admin")
	if err != nil {
		t.Fatalf("failed IssueSessionForUser for admin: %v", err)
	}

	t.Run("success path with valid session cookie (user 501)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/posts", nil)
		req.Header.Set("Cookie", cookieVal501)

		// ServeHTTP to verify normal HTTP handling
		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)

		userID, role, ok := app.SessionUser(req)
		if !ok {
			t.Fatalf("expected SessionUser ok == true for valid session cookie, got false")
		}
		if userID != 501 {
			t.Errorf("expected userID 501, got %d", userID)
		}
		if role != "user" {
			t.Errorf("expected role 'user', got %s", role)
		}
	})

	t.Run("success path with valid admin session cookie (admin 999)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodPost, "/posts/create", strings.NewReader(`{"title":"Glue Post"}`))
		req.Header.Set("Cookie", cookieVal999)

		userID, role, ok := app.SessionUser(req)
		if !ok {
			t.Fatalf("expected SessionUser ok == true for admin session, got false")
		}
		if userID != 999 {
			t.Errorf("expected userID 999, got %d", userID)
		}
		if role != "admin" {
			t.Errorf("expected role 'admin', got %s", role)
		}
	})

	t.Run("unauthenticated request (no cookie)", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/posts", nil)

		userID, role, ok := app.SessionUser(req)
		if ok {
			t.Errorf("expected ok == false for unauthenticated request, got true (userID: %d, role: %s)", userID, role)
		}
		if userID != 0 || role != "" {
			t.Errorf("expected zero values for unauthenticated request, got userID=%d, role=%s", userID, role)
		}
	})

	t.Run("invalid / malformed session cookie", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, "/api/posts", nil)
		req.Header.Set("Cookie", "_mold_session=invalid_token_9999999999")

		userID, role, ok := app.SessionUser(req)
		if ok {
			t.Errorf("expected ok == false for invalid session cookie, got true (userID: %d, role: %s)", userID, role)
		}
	})

	t.Run("nil request or app handles gracefully", func(t *testing.T) {
		userID, role, ok := app.SessionUser(nil)
		if ok || userID != 0 || role != "" {
			t.Errorf("expected SessionUser(nil) to return (0, '', false)")
		}

		dummyReq, _ := http.NewRequest(http.MethodGet, "/", nil)
		var nilApp *runtime.App
		userID, role, ok = nilApp.SessionUser(dummyReq)
		if ok || userID != 0 || role != "" {
			t.Errorf("expected nilApp.SessionUser to return (0, '', false)")
		}
	})
}

