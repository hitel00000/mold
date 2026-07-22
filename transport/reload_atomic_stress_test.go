package transport_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/transport"
)

func TestTransport_ReloadFailurePreservesExistingIR(t *testing.T) {
	resourceDir := t.TempDir()
	dbDir := t.TempDir()
	dbPath := filepath.Join(dbDir, "test_reload_atomic.db")

	// 1. Write valid initial YAML (Post)
	validPostYAML := `
resource:
  name: Post
  timestamps: true
  soft_delete: true
fields:
  - name: title
    type: string
    nullable: false
`
	postPath := filepath.Join(resourceDir, "Post.yaml")
	if err := os.WriteFile(postPath, []byte(validPostYAML), 0644); err != nil {
		t.Fatalf("failed to write valid Post.yaml: %v", err)
	}

	store, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	// Initial Load
	reg, err := resource.LoadAll(resourceDir)
	if err != nil {
		t.Fatalf("failed initial LoadAll: %v", err)
	}

	ctx := t.Context()
	postRes, _ := reg.Get("Post")
	if err := store.EnsureSchema(ctx, postRes); err != nil {
		t.Fatalf("failed EnsureSchema for Post: %v", err)
	}

	transReg := transport.NewRegistry()
	transReg.Register(postRes, store)

	router := transport.NewRouter(transReg)

	// Set reload function that loads from resourceDir
	router.SetReloadFunc(func() (*transport.Registry, error) {
		newResReg, err := resource.LoadAll(resourceDir)
		if err != nil {
			return nil, err
		}
		newTransReg := transport.NewRegistry()
		for _, r := range newResReg.List() {
			if err := store.EnsureSchema(ctx, r); err != nil {
				return nil, err
			}
			newTransReg.Register(r, store)
		}
		return newTransReg, nil
	})

	ts := httptest.NewServer(router)
	defer ts.Close()

	client := ts.Client()

	// 2. Verify initial GET /api/posts succeeds (200 OK)
	resp, err := client.Get(ts.URL + "/api/posts")
	if err != nil {
		t.Fatalf("failed initial GET /api/posts: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK initially, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// 3. Write a corrupted/invalid YAML into resourceDir (invalid ownership field and invalid permission spec)
	corruptedYAML := `
resource:
  name: BadArticle
fields:
  - name: title
    type: string
auth:
  ownership_field: non_existent_user_id
  permissions:
    update: invalid_perm_spec
`
	badPath := filepath.Join(resourceDir, "BadArticle.yaml")
	if err := os.WriteFile(badPath, []byte(corruptedYAML), 0644); err != nil {
		t.Fatalf("failed to write BadArticle.yaml: %v", err)
	}

	// 4. Trigger Reload API -> expect failure (400 Bad Request or 500 Reload error)
	resp, err = client.Post(ts.URL+"/_mold/reload", "application/json", nil)
	if err != nil {
		t.Fatalf("failed reload HTTP call: %v", err)
	}
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("expected reload to fail for corrupted YAML, but got 200 OK")
	}
	resp.Body.Close()

	// 5. CRUCIAL ASSERTION: Verify original IR (Post) is 100% preserved and GET /api/posts still works with 200 OK
	resp, err = client.Get(ts.URL + "/api/posts")
	if err != nil {
		t.Fatalf("failed post-reload-failure GET /api/posts: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("CRITICAL: expected 200 OK for original /api/posts after reload failure, but got %d (existing IR was lost or corrupted!)", resp.StatusCode)
	}
	resp.Body.Close()
}
