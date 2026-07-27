package transport_test

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
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

func TestTransport_PaginationTotalCount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_pagination.db")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	postRes := &resource.Resource{
		Name:          "Post",
		Table:         "posts",
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false},
		},
	}

	ctx := t.Context()
	if err := store.EnsureSchema(ctx, postRes); err != nil {
		t.Fatalf("failed to ensure schema: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(postRes, store)
	router := transport.NewRouter(reg)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Insert 25 posts
	for i := 1; i <= 25; i++ {
		_, err := store.Create(ctx, postRes, map[string]any{"title": "Post"})
		if err != nil {
			t.Fatalf("failed to insert post %d: %v", i, err)
		}
	}

	// Query with limit=10
	resp, err := ts.Client().Get(ts.URL + "/api/posts?limit=10")
	if err != nil {
		t.Fatalf("failed to request list: %v", err)
	}
	defer resp.Body.Close()

	var listResp transport.ListSuccessEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
		t.Fatalf("failed to decode list response: %v", err)
	}

	items, ok := listResp.Data.([]any)
	if !ok {
		t.Fatalf("expected data to be []any, got %T", listResp.Data)
	}

	if len(items) != 10 {
		t.Errorf("expected 10 items in page, got %d", len(items))
	}

	if listResp.Meta.Total != 25 {
		t.Errorf("expected meta.total to be 25, got %d", listResp.Meta.Total)
	}

	if listResp.Meta.Limit != 10 {
		t.Errorf("expected meta.limit to be 10, got %d", listResp.Meta.Limit)
	}
}

// TestTransport_MultipartFormGoldenSnapshot captures pre-migration multipart form coercion behavior for derived FKs.
func TestTransport_MultipartFormGoldenSnapshot(t *testing.T) {
	mpRes := &resource.Resource{
		Name:  "Comment",
		Table: "comments",
		Fields: []resource.Field{
			{Name: "body", Type: resource.TypeString, Nullable: false},
		},
		Relations: []resource.Relation{
			{Name: "post", Kind: resource.KindBelongsTo, Target: "Post", ForeignKey: "post_id"},
		},
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "public",
				Read:   "public",
			},
		},
	}
	postRes := &resource.Resource{
		Name:  "Post",
		Table: "posts",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false},
		},
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "public",
				Read:   "public",
			},
		},
	}

	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "mp_test.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed opening store: %v", err)
	}
	defer store.Close()

	_ = store.EnsureSchema(t.Context(), mpRes)
	_ = store.EnsureSchema(t.Context(), postRes)

	trReg := transport.NewRegistry()
	trReg.Register(mpRes, store)
	trReg.Register(postRes, store)

	router := transport.NewRouter(trReg)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Create post first
	_, _ = store.Create(t.Context(), postRes, map[string]any{"title": "P1"})

	// Send multipart/form-data request for Comment
	var buf bytes.Buffer
	bodyWriter := multipart.NewWriter(&buf)
	_ = bodyWriter.WriteField("body", "Great post!")
	_ = bodyWriter.WriteField("post_id", "1")
	_ = bodyWriter.Close()

	req, _ := http.NewRequest("POST", ts.URL+"/api/comments", &buf)
	req.Header.Set("Content-Type", bodyWriter.FormDataContentType())

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("POST /api/comments multipart failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for multipart comment creation, got %d", resp.StatusCode)
	}
}

func TestRouter_UniqueTogether_E2E(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test_unique_together_e2e.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

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

	ctx := t.Context()
	if err := store.EnsureSchema(ctx, recTagRes); err != nil {
		t.Fatalf("failed to ensure schema: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(recTagRes, store)
	router := transport.NewRouter(reg)

	ts := httptest.NewServer(router)
	defer ts.Close()

	payload1 := `{"sake_record_id": 1, "tag_id": 10}`

	// 1. First creation -> 201 Created
	resp1, err := ts.Client().Post(ts.URL+"/api/record_tags", "application/json", strings.NewReader(payload1))
	if err != nil {
		t.Fatalf("failed to send post: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created on first insert, got %d", resp1.StatusCode)
	}

	// 2. Second creation with duplicate combination -> 400 Bad Request
	resp2, err := ts.Client().Post(ts.URL+"/api/record_tags", "application/json", strings.NewReader(payload1))
	if err != nil {
		t.Fatalf("failed to send duplicate post: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on duplicate unique_together insert, got %d", resp2.StatusCode)
	}

	var errEnvelope transport.ErrorEnvelope
	if err := json.NewDecoder(resp2.Body).Decode(&errEnvelope); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errEnvelope.Error.Code != "INVALID_INPUT" {
		t.Errorf("expected error code INVALID_INPUT, got '%s'", errEnvelope.Error.Code)
	}

	// 3. Soft delete record 1 -> DELETE /api/record_tags/1
	reqDel, _ := http.NewRequest("DELETE", ts.URL+"/api/record_tags/1", nil)
	respDel, err := ts.Client().Do(reqDel)
	if err != nil {
		t.Fatalf("failed to send delete request: %v", err)
	}
	defer respDel.Body.Close()
	if respDel.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on soft delete, got %d", respDel.StatusCode)
	}

	// 4. Third creation after soft delete -> 201 Created (Partial Unique Index working)
	resp3, err := ts.Client().Post(ts.URL+"/api/record_tags", "application/json", strings.NewReader(payload1))
	if err != nil {
		t.Fatalf("failed to send post after soft delete: %v", err)
	}
	defer resp3.Body.Close()
	if resp3.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created on insert after soft delete, got %d", resp3.StatusCode)
	}
}
