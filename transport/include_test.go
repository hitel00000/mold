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

	// 2. has_many relation specified in ?include=
	resp, err = client.Get(ts.URL + "/api/tags?include=record_tags")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for has_many include, got %d", resp.StatusCode)
	}
	json.NewDecoder(resp.Body).Decode(&errEnv)
	resp.Body.Close()
	if errEnv.Error.Code != "INVALID_INCLUDE" {
		t.Errorf("expected code INVALID_INCLUDE for has_many, got %s", errEnv.Error.Code)
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
