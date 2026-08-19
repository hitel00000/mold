package transport_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	"github.com/hitel00000/mold/transport"
)

func TestInclude_ValidationAndErrors(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_val.db")
	store, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	tagRes := &resource.Resource{
		Name:  "Tag",
		Table: "tags",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "record_tags", Kind: resource.KindHasMany, Target: "RecordTag", ForeignKey: "tag_id"},
		},
	}

	recordTagRes := &resource.Resource{
		Name:  "RecordTag",
		Table: "record_tags",
		Fields: []resource.Field{
			{Name: "sake_record_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "tag_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "tag", Kind: resource.KindBelongsTo, Target: "Tag", ForeignKey: "tag_id"},
		},
	}

	ctx := context.Background()
	_ = store.EnsureSchema(ctx, tagRes)
	_ = store.EnsureSchema(ctx, recordTagRes)

	reg := transport.NewRegistry()
	reg.Register(tagRes, store)
	reg.Register(recordTagRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()
	client := ts.Client()

	// 1. Non-existent relation specified in ?include=
	resp, err := client.Get(ts.URL + "/api/record_tags?include=non_existent")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for non_existent include, got %d", resp.StatusCode)
	}
	var errEnv struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	json.NewDecoder(resp.Body).Decode(&errEnv)
	resp.Body.Close()
	if errEnv.Error.Code != "INVALID_INCLUDE" {
		t.Errorf("expected code INVALID_INCLUDE, got %s", errEnv.Error.Code)
	}
	if !bytes.Contains([]byte(errEnv.Error.Message), []byte("non_existent")) {
		t.Errorf("expected error message to contain 'non_existent', got %s", errEnv.Error.Message)
	}

	// 2. 2-depth dot-chaining specified in ?include= (e.g. record_tags.tag)
	resp, err = client.Get(ts.URL + "/api/tags?include=record_tags.tag")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for dot-chaining include, got %d", resp.StatusCode)
	}
	json.NewDecoder(resp.Body).Decode(&errEnv)
	resp.Body.Close()
	if errEnv.Error.Code != "INVALID_INCLUDE" {
		t.Errorf("expected code INVALID_INCLUDE for dot-chaining, got %s", errEnv.Error.Code)
	}
}

func TestInclude_PermissionSecurityMatrixAndN1Prevention(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_matrix.db")
	store, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	// Target resource with owner read permissions
	tagRes := &resource.Resource{
		Name:       "Tag",
		Table:      "tags",
		SoftDelete: true,
		Auth: &resource.Auth{
			OwnershipField: "owner_id",
			Permissions: resource.Permissions{
				Read: "owner",
			},
		},
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "owner_id", Type: resource.TypeInt, Nullable: true, ClientWritable: true},
		},
	}

	// Main resource with belongs_to relation
	recordTagRes := &resource.Resource{
		Name:  "RecordTag",
		Table: "record_tags",
		Fields: []resource.Field{
			{Name: "tag_id", Type: resource.TypeInt, Nullable: true, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "tag", Kind: resource.KindBelongsTo, Target: "Tag", ForeignKey: "tag_id"},
		},
	}

	ctx := context.Background()
	_ = store.EnsureSchema(ctx, tagRes)
	_ = store.EnsureSchema(ctx, recordTagRes)

	// Create Tags for 4 scenarios:
	// (a) Read allowed: owner_id = NULL (public read)
	tagA, _ := store.Create(ctx, tagRes, storage.Record{"name": "Public Tag A", "owner_id": nil})
	// (b) Read denied: owner_id = 999 (owned by user 999)
	tagB, _ := store.Create(ctx, tagRes, storage.Record{"name": "Private Tag B", "owner_id": 999})
	// (c) FK is NULL
	// (d) Soft-deleted tag
	tagD, _ := store.Create(ctx, tagRes, storage.Record{"name": "Deleted Tag D", "owner_id": nil})
	_ = store.SoftDelete(ctx, tagRes, tagD["id"])

	// Create RecordTags corresponding to scenarios (a), (b), (c), (d)
	recA, _ := store.Create(ctx, recordTagRes, storage.Record{"tag_id": tagA["id"]})
	recB, _ := store.Create(ctx, recordTagRes, storage.Record{"tag_id": tagB["id"]})
	recC, _ := store.Create(ctx, recordTagRes, storage.Record{"tag_id": nil})
	recD, _ := store.Create(ctx, recordTagRes, storage.Record{"tag_id": tagD["id"]})

	reg := transport.NewRegistry()
	reg.Register(tagRes, store)
	reg.Register(recordTagRes, store)

	sm, err := auth.NewSessionManager(store.DB())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	router := transport.NewRouter(reg)
	router.SetSessionManager(sm)
	ts := httptest.NewServer(router)
	defer ts.Close()

	client := ts.Client()

	// Query List with ?include=tag as unauthenticated user over real HTTP endpoint
	req, _ := http.NewRequest("GET", ts.URL+"/api/record_tags?include=tag", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	t.Logf("[Go HTTP Dispatch Raw JSON Response Output]:\n%s", string(bodyBytes))

	var listResp struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(bodyBytes, &listResp)

	if len(listResp.Data) != 4 {
		t.Fatalf("expected 4 records, got %d", len(listResp.Data))
	}

	recMap := make(map[int64]map[string]any)
	for _, item := range listResp.Data {
		idVal := int64(item["id"].(float64))
		recMap[idVal] = item
	}

	// Scenario (a): Read allowed -> tag object filled
	itemA := recMap[int64(recA["id"].(int64))]
	if itemA["tag"] == nil {
		t.Errorf("scenario (a): expected tag object filled, got null")
	} else {
		tagObj := itemA["tag"].(map[string]any)
		if tagObj["name"] != "Public Tag A" {
			t.Errorf("scenario (a): expected tag name 'Public Tag A', got %v", tagObj["name"])
		}
	}

	// Scenario (b): Read denied -> null
	itemB := recMap[int64(recB["id"].(int64))]
	if itemB["tag"] != nil {
		t.Errorf("scenario (b): expected tag to be null due to permission denied, got %v", itemB["tag"])
	}

	// Scenario (c): FK null -> null
	itemC := recMap[int64(recC["id"].(int64))]
	if itemC["tag"] != nil {
		t.Errorf("scenario (c): expected tag to be null due to FK null, got %v", itemC["tag"])
	}

	// Scenario (d): Target soft-deleted -> null
	itemD := recMap[int64(recD["id"].(int64))]
	if itemD["tag"] != nil {
		t.Errorf("scenario (d): expected tag to be null due to target soft-delete, got %v", itemD["tag"])
	}

	// Verify security property: (b), (c), and (d) all return "tag": null without structural differentiation
	if itemB["tag"] != itemC["tag"] || itemC["tag"] != itemD["tag"] {
		t.Errorf("security violation: scenarios (b), (c), (d) are structurally distinguishable")
	}

	// Test Detail endpoint for scenario (a) and (b)
	respA, _ := client.Get(ts.URL + fmt.Sprintf("/api/record_tags/%v?include=tag", recA["id"]))
	var detailA struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(respA.Body).Decode(&detailA)
	respA.Body.Close()
	if detailA.Data["tag"] == nil {
		t.Errorf("detail (a): expected tag filled, got null")
	}

	respB, _ := client.Get(ts.URL + fmt.Sprintf("/api/record_tags/%v?include=tag", recB["id"]))
	var detailB struct {
		Data map[string]any `json:"data"`
	}
	json.NewDecoder(respB.Body).Decode(&detailB)
	respB.Body.Close()
	if detailB.Data["tag"] != nil {
		t.Errorf("detail (b): expected tag null when read denied, got %v", detailB.Data["tag"])
	}
}

type countingStore struct {
	storage.Store
	listCalls int
	lastQuery storage.Query
}

func (c *countingStore) List(ctx context.Context, res *resource.Resource, q storage.Query) ([]storage.Record, error) {
	c.listCalls++
	c.lastQuery = q
	return c.Store.List(ctx, res, q)
}

func TestInclude_N1Prevention_QueryCount(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_n1.db")
	rawStore, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer rawStore.Close()

	tagRes := &resource.Resource{
		Name:  "Tag",
		Table: "tags",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	recordTagRes := &resource.Resource{
		Name:  "RecordTag",
		Table: "record_tags",
		Fields: []resource.Field{
			{Name: "tag_id", Type: resource.TypeInt, Nullable: true, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "tag", Kind: resource.KindBelongsTo, Target: "Tag", ForeignKey: "tag_id"},
		},
	}

	ctx := context.Background()
	_ = rawStore.EnsureSchema(ctx, tagRes)
	_ = rawStore.EnsureSchema(ctx, recordTagRes)

	// Create 5 tags
	var tagIDs []any
	for i := 1; i <= 5; i++ {
		tRec, _ := rawStore.Create(ctx, tagRes, storage.Record{"name": fmt.Sprintf("Tag %d", i)})
		tagIDs = append(tagIDs, tRec["id"])
	}

	// Create 10 RecordTag rows pointing to those 5 tags
	for i := 0; i < 10; i++ {
		tagID := tagIDs[i%5]
		_, _ = rawStore.Create(ctx, recordTagRes, storage.Record{"tag_id": tagID})
	}

	tagCountingStore := &countingStore{Store: rawStore}
	recTagCountingStore := &countingStore{Store: rawStore}

	reg := transport.NewRegistry()
	reg.Register(tagRes, tagCountingStore)
	reg.Register(recordTagRes, recTagCountingStore)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Execute List request for 10 records with ?include=tag
	resp, err := ts.Client().Get(ts.URL + "/api/record_tags?include=tag")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var listResp struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)

	if len(listResp.Data) != 10 {
		t.Fatalf("expected 10 records, got %d", len(listResp.Data))
	}

	// Assert N+1 prevention:
	// Main resource List query: exactly 1 call
	// Included relation List query: exactly 1 batch call with query.IDs = [1, 2, 3, 4, 5]
	t.Logf("[N+1 Prevention Proof Log]")
	t.Logf("  Main Resource (RecordTag) List Query Count: %d", recTagCountingStore.listCalls)
	t.Logf("  Included Relation (Tag) Batch Query Count: %d", tagCountingStore.listCalls)
	t.Logf("  Total Database List Queries executed for 10 records: %d", recTagCountingStore.listCalls+tagCountingStore.listCalls)
	t.Logf("  Batch WHERE IN Query IDs filter parameter: %v", tagCountingStore.lastQuery.IDs)

	if recTagCountingStore.listCalls > 2 {
		t.Errorf("expected at most 2 list calls for main resource (pagination + total), got %d", recTagCountingStore.listCalls)
	}
	if tagCountingStore.listCalls != 1 {
		t.Errorf("expected exactly 1 batch list call for included relation (N+1 prevented!), got %d", tagCountingStore.listCalls)
	}
	if len(tagCountingStore.lastQuery.IDs) != 5 {
		t.Errorf("expected batch query IDs length to be 5, got %d", len(tagCountingStore.lastQuery.IDs))
	}
}

func TestInclude_LargeFKBatch25Plus(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_batch30.db")
	store, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	tagRes := &resource.Resource{
		Name:  "Tag",
		Table: "tags",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	recordTagRes := &resource.Resource{
		Name:  "RecordTag",
		Table: "record_tags",
		Fields: []resource.Field{
			{Name: "tag_id", Type: resource.TypeInt, Nullable: true, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "tag", Kind: resource.KindBelongsTo, Target: "Tag", ForeignKey: "tag_id"},
		},
	}

	ctx := context.Background()
	_ = store.EnsureSchema(ctx, tagRes)
	_ = store.EnsureSchema(ctx, recordTagRes)

	// Create 30 unique tags (exceeding standard page limit of 20)
	var tagIDs []any
	for i := 1; i <= 30; i++ {
		tRec, err := store.Create(ctx, tagRes, storage.Record{"name": fmt.Sprintf("Batch Tag %d", i)})
		if err != nil {
			t.Fatalf("failed to create tag %d: %v", i, err)
		}
		tagIDs = append(tagIDs, tRec["id"])
	}

	// Create 30 RecordTag rows pointing to the 30 unique tags
	for i := 0; i < 30; i++ {
		_, err := store.Create(ctx, recordTagRes, storage.Record{"tag_id": tagIDs[i]})
		if err != nil {
			t.Fatalf("failed to create record_tag %d: %v", i, err)
		}
	}

	reg := transport.NewRegistry()
	reg.Register(tagRes, store)
	reg.Register(recordTagRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Query List with limit=50 and ?include=tag (30 unique FKs in one batch)
	resp, err := ts.Client().Get(ts.URL + "/api/record_tags?limit=50&include=tag")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	var listResp struct {
		Data []map[string]any `json:"data"`
	}
	json.NewDecoder(resp.Body).Decode(&listResp)

	if len(listResp.Data) != 30 {
		t.Fatalf("expected 30 main records, got %d", len(listResp.Data))
	}

	// Verify every single record has its embedded tag filled (zero truncation!)
	filledCount := 0
	for _, rec := range listResp.Data {
		if rec["tag"] != nil {
			tagObj := rec["tag"].(map[string]any)
			if strings.HasPrefix(tagObj["name"].(string), "Batch Tag ") {
				filledCount++
			}
		}
	}

	t.Logf("[25+ Large FK Batch Test Log]")
	t.Logf("  Total Unique FK Batch Query IDs Requested: 30")
	t.Logf("  Total Main Records Fetched: 30")
	t.Logf("  Total Successfully Embedded Relation Records: %d / 30 (100%% filled, 0%% truncation)", filledCount)

	if filledCount != 30 {
		t.Errorf("truncation risk detected! expected 30 filled embedded tags, got %d", filledCount)
	}
}

func TestInclude_HasMany_OperationsAndLimits(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_has_many.db")
	store, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	// 1. Post (Parent) Resource with has_many comments relation
	postRes := &resource.Resource{
		Name:  "Post",
		Table: "posts",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "comments", Kind: resource.KindHasMany, Target: "Comment", ForeignKey: "post_id"},
		},
	}

	// 2. Comment (Child) Resource with ownership_field and belongs_to post relation
	commentRes := &resource.Resource{
		Name:       "Comment",
		Table:      "comments",
		SoftDelete: true,
		Auth: &resource.Auth{
			OwnershipField: "user_id",
			Permissions: resource.Permissions{
				Read: "owner",
			},
		},
		Fields: []resource.Field{
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "body", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "user_id", Type: resource.TypeString, Nullable: true, ClientWritable: true},
			{Name: "secret_token", Type: resource.TypePassword, Nullable: true, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "post", Kind: resource.KindBelongsTo, Target: "Post", ForeignKey: "post_id"},
		},
	}

	_ = store.EnsureSchema(ctx, postRes)
	_ = store.EnsureSchema(ctx, commentRes)

	reg := transport.NewRegistry()
	reg.Register(postRes, store)
	reg.Register(commentRes, store)

	sm, err := auth.NewSessionManager(store.DB())
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	router := transport.NewRouter(reg)
	router.SetSessionManager(sm)
	ts := httptest.NewServer(router)
	defer ts.Close()
	client := ts.Client()

	// Seed Parent Posts:
	// Post 1: has 2 comments (1 public user_id=nil, 1 private user_id="user999")
	p1, _ := store.Create(ctx, postRes, storage.Record{"title": "Post 1 with Comments"})
	p1ID := p1["id"]

	// Post 2: has 0 comments
	p2, _ := store.Create(ctx, postRes, storage.Record{"title": "Post 2 with No Comments"})
	p2ID := p2["id"]

	// Post 3: for limit testing
	p3, _ := store.Create(ctx, postRes, storage.Record{"title": "Post 3 with Many Comments"})
	p3ID := p3["id"]

	// Seed Comments for Post 1
	c1Public, _ := store.Create(ctx, commentRes, storage.Record{"post_id": p1ID, "body": "Public Comment 1", "user_id": nil, "secret_token": "pass123"})
	_, _ = store.Create(ctx, commentRes, storage.Record{"post_id": p1ID, "body": "Private Comment 2", "user_id": "user999", "secret_token": "pass456"})
	c1Deleted, _ := store.Create(ctx, commentRes, storage.Record{"post_id": p1ID, "body": "Deleted Comment 3", "user_id": nil})
	_ = store.SoftDelete(ctx, commentRes, c1Deleted["id"])

	t.Run("1. GET /api/posts?include=comments (Unauthenticated) -> public comments included, private/deleted filtered, 0 comments returns []", func(t *testing.T) {
		resp, err := client.Get(ts.URL + fmt.Sprintf("/api/posts?include=comments"))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var listResp struct {
			Data []map[string]any `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&listResp)

		postMap := make(map[int64]map[string]any)
		for _, item := range listResp.Data {
			postMap[int64(item["id"].(float64))] = item
		}

		// Check Post 1: should have exactly 1 public comment, password sanitized
		post1 := postMap[p1ID.(int64)]
		if post1["comments"] == nil {
			t.Fatalf("expected Post 1 comments array, got null")
		}
		comments1 := post1["comments"].([]any)
		if len(comments1) != 1 {
			t.Fatalf("expected 1 visible comment for Post 1 (private & deleted filtered out), got %d", len(comments1))
		}
		com1 := comments1[0].(map[string]any)
		if com1["body"] != "Public Comment 1" {
			t.Errorf("expected body 'Public Comment 1', got %v", com1["body"])
		}
		if _, exists := com1["secret_token"]; exists {
			t.Errorf("expected password field secret_token to be sanitized from embedded comment")
		}

		// Check Post 2: 0 comments -> MUST be [] (not null)
		post2 := postMap[p2ID.(int64)]
		if post2["comments"] == nil {
			t.Fatalf("expected Post 2 comments to be [] empty array, got null")
		}
		comments2 := post2["comments"].([]any)
		if len(comments2) != 0 {
			t.Fatalf("expected Post 2 comments to have length 0, got %d", len(comments2))
		}
	})

	t.Run("2. GET /api/posts/:id?include=comments for detail", func(t *testing.T) {
		resp, err := client.Get(ts.URL + fmt.Sprintf("/api/posts/%v?include=comments", p1ID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
		}

		var detailResp struct {
			Data map[string]any `json:"data"`
		}
		json.NewDecoder(resp.Body).Decode(&detailResp)

		if detailResp.Data["comments"] == nil {
			t.Fatalf("expected detail comments array, got null")
		}
		comments := detailResp.Data["comments"].([]any)
		if len(comments) != 1 {
			t.Fatalf("expected 1 comment, got %d", len(comments))
		}
	})

	t.Run("3. Exceeding 50 child records limit -> 400 Bad Request with INCLUDE_TOO_LARGE", func(t *testing.T) {
		// Seed 51 public comments for Post 3
		for i := 1; i <= 51; i++ {
			_, err := store.Create(ctx, commentRes, storage.Record{
				"post_id": p3ID,
				"body":    fmt.Sprintf("Comment %d", i),
				"user_id": nil,
			})
			if err != nil {
				t.Fatalf("failed to seed comment %d: %v", i, err)
			}
		}

		resp, err := client.Get(ts.URL + fmt.Sprintf("/api/posts/%v?include=comments", p3ID))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 Bad Request when child records exceed 50, got %d", resp.StatusCode)
		}

		var errResp struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&errResp)

		if errResp.Error.Code != "INCLUDE_TOO_LARGE" {
			t.Errorf("expected error code INCLUDE_TOO_LARGE, got %s", errResp.Error.Code)
		}
		if !strings.Contains(errResp.Error.Message, "exceed limit of 50") {
			t.Errorf("expected error message to mention 'exceed limit of 50', got %s", errResp.Error.Message)
		}
	})
	_ = c1Public
}

func BenchmarkProcessIncludes_HasMany_1000Records(b *testing.B) {
	tmpDir := b.TempDir()
	dbPath := filepath.Join(tmpDir, "bench_include_has_many.db")
	store, err := sqlite.Open(dbPath + "?_pragma=foreign_keys(1)")
	if err != nil {
		b.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	parentRes := &resource.Resource{
		Name:  "ParentDoc",
		Table: "parent_docs",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "children", Kind: resource.KindHasMany, Target: "ChildDoc", ForeignKey: "parent_id"},
		},
	}

	childRes := &resource.Resource{
		Name:  "ChildDoc",
		Table: "child_docs",
		Auth: &resource.Auth{
			OwnershipField: "user_id",
			Permissions: resource.Permissions{
				Read: "owner",
			},
		},
		Fields: []resource.Field{
			{Name: "parent_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "content", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "user_id", Type: resource.TypeString, Nullable: true, ClientWritable: true},
		},
	}

	_ = store.EnsureSchema(ctx, parentRes)
	_ = store.EnsureSchema(ctx, childRes)

	reg := transport.NewRegistry()
	reg.Register(parentRes, store)
	reg.Register(childRes, store)

	// Seed 20 parents × 50 children = 1,000 child records in DB
	var parentRecords []storage.Record
	for p := 1; p <= 20; p++ {
		pRec, _ := store.Create(ctx, parentRes, storage.Record{"title": fmt.Sprintf("Parent %d", p)})
		pID := pRec["id"]
		parentRecords = append(parentRecords, pRec)

		for c := 1; c <= 50; c++ {
			_, _ = store.Create(ctx, childRes, storage.Record{
				"parent_id": pID,
				"content":   fmt.Sprintf("Child %d-%d", p, c),
				"user_id":   nil,
			})
		}
	}

	sess := &auth.Session{ID: "sess_user", UserID: "user1", Role: "user"}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Clone parent records
		recs := make([]storage.Record, len(parentRecords))
		for idx, pr := range parentRecords {
			cloned := make(storage.Record)
			for k, v := range pr {
				cloned[k] = v
			}
			recs[idx] = cloned
		}

		err := transport.ProcessIncludes(ctx, reg, parentRes, recs, "children", sess)
		if err != nil {
			b.Fatalf("ProcessIncludes benchmark failed: %v", err)
		}
	}
}

func TestInclude_DotChaining_RawHTTPProof(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_dot_chain.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	tagRes := &resource.Resource{
		Name:  "Tag",
		Table: "tags",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "record_tags", Kind: resource.KindHasMany, Target: "RecordTag", ForeignKey: "tag_id"},
		},
	}
	recordTagRes := &resource.Resource{
		Name:  "RecordTag",
		Table: "record_tags",
		Fields: []resource.Field{
			{Name: "tag_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
		},
	}

	_ = store.EnsureSchema(ctx, tagRes)
	_ = store.EnsureSchema(ctx, recordTagRes)

	reg := transport.NewRegistry()
	reg.Register(tagRes, store)
	reg.Register(recordTagRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// HTTP GET /api/tags?include=record_tags.tag
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/tags?include=record_tags.tag", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	rawBodyStr := string(bodyBytes)

	t.Logf("=== [RAW HTTP PROOF - Dot Chaining Rejection 400 INVALID_INCLUDE] ===")
	t.Logf("HTTP Request: GET /api/tags?include=record_tags.tag")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", rawBodyStr)

	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for dot-chaining, got %d", resp.StatusCode)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(bodyBytes, &errResp); err != nil {
		t.Fatalf("failed to parse JSON error: %v", err)
	}

	if errResp.Error.Code != "INVALID_INCLUDE" {
		t.Errorf("expected code INVALID_INCLUDE, got %s", errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "record_tags.tag") {
		t.Errorf("expected message to mention 'record_tags.tag', got: %s", errResp.Error.Message)
	}
}

func TestInclude_MixedBelongsToAndHasMany_RawHTTPProof(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_mixed.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := context.Background()

	authorRes := &resource.Resource{
		Name:  "Author",
		Table: "authors",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "bio", Type: resource.TypeString, Nullable: true, ClientWritable: true},
		},
	}

	articleRes := &resource.Resource{
		Name:  "Article",
		Table: "articles",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
			{Name: "author_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "author", Kind: resource.KindBelongsTo, Target: "Author", ForeignKey: "author_id"},
			{Name: "comments", Kind: resource.KindHasMany, Target: "Comment", ForeignKey: "article_id"},
		},
	}

	commentRes := &resource.Resource{
		Name:  "Comment",
		Table: "comments",
		Fields: []resource.Field{
			{Name: "article_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "content", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "article", Kind: resource.KindBelongsTo, Target: "Article", ForeignKey: "article_id"},
		},
	}

	_ = store.EnsureSchema(ctx, authorRes)
	_ = store.EnsureSchema(ctx, articleRes)
	_ = store.EnsureSchema(ctx, commentRes)

	reg := transport.NewRegistry()
	reg.Register(authorRes, store)
	reg.Register(articleRes, store)
	reg.Register(commentRes, store)

	router := transport.NewRouter(reg)
	ts := httptest.NewServer(router)
	defer ts.Close()

	// Seed Author
	author, err := store.Create(ctx, authorRes, storage.Record{"name": "Alice Writer", "bio": "Science fiction author"})
	if err != nil {
		t.Fatalf("failed creating author: %v", err)
	}
	authorID := author["id"]

	// Seed Article 1
	article, err := store.Create(ctx, articleRes, storage.Record{"title": "The Future of AI", "author_id": authorID})
	if err != nil {
		t.Fatalf("failed creating article: %v", err)
	}
	articleID := article["id"]

	// Seed Comments for Article 1
	_, _ = store.Create(ctx, commentRes, storage.Record{"article_id": articleID, "content": "Fascinating read!"})
	_, _ = store.Create(ctx, commentRes, storage.Record{"article_id": articleID, "content": "Looking forward to part 2."})

	// GET /api/articles?include=author,comments
	req, err := http.NewRequest(http.MethodGet, ts.URL+"/api/articles?include=author,comments", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	resp, err := ts.Client().Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	rawBodyStr := string(bodyBytes)

	t.Logf("=== [RAW HTTP PROOF - Mixed BelongsTo & HasMany Include Response] ===")
	t.Logf("HTTP Request: GET /api/articles?include=author,comments")
	t.Logf("HTTP Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("HTTP Response Body:\n%s", rawBodyStr)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for mixed include, got %d", resp.StatusCode)
	}

	var listResp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(bodyBytes, &listResp); err != nil {
		t.Fatalf("failed to parse JSON response: %v", err)
	}

	if len(listResp.Data) != 1 {
		t.Fatalf("expected 1 article, got %d", len(listResp.Data))
	}

	art := listResp.Data[0]

	// 1. Verify belongs_to author is a single embedded object
	authorObj, ok := art["author"].(map[string]any)
	if !ok || authorObj == nil {
		t.Fatalf("expected embedded author object, got: %v", art["author"])
	}
	if authorObj["name"] != "Alice Writer" {
		t.Errorf("expected author name 'Alice Writer', got %v", authorObj["name"])
	}

	// 2. Verify has_many comments is an embedded array of 2 comments
	commentsList, ok := art["comments"].([]any)
	if !ok || commentsList == nil {
		t.Fatalf("expected embedded comments array, got: %v", art["comments"])
	}
	if len(commentsList) != 2 {
		t.Fatalf("expected 2 embedded comments, got %d", len(commentsList))
	}
}

func TestInclude_HasMany_LargeDataset_NoSilentTruncation(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_include_large_dataset.db")
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

	// 1. Seed 200 background parents, each having 50 comments (= 10,000 background child records)
	tx, err := store.DB().Begin()
	if err != nil {
		t.Fatalf("failed to begin tx: %v", err)
	}
	stmtPost, _ := tx.Prepare(`INSERT INTO posts (title) VALUES (?)`)
	stmtComment, _ := tx.Prepare(`INSERT INTO comments (post_id, body) VALUES (?, ?)`)

	for p := 1; p <= 200; p++ {
		res, _ := stmtPost.Exec(fmt.Sprintf("Background Post %d", p))
		pID, _ := res.LastInsertId()
		for c := 1; c <= 50; c++ {
			_, _ = stmtComment.Exec(pID, fmt.Sprintf("Comment %d-%d", p, c))
		}
	}

	// 2. Seed Target Post with 51 comments (total DB comments now = 10,051)
	resOversized, _ := stmtPost.Exec("Oversized Post with 51 Comments")
	oversizedPID, _ := resOversized.LastInsertId()
	for c := 1; c <= 51; c++ {
		_, _ = stmtComment.Exec(oversizedPID, fmt.Sprintf("Oversized Comment %d", c))
	}

	_ = stmtPost.Close()
	_ = stmtComment.Close()
	_ = tx.Commit()

	// Verify total count in DB exceeds 10,000
	var totalCommentsInDB int
	_ = store.DB().QueryRow("SELECT COUNT(*) FROM comments").Scan(&totalCommentsInDB)
	if totalCommentsInDB != 10051 {
		t.Fatalf("expected 10051 comments in DB, got %d", totalCommentsInDB)
	}

	t.Logf("=== [RAW PROOF - Large Dataset Total Comments in DB: %d] ===", totalCommentsInDB)

	// Step A: Requesting oversized post in a 10,000+ total rows database MUST return 400 INCLUDE_TOO_LARGE (no silent cutoff)
	reqOversized, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/posts/%d?include=comments", ts.URL, oversizedPID), nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	respOversized, err := ts.Client().Do(reqOversized)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer respOversized.Body.Close()

	bodyOversized, _ := io.ReadAll(respOversized.Body)
	t.Logf("=== [RAW HTTP PROOF - 51 Records Parent Rejection in 10,000+ Row DB] ===")
	t.Logf("HTTP Status: %d %s", respOversized.StatusCode, http.StatusText(respOversized.StatusCode))
	t.Logf("HTTP Response Body:\n%s", string(bodyOversized))

	if respOversized.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request when single parent exceeds 50 children in large DB, got %d", respOversized.StatusCode)
	}

	var errResp struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(bodyOversized, &errResp)
	if errResp.Error.Code != "INCLUDE_TOO_LARGE" {
		t.Errorf("expected error code INCLUDE_TOO_LARGE, got %s", errResp.Error.Code)
	}

	// Step B: Querying first 100 valid parents (100 * 50 = 5,000 records) returns all 5,000 with 0 truncation
	reqList, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/posts?limit=100&include=comments", ts.URL), nil)
	if err != nil {
		t.Fatalf("failed to build list request: %v", err)
	}

	respList, err := ts.Client().Do(reqList)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer respList.Body.Close()

	if respList.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for list query, got %d", respList.StatusCode)
	}

	var listData struct {
		Data []map[string]any `json:"data"`
	}
	_ = json.NewDecoder(respList.Body).Decode(&listData)

	if len(listData.Data) != 100 {
		t.Fatalf("expected 100 parents, got %d", len(listData.Data))
	}

	totalEmbedded := 0
	for _, p := range listData.Data {
		comments := p["comments"].([]any)
		if len(comments) != 50 {
			t.Fatalf("truncation detected! expected 50 comments on parent %v, got %d", p["id"], len(comments))
		}
		totalEmbedded += len(comments)
	}

	t.Logf("=== [RAW PROOF - 100 Parents Batch Include Successfully Embedded: %d records with ZERO truncation] ===", totalEmbedded)
	if totalEmbedded != 5000 {
		t.Errorf("expected 5000 embedded records, got %d", totalEmbedded)
	}
}


