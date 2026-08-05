package quickstart_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/runtime"
)

// TestQuickstart_Basic verifies the basic quickstart example (sections 1-4).
func TestQuickstart_Basic(t *testing.T) {
	resDir := filepath.Join("basic", "resources")
	dbPath := filepath.Join(t.TempDir(), "basic.db")

	app, err := runtime.New(runtime.Config{
		ResourceDir: resDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed initializing basic App: %v", err)
	}
	defer app.Close()

	// 1. Create Post
	reqBody := `{"title": "Hello Mold Basic", "body": "# Content"}`
	req, _ := http.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	app.ServeHTTP(w, req)

	t.Logf("=== BASIC QUICKSTART POST /api/posts REQUEST ===")
	t.Logf("POST /api/posts\n%s", reqBody)
	t.Logf("=== BASIC QUICKSTART RESPONSE ===")
	t.Logf("HTTP/1.1 %d\n%s", w.Code, w.Body.String())

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
	}

	// 2. List Posts
	reqList, _ := http.NewRequest(http.MethodGet, "/api/posts", nil)
	wList := httptest.NewRecorder()
	app.ServeHTTP(wList, reqList)

	if wList.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", wList.Code, wList.Body.String())
	}
}

// TestQuickstart_WithAuth verifies the auth quickstart example (section 5).
func TestQuickstart_WithAuth(t *testing.T) {
	resDir := filepath.Join("with-auth", "resources")
	dbPath := filepath.Join(t.TempDir(), "with_auth.db")

	app, err := runtime.New(runtime.Config{
		ResourceDir: resDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed initializing with-auth App: %v", err)
	}
	defer app.Close()

	// Handlers from with-auth/main.go logic
	signupHandler := func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
			Name     string `json:"name"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx := context.Background()
		user, err := app.CreateRecord(ctx, "User", map[string]any{
			"email":    req.Email,
			"password": req.Password,
			"name":     req.Name,
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var userID int64
		switch v := user["id"].(type) {
		case int64:
			userID = v
		case float64:
			userID = int64(v)
		case int:
			userID = int64(v)
		}
		cookieVal, _, err := app.IssueSessionForUser(ctx, userID, "user")
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Set-Cookie", cookieVal)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"data": user})
	}

	createPostHandler := func(w http.ResponseWriter, r *http.Request) {
		userID, _, ok := app.SessionUser(r)
		if !ok {
			http.Error(w, `{"error":{"code":"UNAUTHORIZED","message":"login required"}}`, http.StatusUnauthorized)
			return
		}
		var req struct {
			Title string `json:"title"`
			Body  string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		ctx := context.Background()
		post, err := app.CreateRecord(ctx, "Post", map[string]any{
			"title":     req.Title,
			"body":      req.Body,
			"author_id": userID, // forced from session
		})
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]any{"data": post})
	}

	// 1. Direct POST /api/users with non-client-writable field (badge: gold) -> Rejection (400 Bad Request / CLIENT_WRITE_FORBIDDEN)
	badBody := `{"email":"hacker@example.com","password":"password123","name":"Hacker","badge":"gold"}`
	reqBad, _ := http.NewRequest(http.MethodPost, "/api/users", strings.NewReader(badBody))
	reqBad.Header.Set("Content-Type", "application/json")
	wBad := httptest.NewRecorder()
	app.ServeHTTP(wBad, reqBad)

	t.Logf("=== DIRECT POST /api/users REJECTION TEST (role: admin) ===")
	t.Logf("POST /api/users\n%s", badBody)
	t.Logf("HTTP/1.1 %d\n%s", wBad.Code, wBad.Body.String())

	if wBad.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request for role: admin payload, got %d: %s", wBad.Code, wBad.Body.String())
	}
	if !strings.Contains(wBad.Body.String(), "CLIENT_WRITE_FORBIDDEN") {
		t.Errorf("expected error code CLIENT_WRITE_FORBIDDEN, got %s", wBad.Body.String())
	}

	// 2. Direct POST /api/users omitting role -> 201 Created & default role: "user" applied
	pubBody := `{"email":"public_user@example.com","password":"password123","name":"Public User"}`
	reqPub, _ := http.NewRequest(http.MethodPost, "/api/users", strings.NewReader(pubBody))
	reqPub.Header.Set("Content-Type", "application/json")
	wPub := httptest.NewRecorder()
	app.ServeHTTP(wPub, reqPub)

	t.Logf("=== DIRECT POST /api/users PUBLIC SIGNUP TEST (role omitted) ===")
	t.Logf("POST /api/users\n%s", pubBody)
	t.Logf("HTTP/1.1 %d\n%s", wPub.Code, wPub.Body.String())

	if wPub.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for public signup, got %d: %s", wPub.Code, wPub.Body.String())
	}

	// Issue session for created user ID 1 for subsequent authenticated GET/POST requests
	cookiePubVal, _, err := app.IssueSessionForUser(context.Background(), 1, "user")
	if err != nil {
		t.Fatalf("failed issuing session for public user: %v", err)
	}

	// 3. GET /api/users/1 -> Verify default role: "user" in response & password field sanitized/absent
	reqGet, _ := http.NewRequest(http.MethodGet, "/api/users/1", nil)
	reqGet.Header.Set("Cookie", cookiePubVal)
	wGet := httptest.NewRecorder()
	app.ServeHTTP(wGet, reqGet)

	t.Logf("=== GET /api/users/1 AUDIT TEST (default role & password sanitization) ===")
	t.Logf("GET /api/users/1\nCookie: %s", cookiePubVal)
	t.Logf("HTTP/1.1 %d\n%s", wGet.Code, wGet.Body.String())

	if wGet.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for user detail GET, got %d: %s", wGet.Code, wGet.Body.String())
	}
	var getRes struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(wGet.Body.Bytes(), &getRes); err != nil {
		t.Fatalf("failed parsing GET response: %v", err)
	}
	if getRes.Data["role"] != "user" {
		t.Errorf("expected default role to be 'user', got %v", getRes.Data["role"])
	}
	if _, passExposed := getRes.Data["password"]; passExposed {
		t.Errorf("SECURITY RISK: password exposed in GET response!")
	}

	// 4. Test /signup Glue Handler (instant session cookie issuance)
	signupBody := `{"email":"newuser@example.com","password":"password123","name":"New User"}`
	reqSignup, _ := http.NewRequest(http.MethodPost, "/signup", strings.NewReader(signupBody))
	reqSignup.Header.Set("Content-Type", "application/json")
	wSignup := httptest.NewRecorder()
	signupHandler(wSignup, reqSignup)

	t.Logf("=== WITH-AUTH QUICKSTART POST /signup REQUEST ===")
	t.Logf("POST /signup\n%s", signupBody)
	t.Logf("=== WITH-AUTH QUICKSTART RESPONSE ===")
	t.Logf("HTTP/1.1 %d\nSet-Cookie: %s\n%s", wSignup.Code, wSignup.Header().Get("Set-Cookie"), wSignup.Body.String())

	if wSignup.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for signup, got %d: %s", wSignup.Code, wSignup.Body.String())
	}

	cookieVal := wSignup.Header().Get("Set-Cookie")
	if !strings.Contains(cookieVal, "_mold_session=") {
		t.Fatalf("expected _mold_session cookie, got: %s", cookieVal)
	}

	// 5. Test /posts/create with session cookie
	postBody := `{"title":"Auth Post Title","body":"Auth Post Body"}`
	reqPost, _ := http.NewRequest(http.MethodPost, "/posts/create", strings.NewReader(postBody))
	reqPost.Header.Set("Content-Type", "application/json")
	reqPost.Header.Set("Cookie", cookieVal)
	wPost := httptest.NewRecorder()
	createPostHandler(wPost, reqPost)

	t.Logf("=== WITH-AUTH QUICKSTART POST /posts/create REQUEST ===")
	t.Logf("POST /posts/create\nCookie: %s\n%s", cookieVal, postBody)
	t.Logf("=== WITH-AUTH QUICKSTART RESPONSE ===")
	t.Logf("HTTP/1.1 %d\n%s", wPost.Code, wPost.Body.String())

	if wPost.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for post create, got %d: %s", wPost.Code, wPost.Body.String())
	}

	var postRes struct {
		Data map[string]any `json:"data"`
	}
	if err := json.Unmarshal(wPost.Body.Bytes(), &postRes); err != nil {
		t.Fatalf("failed parsing post response: %v", err)
	}

	// Verify author_id was automatically forced from session (userID = 2)
	authorIDVal := fmt.Sprintf("%v", postRes.Data["author_id"])
	if authorIDVal != "2" {
		t.Errorf("expected author_id to be forced to 2 from session, got %v", authorIDVal)
	}
}
