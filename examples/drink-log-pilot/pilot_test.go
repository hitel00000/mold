package pilot_test

import (
	"context"
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
		"rating":        5.0,
		"notes":         "Sublime velvet texture",
		"owner_id":      userID,
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
  soft_delete: true

fields:
  - name: name
    type: string
    nullable: false
    constraints:
      unique: true
  - name: owner_id
    type: int
    nullable: true

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

	// Seed System Tag (owner_id = nil) via App.CreateRecord
	sysTag, err := app.CreateRecord(ctx, "Tag", map[string]any{
		"name":     "System Fruity",
		"owner_id": nil,
	})
	if err != nil {
		t.Fatalf("failed creating system tag: %v", err)
	}
	sysTagID := sysTag["id"]

	// Seed User 501 Custom Tag (owner_id = 501)
	userTag, err := app.CreateRecord(ctx, "Tag", map[string]any{
		"name":     "User 501 Custom Tag",
		"owner_id": 501,
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
	reqUser1UpdateSys, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/api/tags/%v", sysTagID), strings.NewReader(`{"name":"Hacked System Tag"}`))
	reqUser1UpdateSys.Header.Set("Content-Type", "application/json")
	reqUser1UpdateSys.Header.Set("Cookie", user1Cookie)
	wUser1UpdateSys := httptest.NewRecorder()
	app.ServeHTTP(wUser1UpdateSys, reqUser1UpdateSys)
	if wUser1UpdateSys.Code != http.StatusForbidden {
		t.Errorf("expected 403 Forbidden for non-admin update of system tag, got %d: %s", wUser1UpdateSys.Code, wUser1UpdateSys.Body.String())
	}

	// 6. Admin update of System Tag (owner_id = null) -> 200 OK
	reqAdminUpdateSys, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("/api/tags/%v", sysTagID), strings.NewReader(`{"name":"Renamed System Tag"}`))
	reqAdminUpdateSys.Header.Set("Content-Type", "application/json")
	reqAdminUpdateSys.Header.Set("Cookie", adminCookie)
	wAdminUpdateSys := httptest.NewRecorder()
	app.ServeHTTP(wAdminUpdateSys, reqAdminUpdateSys)
	if wAdminUpdateSys.Code != http.StatusOK {
		t.Errorf("expected 200 OK for admin update of system tag, got %d: %s", wAdminUpdateSys.Code, wAdminUpdateSys.Body.String())
	}
}

