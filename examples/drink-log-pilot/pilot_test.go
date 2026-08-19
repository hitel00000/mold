package pilot_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hitel00000/mold/runtime"
)

func TestDrinkLogPilot_OAuthSessionE2E(t *testing.T) {
	resDir := filepath.Join("..", "drink-log-pilot")
	dbPath := filepath.Join(t.TempDir(), "pilot_oauth.db")

	cfg := runtime.Config{
		ResourceDir: resDir,
		DBPath:      dbPath,
	}

	app, err := runtime.New(cfg)
	if err != nil {
		t.Fatalf("failed initializing runtime App with pilot resources: %v", err)
	}
	defer app.Close()

	ctx := context.Background()

	// 1. Seed User 501 (OAuth user without password field) via App.CreateRecord
	userRec, err := app.CreateRecord(ctx, "User", map[string]any{
		"provider":         "google",
		"provider_user_id": "g_user_99887766",
		"email":            "oauth_pilot@example.com",
		"role":             "user",
	})
	if err != nil {
		t.Fatalf("failed creating OAuth User record: %v", err)
	}

	userIDRaw, ok := userRec["id"]
	if !ok || userIDRaw == nil {
		t.Fatalf("expected created User record to have id, got: %v", userRec)
	}

	var userID int64
	switch v := userIDRaw.(type) {
	case int64:
		userID = v
	case float64:
		userID = int64(v)
	case int:
		userID = int64(v)
	default:
		t.Fatalf("unexpected id type for User: %T", userIDRaw)
	}

	// 2. Issue session cookie using in-process Escape Hatch: app.IssueSessionForUser
	cookieVal, exp, err := app.IssueSessionForUser(ctx, userID, "user")
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

	// 3. Create SakeRecord owned by User 501
	sakeRec, err := app.CreateRecord(ctx, "SakeRecord", map[string]any{
		"name":          "Kokuryu Daiginjo",
		"consumed_date": "2026-07-27T19:00:00Z",
		"one_line_note": "Sublime velvet texture",
		"owner_id":      fmt.Sprintf("%v", userID),
	})
	if err != nil {
		t.Fatalf("failed creating SakeRecord: %v", err)
	}
	sakeID := sakeRec["id"]

	// 4. Test HTTP GET /api/sake_records/:id (permissions.read: owner) with issued cookie -> 200 OK
	reqOwner, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/sake_records/%v", sakeID), nil)
	reqOwner.Header.Set("Cookie", cookieVal)

	wOwner := httptest.NewRecorder()
	app.ServeHTTP(wOwner, reqOwner)

	if wOwner.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for owner read with issued OAuth cookie, got %d: %s", wOwner.Code, wOwner.Body.String())
	}

	// 5. Test HTTP GET /api/sake_records/:id with another user's cookie -> 403 Forbidden
	cookieValOther, _, err := app.IssueSessionForUser(ctx, 999999, "user")
	if err != nil {
		t.Fatalf("failed issuing session for other user: %v", err)
	}

	reqOther, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/sake_records/%v", sakeID), nil)
	reqOther.Header.Set("Cookie", cookieValOther)

	wOther := httptest.NewRecorder()
	app.ServeHTTP(wOther, reqOther)

	if wOther.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-owner user, got %d: %s", wOther.Code, wOther.Body.String())
	}
}

func TestDrinkLogPilot_NullableOwnerTagPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	tagYamlPath := filepath.Join(tmpDir, "Tag.yaml")
	tagYamlContent := `resource:
  name: Tag
  table: tags
  timestamps: true
  soft_delete: false

fields:
  - name: legacy_id
    type: string
    nullable: true
  - name: slug
    type: string
    nullable: true
  - name: owner_id
    type: string
    nullable: true
  - name: drink_type
    type: string
    nullable: false
    default: "sake"
  - name: tag_group
    type: enum
    nullable: false
    constraints:
      values: ["taste", "aroma", "mood"]
  - name: label
    type: string
    nullable: false
  - name: is_default
    type: bool
    nullable: false
    default: false

auth:
  ownership_field: owner_id
  permissions:
    create: authenticated
    read: owner
    update: owner
    delete: owner
`
	if err := os.WriteFile(tagYamlPath, []byte(tagYamlContent), 0644); err != nil {
		t.Fatalf("failed writing temporary Tag.yaml: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "pilot_nullable_tag.db")
	app, err := runtime.New(runtime.Config{
		ResourceDir: tmpDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed initializing runtime App: %v", err)
	}
	defer app.Close()

	ctx := context.Background()

	// Seed System Default Tag (owner_id = null)
	sysTag, err := app.CreateRecord(ctx, "Tag", map[string]any{
		"tag_group": "taste",
		"label":     "System Fruity",
		"owner_id":  nil,
	})
	if err != nil {
		t.Fatalf("failed creating system tag: %v", err)
	}
	sysTagID := sysTag["id"]

	// Seed User 501 Custom Tag (owner_id = 501)
	userTag, err := app.CreateRecord(ctx, "Tag", map[string]any{
		"tag_group": "taste",
		"label":     "User 501 Custom Tag",
		"owner_id":  "501",
	})
	if err != nil {
		t.Fatalf("failed creating user tag: %v", err)
	}
	userTagID := userTag["id"]

	// Issue cookies for User 501, User 502, and Admin
	user1Cookie, _, _ := app.IssueSessionForUser(ctx, 501, "user")
	user2Cookie, _, _ := app.IssueSessionForUser(ctx, 502, "user")
	adminCookie, _, _ := app.IssueSessionForUser(ctx, 999, "admin")

	// 1. Unauthenticated read of System Tag (owner_id = null) -> 200 OK
	reqUnauthSys, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/tags/%v", sysTagID), nil)
	wUnauthSys := httptest.NewRecorder()
	app.ServeHTTP(wUnauthSys, reqUnauthSys)
	if wUnauthSys.Code != http.StatusOK {
		t.Errorf("expected 200 OK for unauthenticated read of system tag, got %d: %s", wUnauthSys.Code, wUnauthSys.Body.String())
	}

	// 2. User 502 read of System Tag (owner_id = null) -> 200 OK
	reqUser2Sys, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/tags/%v", sysTagID), nil)
	reqUser2Sys.Header.Set("Cookie", user2Cookie)
	wUser2Sys := httptest.NewRecorder()
	app.ServeHTTP(wUser2Sys, reqUser2Sys)
	if wUser2Sys.Code != http.StatusOK {
		t.Errorf("expected 200 OK for user2 read of system tag, got %d: %s", wUser2Sys.Code, wUser2Sys.Body.String())
	}

	// 3. User 502 read of User 501 Custom Tag (owner_id = 501) -> 403 Forbidden
	reqUser2User1Tag, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/tags/%v", userTagID), nil)
	reqUser2User1Tag.Header.Set("Cookie", user2Cookie)
	wUser2User1Tag := httptest.NewRecorder()
	app.ServeHTTP(wUser2User1Tag, reqUser2User1Tag)
	if wUser2User1Tag.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for user2 read of user1 custom tag, got %d: %s", wUser2User1Tag.Code, wUser2User1Tag.Body.String())
	}

	// 4. User 501 read of User 501 Custom Tag -> 200 OK
	reqUser1User1Tag, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/tags/%v", userTagID), nil)
	reqUser1User1Tag.Header.Set("Cookie", user1Cookie)
	wUser1User1Tag := httptest.NewRecorder()
	app.ServeHTTP(wUser1User1Tag, reqUser1User1Tag)
	if wUser1User1Tag.Code != http.StatusOK {
		t.Errorf("expected 200 OK for owner read of user1 custom tag, got %d: %s", wUser1User1Tag.Code, wUser1User1Tag.Body.String())
	}

	// 5. User 501 update of System Tag (owner_id = null) -> 403 Forbidden
	reqUser1UpdateSys, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/api/tags/%v", sysTagID), strings.NewReader(`{"label":"Hacked System Tag"}`))
	reqUser1UpdateSys.Header.Set("Content-Type", "application/json")
	reqUser1UpdateSys.Header.Set("Cookie", user1Cookie)
	wUser1UpdateSys := httptest.NewRecorder()
	app.ServeHTTP(wUser1UpdateSys, reqUser1UpdateSys)
	if wUser1UpdateSys.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-admin update of system tag, got %d: %s", wUser1UpdateSys.Code, wUser1UpdateSys.Body.String())
	}

	// 6. Admin update of System Tag (owner_id = null) -> 200 OK
	reqAdminUpdateSys, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/api/tags/%v", sysTagID), strings.NewReader(`{"label":"Renamed System Tag"}`))
	reqAdminUpdateSys.Header.Set("Content-Type", "application/json")
	reqAdminUpdateSys.Header.Set("Cookie", adminCookie)
	wAdminUpdateSys := httptest.NewRecorder()
	app.ServeHTTP(wAdminUpdateSys, reqAdminUpdateSys)
	if wAdminUpdateSys.Code != http.StatusOK {
		t.Errorf("expected 200 OK for admin update of system tag, got %d: %s", wAdminUpdateSys.Code, wAdminUpdateSys.Body.String())
	}
}

func TestDrinkLogPilot_RelationIncludeE2E(t *testing.T) {
	resDir := filepath.Join("..", "drink-log-pilot")
	dbPath := filepath.Join(t.TempDir(), "pilot_include_e2e.db")

	cfg := runtime.Config{
		ResourceDir: resDir,
		DBPath:      dbPath,
	}

	app, err := runtime.New(cfg)
	if err != nil {
		t.Fatalf("failed initializing runtime App: %v", err)
	}
	defer app.Close()

	ctx := context.Background()

	// 1. Create User, Tag and SakeRecord
	user, err := app.CreateRecord(ctx, "User", map[string]any{
		"provider":         "google",
		"provider_user_id": "g_100",
		"email":            "user100@example.com",
	})
	if err != nil {
		t.Fatalf("failed creating User: %v", err)
	}
	userIDRaw := user["id"]
	var targetUserID int64
	switch v := userIDRaw.(type) {
	case int64:
		targetUserID = v
	case float64:
		targetUserID = int64(v)
	case int:
		targetUserID = int64(v)
	}

	tag, err := app.CreateRecord(ctx, "Tag", map[string]any{
		"tag_group": "taste",
		"label":     "Refresh Fruity",
	})
	if err != nil {
		t.Fatalf("failed creating Tag: %v", err)
	}
	tagID := tag["id"]

	sake, err := app.CreateRecord(ctx, "SakeRecord", map[string]any{
		"name":          "Dassai 23",
		"consumed_date": "2026-07-28T12:00:00Z",
		"one_line_note": "Smooth & aromatic",
		"owner_id":      fmt.Sprintf("%v", targetUserID),
	})
	if err != nil {
		t.Fatalf("failed creating SakeRecord: %v", err)
	}
	sakeID := sake["id"]

	// 2. Create RecordTag Join Resource record connecting SakeRecord and Tag
	recTag, err := app.CreateRecord(ctx, "RecordTag", map[string]any{
		"sake_record_id": sakeID,
		"tag_id":         fmt.Sprintf("%v", tagID),
	})
	if err != nil {
		t.Fatalf("failed creating RecordTag: %v", err)
	}
	recTagID := recTag["id"]

	cookieVal, _, err := app.IssueSessionForUser(ctx, targetUserID, "admin")
	if err != nil {
		t.Fatalf("failed IssueSessionForUser: %v", err)
	}

	// 3. GET /api/record_tags?include=tag,sake_record
	req, _ := http.NewRequest(http.MethodGet, "/api/record_tags?include=tag,sake_record", nil)
	req.Header.Set("Cookie", cookieVal)
	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /api/record_tags?include=tag,sake_record, got %d: %s", w.Code, w.Body.String())
	}

	var listEnv struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &listEnv); err != nil {
		t.Fatalf("failed to decode JSON response: %v", err)
	}

	if len(listEnv.Data) != 1 {
		t.Fatalf("expected 1 record_tag, got %d", len(listEnv.Data))
	}

	row := listEnv.Data[0]
	tagEmbed, ok := row["tag"].(map[string]any)
	if !ok || tagEmbed == nil {
		t.Fatalf("expected embedded tag object, got: %v", row["tag"])
	}
	if tagEmbed["label"] != "Refresh Fruity" {
		t.Errorf("expected tag label 'Refresh Fruity', got %v", tagEmbed["label"])
	}

	sakeEmbed, ok := row["sake_record"].(map[string]any)
	if !ok || sakeEmbed == nil {
		t.Fatalf("expected embedded sake_record object, got: %v", row["sake_record"])
	}
	if sakeEmbed["name"] != "Dassai 23" {
		t.Errorf("expected sake_record name 'Dassai 23', got %v", sakeEmbed["name"])
	}

	// 4. GET /api/record_tags/:id?include=tag
	reqDetail, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/record_tags/%v?include=tag", recTagID), nil)
	reqDetail.Header.Set("Cookie", cookieVal)
	wDetail := httptest.NewRecorder()
	app.ServeHTTP(wDetail, reqDetail)

	if wDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for detail with include, got %d: %s", wDetail.Code, wDetail.Body.String())
	}

	var detailEnv struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(wDetail.Body.Bytes(), &detailEnv); err != nil {
		t.Fatalf("failed to decode detail JSON: %v", err)
	}

	detTag, ok := detailEnv.Data["tag"].(map[string]any)
	if !ok || detTag == nil {
		t.Fatalf("expected detail embedded tag object, got: %v", detailEnv.Data["tag"])
	}
	if detTag["label"] != "Refresh Fruity" {
		t.Errorf("expected detail tag label 'Refresh Fruity', got %v", detTag["label"])
	}

	// 5. Test SSR HTML View GET /view/record_tags?include=tag
	reqViewList, _ := http.NewRequest(http.MethodGet, "/view/record_tags?include=tag", nil)
	reqViewList.Header.Set("Cookie", cookieVal)
	wViewList := httptest.NewRecorder()
	app.ServeHTTP(wViewList, reqViewList)
	if wViewList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for SSR view list with include, got %d: %s", wViewList.Code, wViewList.Body.String())
	}

	// 6. Test SSR HTML View GET /view/record_tags/:id?include=tag
	reqViewDetail, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/view/record_tags/%v?include=tag", recTagID), nil)
	reqViewDetail.Header.Set("Cookie", cookieVal)
	wViewDetail := httptest.NewRecorder()
	app.ServeHTTP(wViewDetail, reqViewDetail)
	if wViewDetail.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for SSR view detail with include, got %d: %s", wViewDetail.Code, wViewDetail.Body.String())
	}
}

// TestDrinkLogPilot_CREATE_OwnershipAutoInjection_HTTP demonstrates that POST /api/sake_records
// auto-assigns owner_id from session user ID (501) and overrides client's forged owner_id (999).
func TestDrinkLogPilot_CREATE_OwnershipAutoInjection_HTTP(t *testing.T) {
	resDir := filepath.Join("..", "drink-log-pilot")
	dbPath := filepath.Join(t.TempDir(), "pilot_autoinject.db")

	app, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed initializing runtime App: %v", err)
	}
	defer app.Close()

	ctx := context.Background()
	userRec, err := app.CreateRecord(ctx, "User", map[string]any{
		"provider":         "google",
		"provider_user_id": "g_user_501",
		"email":            "user501@example.com",
		"role":             "user",
	})
	if err != nil {
		t.Fatalf("failed creating User record: %v", err)
	}

	userID := userRec["id"].(int64)

	cookieVal, _, err := app.IssueSessionForUser(ctx, userID, "user")
	if err != nil {
		t.Fatalf("failed issuing session for user %d: %v", userID, err)
	}

	reqBody := fmt.Sprintf(`{"name":"Dassai 23","consumed_date":"2026-08-04T12:00:00Z","owner_id":999}`)
	req, _ := http.NewRequest(http.MethodPost, "/api/sake_records", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Cookie", cookieVal)

	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	t.Logf("=== DRINK-LOG PILOT RAW HTTP REQUEST (owner_id forgery attempt) ===")
	t.Logf("POST /api/sake_records HTTP/1.1\nCookie: %s\nContent-Type: application/json\n\n%s", cookieVal, reqBody)
	t.Logf("=== DRINK-LOG PILOT RAW HTTP RESPONSE ===")
	t.Logf("HTTP/1.1 %d\nContent-Type: %s\n\n%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	var res struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed decoding JSON: %v", err)
	}

	ownerIDVal, ok := res.Data["owner_id"]
	if !ok || fmt.Sprintf("%v", ownerIDVal) != fmt.Sprintf("%v", userID) {
		t.Errorf("expected owner_id to be auto-injected as %v, got: %v", userID, ownerIDVal)
	}
}

func TestDrinkLogPilot_HasMany_IncludeE2E(t *testing.T) {
	resDir := filepath.Join("..", "drink-log-pilot")
	dbPath := filepath.Join(t.TempDir(), "pilot_has_many.db")

	app, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed initializing runtime App: %v", err)
	}
	defer app.Close()

	ctx := context.Background()

	// 1. Seed User 501
	userRec, err := app.CreateRecord(ctx, "User", map[string]any{
		"provider":         "google",
		"provider_user_id": "g_user_501",
		"email":            "user501@example.com",
		"role":             "user",
	})
	if err != nil {
		t.Fatalf("failed creating User record: %v", err)
	}
	userID := userRec["id"].(int64)

	cookieVal, _, err := app.IssueSessionForUser(ctx, userID, "admin")
	if err != nil {
		t.Fatalf("failed issuing session for user %d: %v", userID, err)
	}

	// 2. Create SakeRecord for User 501
	sakeRec, err := app.CreateRecord(ctx, "SakeRecord", map[string]any{
		"name":          "Dassai 23",
		"consumed_date": "2026-08-19T12:00:00Z",
		"owner_id":      fmt.Sprintf("%v", userID),
	})
	if err != nil {
		t.Fatalf("failed creating SakeRecord: %v", err)
	}
	sakeID := sakeRec["id"]

	// 3. Seed Tag
	tagRec, err := app.CreateRecord(ctx, "Tag", map[string]any{
		"tag_group": "taste",
		"label":     "Fruity",
		"owner_id":  nil,
	})
	if err != nil {
		t.Fatalf("failed creating Tag: %v", err)
	}
	tagID := tagRec["id"]

	// 4. Seed SakeImage and RecordTag pointing to SakeRecord
	imgRec, err := app.CreateRecord(ctx, "SakeImage", map[string]any{
		"record_id":     sakeID,
		"owner_id":      fmt.Sprintf("%v", userID),
		"mime_type":     "image/jpeg",
		"file_name":     "img1.jpg",
		"display_order": 0,
	})
	if err != nil {
		t.Fatalf("failed creating SakeImage: %v", err)
	}
	_ = imgRec

	recTag, err := app.CreateRecord(ctx, "RecordTag", map[string]any{
		"sake_record_id": sakeID,
		"tag_id":         fmt.Sprintf("%v", tagID),
	})
	if err != nil {
		t.Fatalf("failed creating RecordTag: %v", err)
	}
	_ = recTag

	// 5. Test HTTP GET /api/sake_records?include=images,record_tags
	req, _ := http.NewRequest(http.MethodGet, "/api/sake_records?include=images,record_tags", nil)
	req.Header.Set("Cookie", cookieVal)

	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	t.Logf("=== DRINK-LOG PILOT HAS_MANY INCLUDE RESPONSE ===")
	t.Logf("Status: %d\nBody:\n%s", w.Code, w.Body.String())

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", w.Code, w.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed decoding JSON: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Fatalf("expected 1 SakeRecord, got %d", len(resp.Data))
	}

	record := resp.Data[0]
	images, ok := record["images"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("expected images array with 1 item, got: %v", record["images"])
	}
	img0 := images[0].(map[string]any)
	if img0["file_name"] != "img1.jpg" {
		t.Errorf("expected file_name 'img1.jpg', got: %v", img0["file_name"])
	}

	recordTags, ok := record["record_tags"].([]any)
	if !ok || len(recordTags) != 1 {
		t.Fatalf("expected record_tags array with 1 item, got: %v", record["record_tags"])
	}
	recTag0 := recordTags[0].(map[string]any)
	if recTag0["tag_id"] != fmt.Sprintf("%v", tagID) {
		t.Errorf("expected tag_id %v, got: %v", tagID, recTag0["tag_id"])
	}
}



