package runtime_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/runtime"
)

// TestMigrationTroubleshooting_EmpiricalProof verifies the exact behavior of
// adding fields under Mold's destructive-only migration policy.
func TestMigrationTroubleshooting_EmpiricalProof(t *testing.T) {
	tmpDir := t.TempDir()
	resDir := filepath.Join(tmpDir, "resources")
	if err := os.MkdirAll(resDir, 0755); err != nil {
		t.Fatalf("failed creating resDir: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "quickstart.db")

	// Phase 1: Boot app with Post.yaml version 1 (title & body only)
	v1YAML := `
resource:
  name: Post
  schema_version: 1
fields:
  - name: title
    type: string
  - name: body
    type: markdown
`
	postYAMLPath := filepath.Join(resDir, "Post.yaml")
	if err := os.WriteFile(postYAMLPath, []byte(v1YAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml v1: %v", err)
	}

	app1, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed starting app1: %v", err)
	}
	// Insert post 1
	w1 := httptest.NewRecorder()
	req1, _ := http.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(`{"title":"V1 Title","body":"V1 Body"}`))
	req1.Header.Set("Content-Type", "application/json")
	app1.ServeHTTP(w1, req1)
	if w1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on v1 insert, got %d: %s", w1.Code, w1.Body.String())
	}
	app1.Close()

	// Phase 2: Add author_id to Post.yaml WITHOUT bumping schema_version (still schema_version: 1)
	v1WithFieldYAML := `
resource:
  name: Post
  schema_version: 1
fields:
  - name: title
    type: string
  - name: body
    type: markdown
  - name: author_id
    type: int
`
	if err := os.WriteFile(postYAMLPath, []byte(v1WithFieldYAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml v1 with author_id: %v", err)
	}

	app2, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed starting app2: %v", err)
	}
	w2 := httptest.NewRecorder()
	req2, _ := http.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(`{"title":"V2 Title","body":"V2 Body","author_id":100}`))
	req2.Header.Set("Content-Type", "application/json")
	app2.ServeHTTP(w2, req2)

	t.Logf("=== EMPIRICAL PROOF 1: Adding field without bumping schema_version (schema_version=1) ===")
	t.Logf("POST /api/posts with author_id: 100")
	t.Logf("HTTP/1.1 %d\n%s", w2.Code, w2.Body.String())

	if w2.Code != http.StatusBadRequest || !strings.Contains(w2.Body.String(), "no column named author_id") {
		t.Errorf("expected 400 Bad Request with error containing 'no column named author_id', got status %d: %s", w2.Code, w2.Body.String())
	}
	app2.Close()

	// Phase 3 (Choice A): Delete DB file and restart
	if err := os.Remove(dbPath); err != nil {
		t.Fatalf("failed removing dbPath: %v", err)
	}
	app3, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed starting app3 after DB deletion: %v", err)
	}
	w3 := httptest.NewRecorder()
	req3, _ := http.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(`{"title":"Fresh DB Title","body":"Fresh DB Body","author_id":100}`))
	req3.Header.Set("Content-Type", "application/json")
	app3.ServeHTTP(w3, req3)

	t.Logf("=== EMPIRICAL PROOF 2: Choice A (Delete DB file and restart) ===")
	t.Logf("POST /api/posts with author_id: 100 on fresh DB")
	t.Logf("HTTP/1.1 %d\n%s", w3.Code, w3.Body.String())

	if w3.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created on fresh DB, got %d: %s", w3.Code, w3.Body.String())
	}
	app3.Close()

	// Phase 4 (Choice B): Bump schema_version: 2 on existing DB
	v2YAML := `
resource:
  name: Post
  schema_version: 2
fields:
  - name: title
    type: string
  - name: body
    type: markdown
  - name: author_id
    type: int
`
	if err := os.WriteFile(postYAMLPath, []byte(v2YAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml v2: %v", err)
	}

	app4, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: dbPath})
	if err != nil {
		t.Fatalf("failed starting app4 with schema_version: 2: %v", err)
	}
	w4 := httptest.NewRecorder()
	req4, _ := http.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(`{"title":"Bumped Version Title","body":"Bumped Version Body","author_id":200}`))
	req4.Header.Set("Content-Type", "application/json")
	app4.ServeHTTP(w4, req4)

	t.Logf("=== EMPIRICAL PROOF 3: Choice B (Bump schema_version: 2 on existing DB) ===")
	t.Logf("POST /api/posts with author_id: 200 after schema_version bump")
	t.Logf("HTTP/1.1 %d\n%s", w4.Code, w4.Body.String())

	if w4.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created after schema_version bump, got %d: %s", w4.Code, w4.Body.String())
	}
	app4.Close()

	// Phase 5 (Choice B Multi-Resource Preservation Test):
	// Register both User (v1) and Post (v1), insert records in both,
	// then bump ONLY Post to schema_version: 2.
	// Verify User records remain intact while Post is re-created.
	multiDBPath := filepath.Join(tmpDir, "multi_resource.db")
	userYAMLPath := filepath.Join(resDir, "User.yaml")

	userYAML := `
resource:
  name: User
  schema_version: 1
fields:
  - name: email
    type: email
  - name: name
    type: string
`
	if err := os.WriteFile(userYAMLPath, []byte(userYAML), 0644); err != nil {
		t.Fatalf("failed writing User.yaml: %v", err)
	}

	// Reset Post.yaml to v1
	if err := os.WriteFile(postYAMLPath, []byte(v1YAML), 0644); err != nil {
		t.Fatalf("failed resetting Post.yaml: %v", err)
	}

	appMulti1, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: multiDBPath})
	if err != nil {
		t.Fatalf("failed starting appMulti1: %v", err)
	}
	// Insert User
	wU1 := httptest.NewRecorder()
	reqU1, _ := http.NewRequest(http.MethodPost, "/api/users", strings.NewReader(`{"email":"user1@example.com","name":"User 1"}`))
	reqU1.Header.Set("Content-Type", "application/json")
	appMulti1.ServeHTTP(wU1, reqU1)
	if wU1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for User insert, got %d: %s", wU1.Code, wU1.Body.String())
	}

	// Insert Post
	wP1 := httptest.NewRecorder()
	reqP1, _ := http.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(`{"title":"Initial Post","body":"Initial Body"}`))
	reqP1.Header.Set("Content-Type", "application/json")
	appMulti1.ServeHTTP(wP1, reqP1)
	if wP1.Code != http.StatusCreated {
		t.Fatalf("expected 201 Created for Post insert, got %d: %s", wP1.Code, wP1.Body.String())
	}
	appMulti1.Close()

	// Bump ONLY Post to schema_version: 2 (User remains schema_version: 1)
	if err := os.WriteFile(postYAMLPath, []byte(v2YAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml v2 for multi test: %v", err)
	}

	appMulti2, err := runtime.New(runtime.Config{ResourceDir: resDir, DBPath: multiDBPath})
	if err != nil {
		t.Fatalf("failed starting appMulti2: %v", err)
	}
	defer appMulti2.Close()

	// Check GET /api/users -> User record MUST still exist (200 OK, len == 1)
	wUCheck := httptest.NewRecorder()
	reqUCheck, _ := http.NewRequest(http.MethodGet, "/api/users", nil)
	appMulti2.ServeHTTP(wUCheck, reqUCheck)

	t.Logf("=== EMPIRICAL PROOF 4: Multi-Resource Test (User v1 intact after Post v2 bump) ===")
	t.Logf("GET /api/users")
	t.Logf("HTTP/1.1 %d\n%s", wUCheck.Code, wUCheck.Body.String())

	if wUCheck.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /api/users, got %d: %s", wUCheck.Code, wUCheck.Body.String())
	}

	var userListRes struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(wUCheck.Body.Bytes(), &userListRes); err != nil {
		t.Fatalf("failed parsing users list: %v", err)
	}
	if len(userListRes.Data) != 1 || userListRes.Data[0]["email"] != "user1@example.com" {
		t.Fatalf("expected User record 'user1@example.com' to be preserved, got: %v", userListRes.Data)
	}

	// Check GET /api/posts -> Post records were dropped and recreated (200 OK, len == 0)
	wPCheck := httptest.NewRecorder()
	reqPCheck, _ := http.NewRequest(http.MethodGet, "/api/posts", nil)
	appMulti2.ServeHTTP(wPCheck, reqPCheck)

	t.Logf("=== EMPIRICAL PROOF 5: Multi-Resource Test (Post v2 table dropped & recreated) ===")
	t.Logf("GET /api/posts")
	t.Logf("HTTP/1.1 %d\n%s", wPCheck.Code, wPCheck.Body.String())

	if wPCheck.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /api/posts, got %d: %s", wPCheck.Code, wPCheck.Body.String())
	}

	var postListRes struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(wPCheck.Body.Bytes(), &postListRes); err != nil {
		t.Fatalf("failed parsing posts list: %v", err)
	}
	if len(postListRes.Data) != 0 {
		t.Fatalf("expected Post records to be destructively dropped (len == 0), got len == %d", len(postListRes.Data))
	}
}
