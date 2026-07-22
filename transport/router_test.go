package transport_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/transport"
)

func TestTransport_E2E(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_transport.db")

	// 1. Open SQLite database with PRAGMA foreign_keys=1
	dsn := dbPath + "?_pragma=foreign_keys(1)"
	store, err := sqlite.Open(dsn)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	// 2. Define IR for Post and Comment
	depSince := 1
	postRes := &resource.Resource{
		Name:          "Post",
		Table:         "posts",
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false},
			{Name: "body", Type: resource.TypeMarkdown, Nullable: false},
			{Name: "legacy_slug", Type: resource.TypeString, Nullable: true, Deprecated: true, DeprecatedSince: &depSince},
		},
	}

	commentRes := &resource.Resource{
		Name:          "Comment",
		Table:         "comments",
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "body", Type: resource.TypeText, Nullable: false},
		},
		Relations: []resource.Relation{
			{
				Name:       "post",
				Kind:       resource.KindBelongsTo,
				Target:     "Post",
				ForeignKey: "post_id",
			},
		},
	}

	ctx := t.Context()
	if err := store.EnsureSchema(ctx, postRes); err != nil {
		t.Fatalf("failed to ensure schema for Post: %v", err)
	}
	if err := store.EnsureSchema(ctx, commentRes); err != nil {
		t.Fatalf("failed to ensure schema for Comment: %v", err)
	}

	// 3. Initialize Registry and Router
	reg := transport.NewRegistry()
	reg.Register(postRes, store)
	reg.Register(commentRes, store)

	router := transport.NewRouter(reg)

	ts := httptest.NewServer(router)
	defer ts.Close()

	client := ts.Client()

	// Scenario A: System Column Rejection on Create
	sysPayload := map[string]any{
		"title":      "Post with system column",
		"body":       "Body content",
		"created_at": "2026-01-01T00:00:00Z",
	}
	sysBody, _ := json.Marshal(sysPayload)
	resp, err := client.Post(ts.URL+"/api/posts", "application/json", bytes.NewReader(sysBody))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for system column in payload, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Scenario B: Successful Post Creation
	postPayload := map[string]any{
		"title": "First Post",
		"body":  "# Hello World",
	}
	postBody, _ := json.Marshal(postPayload)
	resp, err = client.Post(ts.URL+"/api/posts", "application/json", bytes.NewReader(postBody))
	if err != nil {
		t.Fatalf("failed to create post: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for post, got %d", resp.StatusCode)
	}

	var postCreateResp transport.SuccessEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&postCreateResp)
	resp.Body.Close()

	createdPostMap, ok := postCreateResp.Data.(map[string]any)
	if !ok || createdPostMap["id"] == nil {
		t.Fatalf("expected created post to have an id, got %v", postCreateResp.Data)
	}
	postID := createdPostMap["id"]

	// Scenario C: Deprecated field sanitization in List response
	resp, err = client.Get(ts.URL + "/api/posts")
	if err != nil {
		t.Fatalf("failed to list posts: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for list posts, got %d", resp.StatusCode)
	}

	var listResp transport.ListSuccessEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&listResp)
	resp.Body.Close()

	if listResp.Meta.Total != 1 {
		t.Errorf("expected total count 1, got %d", listResp.Meta.Total)
	}
	items, ok := listResp.Data.([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected 1 item in list data, got %v", listResp.Data)
	}
	itemMap := items[0].(map[string]any)
	if _, exists := itemMap["legacy_slug"]; exists {
		t.Errorf("expected deprecated field 'legacy_slug' to be sanitized and absent, but found it in response")
	}

	// Scenario D: Comment Creation with Valid Foreign Key (post_id)
	commentPayload := map[string]any{
		"body":    "Great post!",
		"post_id": postID,
	}
	commBody, _ := json.Marshal(commentPayload)
	resp, err = client.Post(ts.URL+"/api/comments", "application/json", bytes.NewReader(commBody))
	if err != nil {
		t.Fatalf("failed to create comment: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for comment with valid post_id, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Scenario E: Comment Creation with Non-Existent Foreign Key (post_id = 9999) -> Verifying DB FK Error Mapping
	invalidFKPayload := map[string]any{
		"body":    "Orphan comment",
		"post_id": 9999,
	}
	invFKBody, _ := json.Marshal(invalidFKPayload)
	resp, err = client.Post(ts.URL+"/api/comments", "application/json", bytes.NewReader(invFKBody))
	if err != nil {
		t.Fatalf("failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for non-existent foreign key, got %d", resp.StatusCode)
	}

	var errResp transport.ErrorEnvelope
	_ = json.NewDecoder(resp.Body).Decode(&errResp)
	resp.Body.Close()

	if errResp.Error.Code != "INVALID_FOREIGN_KEY" {
		t.Errorf("expected error code 'INVALID_FOREIGN_KEY', got '%s'", errResp.Error.Code)
	}

	// Scenario F: Soft Delete and Detail 404
	req, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/posts/1", nil)
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("failed to delete post: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for delete post, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Verify post is soft-deleted and Detail returns 404
	resp, err = client.Get(ts.URL + "/api/posts/1")
	if err != nil {
		t.Fatalf("failed to fetch deleted post: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for soft-deleted post detail, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Scenario G: Reload API Atomic Pointer Swap
	reloaded := false
	router.SetReloadFunc(func() (*transport.Registry, error) {
		reloaded = true
		newReg := transport.NewRegistry()
		newReg.Register(postRes, store)
		// exclude Comment in reloaded schema for test verification
		return newReg, nil
	})

	resp, err = client.Post(ts.URL+"/_mold/reload", "application/json", nil)
	if err != nil {
		t.Fatalf("failed to trigger reload: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for reload API, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	if !reloaded {
		t.Errorf("expected reload callback function to be called")
	}

	// Verify comments endpoint now returns 404 Not Found after atomic registry swap
	resp, err = client.Get(ts.URL + "/api/comments")
	if err != nil {
		t.Fatalf("failed to query reloaded comments: %v", err)
	}
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected 404 Not Found for comments after reload removed it, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
