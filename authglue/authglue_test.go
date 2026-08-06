package authglue_test

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

	"github.com/hitel00000/mold/authglue"
	"github.com/hitel00000/mold/runtime"
)

func setupTestApp(t *testing.T) (*runtime.App, string) {
	t.Helper()
	tmpDir := t.TempDir()
	resDir := filepath.Join(tmpDir, "resources")
	if err := os.MkdirAll(resDir, 0755); err != nil {
		t.Fatalf("failed to create temp resDir: %v", err)
	}

	userYamlContent := `resource:
  name: User
  table: users
  timestamps: true
  soft_delete: true

fields:
  - name: email
    type: email
    nullable: true
    constraints:
      unique: true
  - name: password
    type: password
    nullable: true
    constraints:
      min_length: 8
  - name: name
    type: string
    nullable: true
  - name: provider
    type: string
    nullable: true
  - name: provider_user_id
    type: string
    nullable: true
  - name: role
    type: enum
    nullable: false
    default: "user"
    client_writable: false
    constraints:
      values: ["admin", "user"]

constraints:
  unique_together:
    - [provider, provider_user_id]

auth:
  ownership_field: id
  permissions:
    create: public
    read: authenticated
    update: owner
    delete: role:admin
`
	postYamlContent := `resource:
  name: Post
  table: posts
  timestamps: true
  soft_delete: true

fields:
  - name: title
    type: string
    nullable: false
  - name: body
    type: markdown
    nullable: false
  - name: author_id
    type: int
    nullable: true

auth:
  ownership_field: author_id
  permissions:
    create: authenticated
    read: owner
    update: owner
    delete: owner
`

	if err := os.WriteFile(filepath.Join(resDir, "User.yaml"), []byte(userYamlContent), 0644); err != nil {
		t.Fatalf("failed writing User.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resDir, "Post.yaml"), []byte(postYamlContent), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "authglue_test.db")
	app, err := runtime.New(runtime.Config{
		ResourceDir: resDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed initializing runtime App: %v", err)
	}
	t.Cleanup(func() {
		app.Close()
	})

	return app, dbPath
}

func setupTestServer(app *runtime.App, mockVerifier authglue.OAuthVerifier) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/signup", authglue.SignupHandler(app))
	if mockVerifier == nil {
		mockVerifier = authglue.UnsafeTestStubOAuthVerifier("google")
	}
	mux.HandleFunc("/auth/google/callback", authglue.OAuthCallbackHandler(app, "google", mockVerifier))
	mux.Handle("/", app)
	return mux
}

// TestAuthGlue_CookieNameEmpiricalSpike verifies empirical session cookie name output.
func TestAuthGlue_CookieNameEmpiricalSpike(t *testing.T) {
	app, _ := setupTestApp(t)
	ctx := context.Background()

	userRec, err := app.CreateRecord(ctx, "User", map[string]any{
		"email": "spike@example.com",
		"name":  "Spike User",
	})
	if err != nil {
		t.Fatalf("failed creating user record: %v", err)
	}

	userID := userRec["id"].(int64)
	cookieVal, exp, err := app.IssueSessionForUser(ctx, userID, "user")
	if err != nil {
		t.Fatalf("IssueSessionForUser failed: %v", err)
	}

	t.Logf("=== EMPIRICAL SESSION COOKIE VALUE MEASUREMENT ===")
	t.Logf("Cookie String: %s", cookieVal)
	t.Logf("Expiration: %v", exp)

	if !strings.HasPrefix(cookieVal, "_mold_session=") {
		t.Errorf("expected cookie value to start with '_mold_session=', got: %s", cookieVal)
	}
}

// TestAuthGlue_SignupSuccessAndSanitize verifies normal signup creates user, issues _mold_session cookie,
// and sanitizes password hash from JSON response.
func TestAuthGlue_SignupSuccessAndSanitize(t *testing.T) {
	app, _ := setupTestApp(t)
	mux := setupTestServer(app, nil)

	reqBody := `{"email":"newuser@example.com","password":"securepassword123","name":"New User"}`
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	t.Logf("=== RAW HTTP REQUEST: POST /signup ===")
	t.Logf("POST /signup HTTP/1.1\nContent-Type: application/json\n\n%s", reqBody)

	mux.ServeHTTP(w, req)

	t.Logf("=== RAW HTTP RESPONSE: POST /signup ===")
	t.Logf("HTTP/1.1 %d\nSet-Cookie: %s\nContent-Type: %s\n\n%s",
		w.Code, w.Header().Get("Set-Cookie"), w.Header().Get("Content-Type"), w.Body.String())

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	setCookie := w.Header().Get("Set-Cookie")
	if !strings.HasPrefix(setCookie, "_mold_session=") {
		t.Errorf("expected Set-Cookie header starting with '_mold_session=', got: %s", setCookie)
	}

	var res struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
		t.Fatalf("failed decoding JSON response: %v", err)
	}

	if _, exists := res.Data["password"]; exists {
		t.Errorf("SECURITY RISK: password field found in signup response: %v", res.Data)
	}

	if res.Data["email"] != "newuser@example.com" {
		t.Errorf("expected email 'newuser@example.com', got: %v", res.Data["email"])
	}
	if res.Data["role"] != "user" {
		t.Errorf("expected role default 'user', got: %v", res.Data["role"])
	}
}

// TestAuthGlue_SignupPreAccountTakeoverBlocked verifies that passing provider & provider_user_id in /signup
// is ignored by payload whitelisting and does NOT pre-link the account.
func TestAuthGlue_SignupPreAccountTakeoverBlocked(t *testing.T) {
	app, _ := setupTestApp(t)
	mux := setupTestServer(app, nil)

	// Attacker attempts pre-account takeover by providing target victim's Google sub in signup payload
	attackerReqBody := `{"email":"victim@example.com","password":"attackerpassword123","provider":"google","provider_user_id":"g_victim_12345"}`
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(attackerReqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected signup to succeed for whitelisted fields, got %d: %s", w.Code, w.Body.String())
	}

	var res struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)

	if res.Data["provider"] != nil || res.Data["provider_user_id"] != nil {
		t.Errorf("SECURITY VULNERABILITY: Pre-account takeover failed! provider/provider_user_id were copied: %v", res.Data)
	}
}

// TestAuthGlue_DuplicateEmailSignupBlocked verifies duplicate email registrations are blocked by unique constraint.
func TestAuthGlue_DuplicateEmailSignupBlocked(t *testing.T) {
	app, _ := setupTestApp(t)
	mux := setupTestServer(app, nil)

	reqBody := `{"email":"dup@example.com","password":"password123","name":"First Registrant"}`
	req1 := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(reqBody))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	mux.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("first signup failed: %d: %s", w1.Code, w1.Body.String())
	}

	// Second signup with same email address
	req2 := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(reqBody))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	mux.ServeHTTP(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for duplicate email signup, got %d: %s", w2.Code, w2.Body.String())
	}
}

// TestAuthGlue_PrivilegeEscalationBlocked verifies passing role: "admin" in signup payload
// is rejected with 400 Bad Request (CLIENT_WRITE_FORBIDDEN) due to client_writable: false.
func TestAuthGlue_PrivilegeEscalationBlocked(t *testing.T) {
	app, _ := setupTestApp(t)
	mux := setupTestServer(app, nil)

	reqBody := `{"email":"attacker@example.com","password":"password123","name":"Attacker","role":"admin"}`
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	t.Logf("=== RAW HTTP REQUEST: POST /signup (Privilege Escalation Attempt) ===")
	t.Logf("POST /signup HTTP/1.1\nContent-Type: application/json\n\n%s", reqBody)

	mux.ServeHTTP(w, req)

	t.Logf("=== RAW HTTP RESPONSE: POST /signup (Privilege Escalation Rejected) ===")
	t.Logf("HTTP/1.1 %d\nContent-Type: %s\n\n%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for client_writable role write, got %d: %s", w.Code, w.Body.String())
	}

	var errRes struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &errRes); err != nil {
		t.Fatalf("failed decoding error response: %v", err)
	}

	if errRes.Error.Code != "CLIENT_WRITE_FORBIDDEN" {
		t.Errorf("expected error code 'CLIENT_WRITE_FORBIDDEN', got: %s", errRes.Error.Code)
	}
}

// TestAuthGlue_NilVerifierRejected verifies passing a nil verifier returns HTTP 500 (OAUTH_VERIFIER_REQUIRED).
func TestAuthGlue_NilVerifierRejected(t *testing.T) {
	app, _ := setupTestApp(t)
	handler := authglue.OAuthCallbackHandler(app, "google", nil)

	req := httptest.NewRequest(http.MethodPost, "/auth/google/callback", strings.NewReader(`{"code":"123"}`))
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 Internal Server Error for nil verifier, got %d: %s", w.Code, w.Body.String())
	}

	var res struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &res)
	if res.Error.Code != "OAUTH_VERIFIER_REQUIRED" {
		t.Errorf("expected error code 'OAUTH_VERIFIER_REQUIRED', got: %s", res.Error.Code)
	}
}

// TestAuthGlue_OAuthCallbackFindOrCreate verifies 1st login creates new user and 2nd login reuses user.
func TestAuthGlue_OAuthCallbackFindOrCreate(t *testing.T) {
	app, _ := setupTestApp(t)

	mockVerifier := func(ctx context.Context, r *http.Request) (*authglue.OAuthUser, error) {
		var body struct {
			Code string `json:"code"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body.Code == "google_valid_code" {
			return &authglue.OAuthUser{
				Provider:       "google",
				ProviderUserID: "g_1029384756",
				Email:          "oauth_user@example.com",
				Name:           "OAuth User",
			}, nil
		}
		return nil, fmt.Errorf("invalid oauth code")
	}

	mux := setupTestServer(app, mockVerifier)

	// --- 1st Login: Create new user ---
	reqBody1 := `{"code":"google_valid_code"}`
	req1 := httptest.NewRequest(http.MethodPost, "/auth/google/callback", strings.NewReader(reqBody1))
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()

	t.Logf("=== RAW HTTP REQUEST: 1st OAuth Callback (New User) ===")
	t.Logf("POST /auth/google/callback HTTP/1.1\nContent-Type: application/json\n\n%s", reqBody1)

	mux.ServeHTTP(w1, req1)

	t.Logf("=== RAW HTTP RESPONSE: 1st OAuth Callback (201 Created) ===")
	t.Logf("HTTP/1.1 %d\nSet-Cookie: %s\nContent-Type: %s\n\n%s",
		w1.Code, w1.Header().Get("Set-Cookie"), w1.Header().Get("Content-Type"), w1.Body.String())

	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on first OAuth login, got %d: %s", w1.Code, w1.Body.String())
	}

	cookie1 := w1.Header().Get("Set-Cookie")
	if !strings.HasPrefix(cookie1, "_mold_session=") {
		t.Errorf("expected session cookie on 1st login, got: %s", cookie1)
	}

	var res1 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w1.Body.Bytes(), &res1); err != nil {
		t.Fatalf("failed decoding 1st login JSON: %v", err)
	}
	userID1 := res1.Data["id"]

	// --- 2nd Login: Reuse existing user (no duplicate creation) ---
	reqBody2 := `{"code":"google_valid_code"}`
	req2 := httptest.NewRequest(http.MethodPost, "/auth/google/callback", strings.NewReader(reqBody2))
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()

	t.Logf("=== RAW HTTP REQUEST: 2nd OAuth Callback (Existing User Re-login) ===")
	t.Logf("POST /auth/google/callback HTTP/1.1\nContent-Type: application/json\n\n%s", reqBody2)

	mux.ServeHTTP(w2, req2)

	t.Logf("=== RAW HTTP RESPONSE: 2nd OAuth Callback (200 OK Reused User) ===")
	t.Logf("HTTP/1.1 %d\nSet-Cookie: %s\nContent-Type: %s\n\n%s",
		w2.Code, w2.Header().Get("Set-Cookie"), w2.Header().Get("Content-Type"), w2.Body.String())

	if w2.Code != http.StatusOK {
		t.Fatalf("expected 200 OK on re-login, got %d: %s", w2.Code, w2.Body.String())
	}

	var res2 struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(w2.Body.Bytes(), &res2); err != nil {
		t.Fatalf("failed decoding 2nd login JSON: %v", err)
	}
	userID2 := res2.Data["id"]

	if fmt.Sprintf("%v", userID1) != fmt.Sprintf("%v", userID2) {
		t.Errorf("expected same user id on re-login (find-or-create), got %v vs %v", userID1, userID2)
	}
}

// TestAuthGlue_ProtectedResourceAccessWithIssuedCookie verifies issued session cookie works with protected endpoints.
func TestAuthGlue_ProtectedResourceAccessWithIssuedCookie(t *testing.T) {
	app, _ := setupTestApp(t)
	mux := setupTestServer(app, nil)
	ctx := context.Background()

	// 1. Signup user via /signup to get session cookie
	reqBody := `{"email":"owner@example.com","password":"password123","name":"Owner User"}`
	req := httptest.NewRequest(http.MethodPost, "/signup", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("signup failed: %d: %s", w.Code, w.Body.String())
	}

	cookieVal := w.Header().Get("Set-Cookie")
	var signupRes struct {
		Data map[string]any `json:"data"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &signupRes)
	userID := signupRes.Data["id"].(float64)

	// 2. Create Post record owned by this user
	postRec, err := app.CreateRecord(ctx, "Post", map[string]any{
		"title":     "Protected Post Title",
		"body":      "# Protected Content",
		"author_id": int64(userID),
	})
	if err != nil {
		t.Fatalf("failed creating post: %v", err)
	}
	postID := postRec["id"]

	// 3. Access GET /api/posts/{id} with session cookie -> 200 OK
	reqPost, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/posts/%v", postID), nil)
	reqPost.Header.Set("Cookie", cookieVal)
	wPost := httptest.NewRecorder()

	t.Logf("=== RAW HTTP REQUEST: GET Protected Resource with Issued Cookie ===")
	t.Logf("GET /api/posts/%v HTTP/1.1\nCookie: %s", postID, cookieVal)

	mux.ServeHTTP(wPost, reqPost)

	t.Logf("=== RAW HTTP RESPONSE: GET Protected Resource ===")
	t.Logf("HTTP/1.1 %d\nContent-Type: %s\n\n%s", wPost.Code, wPost.Header().Get("Content-Type"), wPost.Body.String())

	if wPost.Code != http.StatusOK {
		t.Fatalf("expected 200 OK when accessing protected resource with session cookie, got %d: %s", wPost.Code, wPost.Body.String())
	}

	// 4. Access GET /api/posts/{id} without session cookie -> 401 Unauthorized / 403 Forbidden
	reqUnauth, _ := http.NewRequest(http.MethodGet, fmt.Sprintf("/api/posts/%v", postID), nil)
	wUnauth := httptest.NewRecorder()
	mux.ServeHTTP(wUnauth, reqUnauth)

	if wUnauth.Code != http.StatusUnauthorized && wUnauth.Code != http.StatusForbidden {
		t.Errorf("expected 401/403 for unauthenticated read of owner-restricted post, got %d: %s", wUnauth.Code, wUnauth.Body.String())
	}
}
