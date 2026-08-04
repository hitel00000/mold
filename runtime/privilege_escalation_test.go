package runtime_test

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

	"github.com/hitel00000/mold/runtime"
)

// TestPrivilegeEscalation_EmpiricalProof executes raw HTTP requests against Mold runtime
// to empirically demonstrate field-level privilege escalation & protection behaviors across 3 distinct resources/fields.
func TestPrivilegeEscalation_EmpiricalProof(t *testing.T) {
	tmpDir := t.TempDir()

	// 1. User.yaml (Case 1: User.role + create: public)
	userYaml := `resource:
  name: User
  timestamps: true
  soft_delete: true

fields:
  - name: email
    type: email
    nullable: false
    constraints:
      unique: true
  - name: password
    type: password
    nullable: false
  - name: name
    type: string
    nullable: false
  - name: role
    type: enum
    nullable: false
    default: "user"
    constraints:
      values: ["admin", "user"]

auth:
  permissions:
    create: public
    read: authenticated
    update: owner
    delete: role:admin
`
	if err := os.WriteFile(filepath.Join(tmpDir, "User.yaml"), []byte(userYaml), 0644); err != nil {
		t.Fatalf("failed writing User.yaml: %v", err)
	}

	// 2. Post.yaml (Case 2: Post.author_id + create: authenticated)
	postYaml := `resource:
  name: Post
  timestamps: true
  soft_delete: true

fields:
  - name: title
    type: string
    nullable: false
  - name: body
    type: text
    nullable: false
  - name: author_id
    type: int
    nullable: true

auth:
  ownership_field: author_id
  permissions:
    create: authenticated
    read: public
    update: owner
    delete: owner
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Post.yaml"), []byte(postYaml), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml: %v", err)
	}

	// 3. SakeRecord.yaml (Case 3: SakeRecord.owner_id + create: authenticated from drink-log pilot)
	sakeYaml := `resource:
  name: SakeRecord
  timestamps: true
  soft_delete: true

fields:
  - name: name
    type: string
    nullable: false
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
	if err := os.WriteFile(filepath.Join(tmpDir, "SakeRecord.yaml"), []byte(sakeYaml), 0644); err != nil {
		t.Fatalf("failed writing SakeRecord.yaml: %v", err)
	}

	dbPath := filepath.Join(tmpDir, "escalation_test.db")
	app, err := runtime.New(runtime.Config{
		ResourceDir: tmpDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed starting runtime app: %v", err)
	}
	defer app.Close()

	ctx := context.Background()

	// --- Case 1: User.role payload write attempt by non-admin ---
	t.Run("Case1_UserRole_EscalationBlockedByCoreAuth", func(t *testing.T) {
		// Attempting to send "role": "admin" in payload without admin session
		reqBody := `{"email":"attacker@example.com","password":"password123","name":"Attacker","role":"admin"}`
		req, _ := http.NewRequest(http.MethodPost, "/api/users", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")

		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)

		t.Logf("=== CASE 1 RAW HTTP REQUEST (role: admin) ===")
		t.Logf("POST /api/users HTTP/1.1\nContent-Type: application/json\n\n%s", reqBody)
		t.Logf("=== CASE 1 RAW HTTP RESPONSE ===")
		t.Logf("HTTP/1.1 %d\nContent-Type: %s\n\n%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())

		// Core auth Evaluate() traps "role" payload write and returns 403 FORBIDDEN
		if w.Code != http.StatusForbidden {
			t.Fatalf("expected 403 Forbidden for non-admin role payload, got %d: %s", w.Code, w.Body.String())
		}
		if !strings.Contains(w.Body.String(), "role field can only be modified by admin users") {
			t.Errorf("expected role escalation error message, got: %s", w.Body.String())
		}
	})

	// --- Case 2: Post.author_id forgery (Authenticated user 100 specifying author_id: 999) ---
	t.Run("Case2_PostAuthorID_ForgeryAccepted", func(t *testing.T) {
		cookieUser100, _, err := app.IssueSessionForUser(ctx, 100, "user")
		if err != nil {
			t.Fatalf("failed issuing session for user 100: %v", err)
		}

		reqBody := `{"title":"Impersonated Post","body":"Written by User 100 pretending to be User 999","author_id":999}`
		req, _ := http.NewRequest(http.MethodPost, "/api/posts", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookieUser100)

		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)

		t.Logf("=== CASE 2 RAW HTTP REQUEST (author_id forgery) ===")
		t.Logf("POST /api/posts HTTP/1.1\nCookie: %s\nContent-Type: application/json\n\n%s", cookieUser100, reqBody)
		t.Logf("=== CASE 2 RAW HTTP RESPONSE ===")
		t.Logf("HTTP/1.1 %d\nContent-Type: %s\n\n%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		var res struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed parsing response JSON: %v", err)
		}

		// Empirical proof: author_id was accepted as 999 despite session being user 100
		authorIDVal, ok := res.Data["author_id"]
		if !ok || fmt.Sprintf("%v", authorIDVal) != "999" {
			t.Errorf("expected created post author_id to be 999, got %v", authorIDVal)
		}
	})

	// --- Case 3: SakeRecord.owner_id forgery (Authenticated user 101 specifying owner_id: 202) ---
	t.Run("Case3_SakeRecordOwnerID_ForgeryAccepted", func(t *testing.T) {
		cookieUser101, _, err := app.IssueSessionForUser(ctx, 101, "user")
		if err != nil {
			t.Fatalf("failed issuing session for user 101: %v", err)
		}

		reqBody := `{"name":"Forgery Sake Record","owner_id":202}`
		req, _ := http.NewRequest(http.MethodPost, "/api/sake_records", strings.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Cookie", cookieUser101)

		w := httptest.NewRecorder()
		app.ServeHTTP(w, req)

		t.Logf("=== CASE 3 RAW HTTP REQUEST (owner_id forgery) ===")
		t.Logf("POST /api/sake_records HTTP/1.1\nCookie: %s\nContent-Type: application/json\n\n%s", cookieUser101, reqBody)
		t.Logf("=== CASE 3 RAW HTTP RESPONSE ===")
		t.Logf("HTTP/1.1 %d\nContent-Type: %s\n\n%s", w.Code, w.Header().Get("Content-Type"), w.Body.String())

		if w.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d: %s", w.Code, w.Body.String())
		}

		var res struct {
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed parsing response JSON: %v", err)
		}

		// Empirical proof: owner_id was accepted as 202 despite session being user 101
		ownerIDVal, ok := res.Data["owner_id"]
		if !ok || fmt.Sprintf("%v", ownerIDVal) != "202" {
			t.Errorf("expected created sake record owner_id to be 202, got %v", ownerIDVal)
		}
	})
}
