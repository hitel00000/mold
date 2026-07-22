package auth_test

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/auth"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/transport"
	"github.com/hitel00000/mold/view"
	_ "modernc.org/sqlite"
)

func TestPermissionMatrix_Coverage(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "matrix.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open raw sqlite db: %v", err)
	}
	defer rawDB.Close()

	sm, err := auth.NewSessionManager(rawDB)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	// 1. Setup Resource definitions with different permissions
	publicRes := &resource.Resource{
		Name:  "PublicDoc",
		Table: "public_docs",
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "public",
				Read:   "public",
				Update: "public",
				Delete: "public",
			},
		},
		Fields: []resource.Field{{Name: "title", Type: resource.TypeString}},
	}

	authenticatedRes := &resource.Resource{
		Name:  "AuthDoc",
		Table: "auth_docs",
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "authenticated",
				Update: "authenticated",
				Delete: "authenticated",
			},
		},
		Fields: []resource.Field{{Name: "title", Type: resource.TypeString}},
	}

	ownerRes := &resource.Resource{
		Name:       "OwnerDoc",
		Table:      "owner_docs",
		SoftDelete: true,
		Auth: &resource.Auth{
			OwnershipField: "user_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "owner",
				Update: "owner",
				Delete: "owner",
			},
		},
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString},
			{Name: "user_id", Type: resource.TypeString},
		},
	}

	roleAdminRes := &resource.Resource{
		Name:  "AdminDoc",
		Table: "admin_docs",
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "role:admin",
				Read:   "role:admin",
				Update: "role:admin",
				Delete: "role:admin",
			},
		},
		Fields: []resource.Field{{Name: "title", Type: resource.TypeString}},
	}

	ctx := t.Context()
	for _, r := range []*resource.Resource{publicRes, authenticatedRes, ownerRes, roleAdminRes} {
		if err := store.EnsureSchema(ctx, r); err != nil {
			t.Fatalf("failed to ensure schema for %s: %v", r.Name, err)
		}
	}

	reg := transport.NewRegistry()
	reg.Register(publicRes, store)
	reg.Register(authenticatedRes, store)
	reg.Register(ownerRes, store)
	reg.Register(roleAdminRes, store)

	router := transport.NewRouter(reg)
	router.SetSessionManager(sm)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Create test sessions
	user1Sess, err := sm.CreateSession(ctx, "user1", "user1", "user")
	if err != nil {
		t.Fatalf("failed to create user1 session: %v", err)
	}

	user2Sess, err := sm.CreateSession(ctx, "user2", "user2", "user")
	if err != nil {
		t.Fatalf("failed to create user2 session: %v", err)
	}

	adminSess, err := sm.CreateSession(ctx, "admin1", "admin1", "admin")
	if err != nil {
		t.Fatalf("failed to create admin session: %v", err)
	}

	// Matrix tests across all 5 endpoints (List, Detail, Create, Update, Delete)
	t.Run("Public Resource - Anonymous Access across all operations", func(t *testing.T) {
		// List
		resp, _ := http.Get(ts.URL + "/api/public_docs")
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for public list, got %d", resp.StatusCode)
		}
		// Create
		cBody, _ := json.Marshal(map[string]any{"title": "Public Doc 1"})
		respC, _ := http.Post(ts.URL+"/api/public_docs", "application/json", bytes.NewBuffer(cBody))
		if respC.StatusCode != http.StatusCreated {
			t.Errorf("expected 201 Created for public create, got %d", respC.StatusCode)
		}
	})

	t.Run("Authenticated Resource - Unauthenticated -> 401 Unauthorized across all operations", func(t *testing.T) {
		// List
		respL, _ := http.Get(ts.URL + "/api/auth_docs")
		if respL.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for unauthenticated list, got %d", respL.StatusCode)
		}
		// Create
		cBody, _ := json.Marshal(map[string]any{"title": "Auth Doc 1"})
		respC, _ := http.Post(ts.URL+"/api/auth_docs", "application/json", bytes.NewBuffer(cBody))
		if respC.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for unauthenticated create, got %d", respC.StatusCode)
		}
	})

	t.Run("Authenticated Resource - Authenticated User -> 200 OK across operations", func(t *testing.T) {
		// List
		reqL, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/auth_docs", nil)
		reqL.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user1Sess.ID})
		respL, _ := ts.Client().Do(reqL)
		if respL.StatusCode != http.StatusOK {
			t.Errorf("expected 200 for authenticated list, got %d", respL.StatusCode)
		}
		// Create
		cBody, _ := json.Marshal(map[string]any{"title": "Auth Doc 1"})
		reqC, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/auth_docs", bytes.NewBuffer(cBody))
		reqC.Header.Set("Content-Type", "application/json")
		reqC.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user1Sess.ID})
		respC, _ := ts.Client().Do(reqC)
		if respC.StatusCode != http.StatusCreated {
			t.Errorf("expected 201 for authenticated create, got %d", respC.StatusCode)
		}
	})

	// Seed OwnerDoc for user1
	rec1, err := store.Create(ctx, ownerRes, map[string]any{"title": "User1 Doc", "user_id": "user1"})
	if err != nil {
		t.Fatalf("failed to seed owner doc: %v", err)
	}
	docID := rec1["id"]

	t.Run("Owner Resource - Unauthenticated -> 401 Unauthorized across all operations", func(t *testing.T) {
		// Detail
		reqD, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), nil)
		respD, _ := ts.Client().Do(reqD)
		if respD.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for unauthenticated detail, got %d", respD.StatusCode)
		}
		// Update
		uBody, _ := json.Marshal(map[string]any{"title": "Hacked Title"})
		reqU, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), bytes.NewBuffer(uBody))
		reqU.Header.Set("Content-Type", "application/json")
		respU, _ := ts.Client().Do(reqU)
		if respU.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for unauthenticated update, got %d", respU.StatusCode)
		}
		// Delete
		reqDel, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), nil)
		respDel, _ := ts.Client().Do(reqDel)
		if respDel.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for unauthenticated delete, got %d", respDel.StatusCode)
		}
	})

	t.Run("Owner Resource - Owner User1 -> 200 OK across Read/Update/Delete", func(t *testing.T) {
		// Detail
		reqD, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), nil)
		reqD.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user1Sess.ID})
		respD, _ := ts.Client().Do(reqD)
		if respD.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for owner user1 detail, got %d", respD.StatusCode)
		}
		// Update
		uBody, _ := json.Marshal(map[string]any{"title": "User1 Updated Title"})
		reqU, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), bytes.NewBuffer(uBody))
		reqU.Header.Set("Content-Type", "application/json")
		reqU.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user1Sess.ID})
		respU, _ := ts.Client().Do(reqU)
		if respU.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for owner user1 update, got %d", respU.StatusCode)
		}
	})

	t.Run("Owner Resource - Non-Owner User2 -> 403 Forbidden across Read/Update/Delete", func(t *testing.T) {
		// Detail
		reqD, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), nil)
		reqD.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respD, _ := ts.Client().Do(reqD)
		if respD.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for non-owner user2 detail, got %d", respD.StatusCode)
		}
		// Update
		uBody, _ := json.Marshal(map[string]any{"title": "Hacked Title User2"})
		reqU, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), bytes.NewBuffer(uBody))
		reqU.Header.Set("Content-Type", "application/json")
		reqU.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respU, _ := ts.Client().Do(reqU)
		if respU.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for non-owner user2 update, got %d", respU.StatusCode)
		}
		// Delete
		reqDel, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), nil)
		reqDel.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respDel, _ := ts.Client().Do(reqDel)
		if respDel.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for non-owner user2 delete, got %d", respDel.StatusCode)
		}
	})

	t.Run("Owner Resource - Non-existent ID -> 404 Not Found before 403 on Update/Delete", func(t *testing.T) {
		// Update non-existent
		uBody, _ := json.Marshal(map[string]any{"title": "NonExistent"})
		reqU, _ := http.NewRequest(http.MethodPut, ts.URL+"/api/owner_docs/9999", bytes.NewBuffer(uBody))
		reqU.Header.Set("Content-Type", "application.json")
		reqU.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respU, _ := ts.Client().Do(reqU)
		if respU.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for non-existent record update, got %d", respU.StatusCode)
		}
		// Delete non-existent
		reqDel, _ := http.NewRequest(http.MethodDelete, ts.URL+"/api/owner_docs/9999", nil)
		reqDel.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respDel, _ := ts.Client().Do(reqDel)
		if respDel.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for non-existent record delete, got %d", respDel.StatusCode)
		}
	})

	t.Run("Owner Resource - Soft-deleted record -> 404 Not Found before 403 on Read/Update/Delete", func(t *testing.T) {
		_ = store.SoftDelete(ctx, ownerRes, docID)

		// Detail
		reqD, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), nil)
		reqD.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respD, _ := ts.Client().Do(reqD)
		if respD.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for soft-deleted record detail, got %d", respD.StatusCode)
		}
		// Update
		uBody, _ := json.Marshal(map[string]any{"title": "Deleted Record Update"})
		reqU, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), bytes.NewBuffer(uBody))
		reqU.Header.Set("Content-Type", "application/json")
		reqU.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respU, _ := ts.Client().Do(reqU)
		if respU.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for soft-deleted record update, got %d", respU.StatusCode)
		}
		// Delete
		reqDel, _ := http.NewRequest(http.MethodDelete, fmt.Sprintf("%s/api/owner_docs/%v", ts.URL, docID), nil)
		reqDel.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user2Sess.ID})
		respDel, _ := ts.Client().Do(reqDel)
		if respDel.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404 Not Found for soft-deleted record delete, got %d", respDel.StatusCode)
		}
	})

	t.Run("Role Admin Resource - User -> 403 Forbidden across Create/Read/Update/Delete", func(t *testing.T) {
		// List
		reqL, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin_docs", nil)
		reqL.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user1Sess.ID})
		respL, _ := ts.Client().Do(reqL)
		if respL.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for non-admin user list, got %d", respL.StatusCode)
		}
		// Create
		cBody, _ := json.Marshal(map[string]any{"title": "Admin Only"})
		reqC, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/admin_docs", bytes.NewBuffer(cBody))
		reqC.Header.Set("Content-Type", "application/json")
		reqC.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: user1Sess.ID})
		respC, _ := ts.Client().Do(reqC)
		if respC.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for non-admin user create, got %d", respC.StatusCode)
		}
	})

	t.Run("Role Admin Resource - Admin -> 200/201 OK across List/Create", func(t *testing.T) {
		// Create
		cBody, _ := json.Marshal(map[string]any{"title": "Admin Created Doc"})
		reqC, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/admin_docs", bytes.NewBuffer(cBody))
		reqC.Header.Set("Content-Type", "application/json")
		reqC.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: adminSess.ID})
		respC, _ := ts.Client().Do(reqC)
		if respC.StatusCode != http.StatusCreated {
			t.Errorf("expected 201 Created for admin create, got %d", respC.StatusCode)
		}
		// List
		reqL, _ := http.NewRequest(http.MethodGet, ts.URL+"/api/admin_docs", nil)
		reqL.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: adminSess.ID})
		respL, _ := ts.Client().Do(reqL)
		if respL.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK for admin list, got %d", respL.StatusCode)
		}
	})
}

func TestUser_Role_PrivilegeEscalation_Protection(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "escalation.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open raw sqlite db: %v", err)
	}
	defer rawDB.Close()

	sm, err := auth.NewSessionManager(rawDB)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	roleValues := []string{"user", "admin"}
	userRes := &resource.Resource{
		Name:  "User",
		Table: "users",
		Auth: &resource.Auth{
			OwnershipField: "id",
			Permissions: resource.Permissions{
				Create: "public",
				Read:   "authenticated",
				Update: "owner",
				Delete: "role:admin",
			},
		},
		Fields: []resource.Field{
			{Name: "username", Type: resource.TypeString, Nullable: false},
			{Name: "email", Type: resource.TypeEmail, Nullable: false},
			{Name: "password", Type: resource.TypePassword, Nullable: false},
			{Name: "role", Type: resource.TypeEnum, Nullable: false, Constraints: resource.Constraints{Values: roleValues}},
		},
	}

	ctx := t.Context()
	if err := store.EnsureSchema(ctx, userRes); err != nil {
		t.Fatalf("failed to ensure schema for User: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(userRes, store)

	router := transport.NewRouter(reg)
	router.SetSessionManager(sm)

	ts := httptest.NewServer(router)
	defer ts.Close()

	// Seed normal user
	hashedPass, _ := auth.HashPassword("secret123")
	userRec, err := store.Create(ctx, userRes, map[string]any{
		"username": "john",
		"email":    "john@example.com",
		"password": hashedPass,
		"role":     "user",
	})
	if err != nil {
		t.Fatalf("failed to seed user: %v", err)
	}
	userID := userRec["id"]

	normalSess, err := sm.CreateSession(ctx, userID, "john", "user")
	if err != nil {
		t.Fatalf("failed to create normal user session: %v", err)
	}

	adminSess, err := sm.CreateSession(ctx, "admin99", "admin99", "admin")
	if err != nil {
		t.Fatalf("failed to create admin session: %v", err)
	}

	client := ts.Client()

	// 1. Normal user attempts to update their own profile AND escalate role to admin -> 403 Forbidden
	escalateBody := map[string]any{
		"email": "john_updated@example.com",
		"role":  "admin",
	}
	bodyBytes, _ := json.Marshal(escalateBody)
	req1, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/users/%v", ts.URL, userID), bytes.NewBuffer(bodyBytes))
	req1.Header.Set("Content-Type", "application.json")
	req1.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: normalSess.ID})

	resp1, err := client.Do(req1)
	if err != nil {
		t.Fatalf("failed to send escalate request: %v", err)
	}
	defer resp1.Body.Close()

	if resp1.StatusCode != http.StatusForbidden {
		t.Errorf("SECURITY RISK: expected 403 Forbidden when normal user attempts role escalation, got %d", resp1.StatusCode)
	}

	// 2. Normal user updates their own profile WITHOUT modifying role -> 200 OK (No over-blocking!)
	normalUpdateBody := map[string]any{
		"email": "john_legit@example.com",
	}
	bodyBytes2, _ := json.Marshal(normalUpdateBody)
	req2, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/users/%v", ts.URL, userID), bytes.NewBuffer(bodyBytes2))
	req2.Header.Set("Content-Type", "application/json")
	req2.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: normalSess.ID})

	resp2, err := client.Do(req2)
	if err != nil {
		t.Fatalf("failed to send normal update request: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK for normal profile update without role field, got %d", resp2.StatusCode)
	}

	// Verify email was updated in DB
	updatedRec, _ := store.Get(ctx, userRes, userID)
	if updatedRec["email"] != "john_legit@example.com" {
		t.Errorf("expected email to be updated to john_legit@example.com, got %v", updatedRec["email"])
	}
	if updatedRec["role"] != "user" {
		t.Errorf("expected role to remain 'user', got %v", updatedRec["role"])
	}

	// 3. Admin user updates normal user's role -> 200 OK
	adminUpdateBody := map[string]any{
		"role": "admin",
	}
	bodyBytes3, _ := json.Marshal(adminUpdateBody)
	req3, _ := http.NewRequest(http.MethodPut, fmt.Sprintf("%s/api/users/%v", ts.URL, userID), bytes.NewBuffer(bodyBytes3))
	req3.Header.Set("Content-Type", "application/json")
	req3.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: adminSess.ID})

	resp3, err := client.Do(req3)
	if err != nil {
		t.Fatalf("failed to send admin update request: %v", err)
	}
	defer resp3.Body.Close()

	if resp3.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK when admin updates user role, got %d", resp3.StatusCode)
	}

	// Verify role was updated to admin by admin user
	updatedByAdmin, _ := store.Get(ctx, userRes, userID)
	if updatedByAdmin["role"] != "admin" {
		t.Errorf("expected role to be updated to 'admin' by admin, got %v", updatedByAdmin["role"])
	}
}

func TestPassword_ValidationAndHashing(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "password.db")

	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("failed to open raw db: %v", err)
	}
	defer rawDB.Close()

	sm, err := auth.NewSessionManager(rawDB)
	if err != nil {
		t.Fatalf("failed to create session manager: %v", err)
	}

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	minLen := 6
	roleValues := []string{"user", "admin"}
	userRes := &resource.Resource{
		Name:  "User",
		Table: "users",
		Auth: &resource.Auth{
			Permissions: resource.Permissions{
				Create: "public",
				Read:   "public",
			},
		},
		Fields: []resource.Field{
			{Name: "username", Type: resource.TypeString, Nullable: false},
			{Name: "password", Type: resource.TypePassword, Nullable: false, Constraints: resource.Constraints{MinLength: &minLen}},
			{Name: "role", Type: resource.TypeEnum, Nullable: false, Constraints: resource.Constraints{Values: roleValues}},
		},
	}

	ctx := t.Context()
	if err := store.EnsureSchema(ctx, userRes); err != nil {
		t.Fatalf("failed to ensure schema: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(userRes, store)

	router := transport.NewRouter(reg)
	router.SetSessionManager(sm)

	ts := httptest.NewServer(router)
	defer ts.Close()

	client := ts.Client()

	adminSess, err := sm.CreateSession(ctx, "admin0", "admin0", "admin")
	if err != nil {
		t.Fatalf("failed to create admin session: %v", err)
	}

	// 1. Submit short password ("12345" < min_length 6) -> 400 Bad Request
	shortPayload := map[string]any{
		"username": "shortpass",
		"password": "12345",
		"role":     "user",
	}
	b1, _ := json.Marshal(shortPayload)
	req1, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/users", bytes.NewBuffer(b1))
	req1.Header.Set("Content-Type", "application.json")
	req1.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: adminSess.ID})
	resp1, _ := client.Do(req1)
	if resp1.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for short password validation, got %d", resp1.StatusCode)
	}

	// 2. Submit password exceeding 72 bytes -> 400 Bad Request
	longPass := string(make([]byte, 80))
	longPayload := map[string]any{
		"username": "longpass",
		"password": longPass,
		"role":     "user",
	}
	b2, _ := json.Marshal(longPayload)
	req2, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/users", bytes.NewBuffer(b2))
	req2.Header.Set("Content-Type", "application.json")
	req2.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: adminSess.ID})
	resp2, _ := client.Do(req2)
	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for >72 byte password, got %d", resp2.StatusCode)
	}

	// 3. Submit valid password -> 201 Created & Verify Password Strip from API response
	validPayload := map[string]any{
		"username": "validpass",
		"password": "securepassword123",
		"role":     "user",
	}
	b3, _ := json.Marshal(validPayload)
	req3, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/users", bytes.NewBuffer(b3))
	req3.Header.Set("Content-Type", "application.json")
	req3.AddCookie(&http.Cookie{Name: auth.SessionCookieName, Value: adminSess.ID})
	resp3, err := client.Do(req3)
	if err != nil || resp3.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created for valid user creation, got %d", resp3.StatusCode)
	}
	defer resp3.Body.Close()

	resBody, _ := io.ReadAll(resp3.Body)
	var respMap map[string]any
	_ = json.Unmarshal(resBody, &respMap)

	dataMap := respMap["data"].(map[string]any)
	if _, passwordExposed := dataMap["password"]; passwordExposed {
		t.Errorf("SECURITY RISK: password field exposed in REST API response!")
	}

	// 4. Verify password stored in DB is bcrypt hashed
	userID := dataMap["id"]
	rawRec, _ := store.Get(ctx, userRes, userID)
	storedPass := fmt.Sprintf("%v", rawRec["password"])
	if !auth.CheckPasswordHash("securepassword123", storedPass) {
		t.Errorf("expected stored password to be bcrypt hash matching 'securepassword123', got %s", storedPass)
	}

	// 5. Test Login E2E via View Handler
	vh, err := view.NewViewHandler(router)
	if err != nil {
		t.Fatalf("failed to create view handler: %v", err)
	}
	tsView := httptest.NewServer(vh)
	defer tsView.Close()

	formVals := url.Values{}
	formVals.Set("username", "validpass")
	formVals.Set("password", "securepassword123")

	clientView := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	loginResp, err := clientView.PostForm(tsView.URL+"/login", formVals)
	if err != nil {
		t.Fatalf("failed to submit login form: %v", err)
	}
	defer loginResp.Body.Close()

	// Verify session cookie set
	cookies := loginResp.Cookies()
	var sessCookie *http.Cookie
	for _, c := range cookies {
		if c.Name == auth.SessionCookieName {
			sessCookie = c
			break
		}
	}
	if sessCookie == nil || sessCookie.Value == "" {
		t.Errorf("expected session cookie %s to be set upon successful login", auth.SessionCookieName)
	}
}
