package pilot_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
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
	cookieVal, exp, err := app.IssueSessionForUser(ctx, userID)
	if err != nil {
		t.Fatalf("failed IssueSessionForUser: %v", err)
	}
	if !strings.HasPrefix(cookieVal, "_mold_session=") {
		t.Errorf("expected cookie value to start with '_mold_session=', got %s", cookieVal)
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
	cookieValOther, _, err := app.IssueSessionForUser(ctx, 999999)
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
