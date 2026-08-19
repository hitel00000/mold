package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/adapters/fsblob"
	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	"github.com/hitel00000/mold/transport"
)

func TestNestedWrites_Success(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_success.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

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

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	payload := map[string]any{
		"title": "Nested Post 1",
		"comments": []map[string]any{
			{"body": "Nested Comment 101"},
			{"body": "Nested Comment 102"},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/posts", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/posts: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - Nested Writes Success Response] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	var createdResp struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &createdResp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	parentData := createdResp.Data
	if parentData["title"] != "Nested Post 1" {
		t.Errorf("expected title 'Nested Post 1', got %v", parentData["title"])
	}
	parentID := parentData["id"]
	if parentID == nil {
		t.Fatalf("expected non-nil parent id")
	}

	commentsRaw, ok := parentData["comments"].([]any)
	if !ok || len(commentsRaw) != 2 {
		t.Fatalf("expected 2 embedded comments, got %v", parentData["comments"])
	}

	c1 := commentsRaw[0].(map[string]any)
	if c1["body"] != "Nested Comment 101" {
		t.Errorf("expected body 'Nested Comment 101', got %v", c1["body"])
	}

	c2 := commentsRaw[1].(map[string]any)
	if c2["body"] != "Nested Comment 102" {
		t.Errorf("expected body 'Nested Comment 102', got %v", c2["body"])
	}

	// Verify DB state
	var postCount, commentCount int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM comments").Scan(&commentCount)

	if postCount != 1 || commentCount != 2 {
		t.Errorf("expected 1 post and 2 comments in DB, got %d posts, %d comments", postCount, commentCount)
	}
}

func TestNestedWrites_PreValidationFailure_ZeroDanglingRows(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_preval.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

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
			{Name: "rating", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
		},
	}

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Child has missing non-nullable 'rating'
	payload := map[string]any{
		"title": "Invalid Nested Post",
		"comments": []map[string]any{
			{"body": "Valid body 1", "rating": 5},
			{"body": "Invalid body 2"}, // missing required rating
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/posts", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/posts: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - Pre-validation Failure Response] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	// Verify 0 rows created in DB (clean pre-validation, no parent created)
	var postCount, commentCount int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM comments").Scan(&commentCount)

	if postCount != 0 || commentCount != 0 {
		t.Errorf("expected 0 posts and 0 comments in DB, got %d posts, %d comments", postCount, commentCount)
	}
}

func TestNestedWrites_ChildLimitExceeded_50Max(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_limit.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

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

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// 51 comments exceeds MaxNestedRecordsPerParent (50)
	var comments []map[string]any
	for i := 1; i <= 51; i++ {
		comments = append(comments, map[string]any{"body": fmt.Sprintf("Comment %d", i)})
	}

	payload := map[string]any{
		"title":    "Oversized Comments Post",
		"comments": comments,
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/posts", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/posts: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - Nested Write Limit Exceeded 400 NESTED_WRITE_TOO_LARGE] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(bodyBytes, &errResp)
	if errResp.Error.Code != "NESTED_WRITE_TOO_LARGE" {
		t.Errorf("expected code NESTED_WRITE_TOO_LARGE, got %s", errResp.Error.Code)
	}

	// Verify 0 rows in DB
	var postCount int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	if postCount != 0 {
		t.Errorf("expected 0 posts in DB, got %d", postCount)
	}
}

func TestNestedWrites_ExecutionFailure_CompensatingRollback(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_rollback.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	parentRes := &resource.Resource{
		Name:  "Post",
		Table: "posts",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "tags", Kind: resource.KindHasMany, Target: "PostTag", ForeignKey: "post_id"},
		},
	}

	childRes := &resource.Resource{
		Name:  "PostTag",
		Table: "post_tags",
		Fields: []resource.Field{
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "tag_name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Constraints: &resource.ResourceConstraints{
			UniqueTogether: [][]string{{"tag_name"}},
		},
	}

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	// Pre-seed a tag with name "duplicate_tag"
	_, _ = store.Create(ctx, childRes, storage.Record{"post_id": 999, "tag_name": "duplicate_tag"})

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Child 1 is valid ("tag_one"), Child 2 collides on unique constraint ("duplicate_tag")
	payload := map[string]any{
		"title": "Post with Colliding Tag",
		"tags": []map[string]any{
			{"tag_name": "tag_one"},
			{"tag_name": "duplicate_tag"},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/posts", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/posts: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - Compensating Rollback Error Response] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for collision, got %d", resp.StatusCode)
	}

	// Verify compensating rollback: Parent post and 1st child ("tag_one") MUST be hard-deleted physically
	var postCount, postTagCount int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM post_tags WHERE tag_name = 'tag_one'").Scan(&postTagCount)

	if postCount != 0 {
		t.Errorf("expected 0 posts in DB after rollback, got %d", postCount)
	}
	if postTagCount != 0 {
		t.Errorf("expected tag_one to be physically deleted after rollback, got %d", postTagCount)
	}

	t.Logf("=== [RAW PROOF - Compensating Rollback: 0 Dangling Parent Posts, 0 Dangling Child Tags in DB] ===")
}

func TestNestedWrites_OwnershipAutoInjection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_ownership.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	parentRes := &resource.Resource{
		Name:  "Article",
		Table: "articles",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "owner_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
		},
		Auth: &resource.Auth{
			OwnershipField: "owner_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "authenticated",
			},
		},
		Relations: []resource.Relation{
			{Name: "notes", Kind: resource.KindHasMany, Target: "Note", ForeignKey: "article_id"},
		},
	}

	childRes := &resource.Resource{
		Name:  "Note",
		Table: "notes",
		Fields: []resource.Field{
			{Name: "article_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "content", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "user_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
		},
		Auth: &resource.Auth{
			OwnershipField: "user_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "authenticated",
			},
		},
	}

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	sm, err := auth.NewSessionManager(store.DB())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	router.SetSessionManager(sm)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Create session for user id 42
	sess, err := sm.CreateSession(ctx, 42, "user42", "user")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	payload := map[string]any{
		"title": "Article with Owned Notes",
		"notes": []map[string]any{
			{"content": "Note 1"},
			{"content": "Note 2"},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/articles", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{
		Name:  "_mold_session",
		Value: sess.ID,
	})

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/articles: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - Ownership Auto Injection Response] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	// Verify both parent and child records in DB have owner_id / user_id = 42
	var parentOwnerID int
	_ = store.DB().QueryRow("SELECT owner_id FROM articles LIMIT 1").Scan(&parentOwnerID)
	if parentOwnerID != 42 {
		t.Errorf("expected parent owner_id 42, got %d", parentOwnerID)
	}

	rows, _ := store.DB().Query("SELECT user_id FROM notes")
	defer rows.Close()
	childCount := 0
	for rows.Next() {
		childCount++
		var childUserID int
		_ = rows.Scan(&childUserID)
		if childUserID != 42 {
			t.Errorf("expected child user_id 42, got %d", childUserID)
		}
	}
	if childCount != 2 {
		t.Errorf("expected 2 notes in DB, got %d", childCount)
	}
}

func TestNestedWrites_ClientWritable_Rejection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_cw_rejection.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	parentRes := &resource.Resource{
		Name:  "User",
		Table: "users",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "roles", Kind: resource.KindHasMany, Target: "UserRole", ForeignKey: "user_id"},
		},
	}

	childRes := &resource.Resource{
		Name:  "UserRole",
		Table: "user_roles",
		Fields: []resource.Field{
			{Name: "user_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "role_name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "is_system_admin", Type: resource.TypeBool, Nullable: false, ClientWritable: false}, // ClientWritable: false!
		},
	}

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Client illegally attempts to write is_system_admin: true in nested child payload
	payload := map[string]any{
		"name": "Hacker User",
		"roles": []map[string]any{
			{"role_name": "admin", "is_system_admin": true},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/users", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/users: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - ClientWritable False Rejection 400 CLIENT_WRITE_FORBIDDEN] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(bodyBytes, &errResp)
	if errResp.Error.Code != "CLIENT_WRITE_FORBIDDEN" {
		t.Errorf("expected error code CLIENT_WRITE_FORBIDDEN, got %s", errResp.Error.Code)
	}

	// Verify 0 rows in DB
	var userCount int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM users").Scan(&userCount)
	if userCount != 0 {
		t.Errorf("expected 0 users in DB, got %d", userCount)
	}
}

func TestNestedWrites_ChildPermissionDenied_ZeroDanglingRows(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_child_perm.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	parentRes := &resource.Resource{
		Name:  "Post",
		Table: "posts",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "public",
			},
		},
		Relations: []resource.Relation{
			{Name: "audits", Kind: resource.KindHasMany, Target: "AuditLog", ForeignKey: "post_id"},
		},
	}

	childRes := &resource.Resource{
		Name:  "AuditLog",
		Table: "audit_logs",
		Fields: []resource.Field{
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "action", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "role:admin", // Locked to admin only!
				Read:   "role:admin",
			},
		},
	}

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	sm, err := auth.NewSessionManager(store.DB())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}
	router.SetSessionManager(sm)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Non-admin user (role: "user") tries to nested-write child with create: role:admin
	sess, err := sm.CreateSession(ctx, 101, "normaluser", "user")
	if err != nil {
		t.Fatalf("failed to create session: %v", err)
	}

	payload := map[string]any{
		"title": "Unprivileged Nested Write",
		"audits": []map[string]any{
			{"action": "ILLEGAL_ADMIN_ACTION"},
		},
	}
	payloadBytes, _ := json.Marshal(payload)

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/posts", bytes.NewReader(payloadBytes))
	req.Header.Set("Content-Type", "application/json")
	req.AddCookie(&http.Cookie{
		Name:  "_mold_session",
		Value: sess.ID,
	})

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/posts: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - Child Permission Denied 403 FORBIDDEN Response] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden, got %d", resp.StatusCode)
	}

	// Verify ZERO dangling parent posts and ZERO child audit logs in DB
	var postCount, auditCount int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM audit_logs").Scan(&auditCount)

	if postCount != 0 {
		t.Errorf("expected 0 posts in DB (zero dangling parent), got %d", postCount)
	}
	if auditCount != 0 {
		t.Errorf("expected 0 audit_logs in DB, got %d", auditCount)
	}
	t.Logf("=== [RAW PROOF: 0 Posts, 0 AuditLogs in DB - Zero Dangling Parent/Child on Pre-validation 403] ===")
}

func TestNestedWrites_WithParentBlobUpload(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_nested_writes_blob.db")
	blobDir := filepath.Join(tmpDir, "blobs")
	_ = os.MkdirAll(blobDir, 0755)

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	blobStore, err := fsblob.New(blobDir)
	if err != nil {
		t.Fatalf("failed to create blob store: %v", err)
	}

	ctx := context.Background()

	parentRes := &resource.Resource{
		Name:  "Post",
		Table: "posts",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "cover_image", Type: resource.TypeBlob, Nullable: true, ClientWritable: true},
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

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	router := transport.NewRouter(reg)
	router.SetBlobStore(blobStore)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Build multipart request with title + cover_image file + nested comments JSON
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_ = w.WriteField("title", "Post with Blob and Nested Comments")
	_ = w.WriteField("comments", `[{"body": "First nested comment with blob parent"}, {"body": "Second nested comment"}]`)

	part, err := w.CreateFormFile("cover_image", "cover.jpg")
	if err != nil {
		t.Fatalf("failed creating form file: %v", err)
	}
	_, _ = part.Write([]byte("FAKE_JPEG_IMAGE_DATA_12345"))
	_ = w.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/posts", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("failed POST /api/posts: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	t.Logf("=== [RAW HTTP PROOF - Parent Blob + Nested Writes 201 Created Response] ===")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyBytes))

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var respEnvelope struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(bodyBytes, &respEnvelope)

	// Verify parent blob key attached
	coverKey, ok := respEnvelope.Data["cover_image"].(string)
	if !ok || !strings.HasPrefix(coverKey, "blobs/posts/") {
		t.Errorf("expected cover_image blob key in response, got: %v", respEnvelope.Data["cover_image"])
	}

	// Verify embedded comments attached
	comments, ok := respEnvelope.Data["comments"].([]any)
	if !ok || len(comments) != 2 {
		t.Fatalf("expected comments array with 2 items, got: %v", respEnvelope.Data["comments"])
	}

	// Verify 1 post, 2 comments in DB
	var postCount, commentCount int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM posts").Scan(&postCount)
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM comments").Scan(&commentCount)
	if postCount != 1 {
		t.Errorf("expected 1 post in DB, got %d", postCount)
	}
	if commentCount != 2 {
		t.Errorf("expected 2 comments in DB, got %d", commentCount)
	}
}


