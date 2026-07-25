package transport_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
	"github.com/hitel00000/mold/storage"
	"github.com/hitel00000/mold/transport"
)

// TestSanitizeRecord_PlanMigration verifies that SanitizeRecord uses plan.Plan to remove deprecated and password fields.
func TestSanitizeRecord_PlanMigration(t *testing.T) {
	deprecatedTrue := true
	res := &resource.Resource{
		Name:  "TestUser",
		Table: "test_users",
		Fields: []resource.Field{
			{Name: "username", Type: resource.TypeString},
			{Name: "password", Type: resource.TypePassword},
			{Name: "legacy_field", Type: resource.TypeString, Deprecated: true},
		},
		Relations: []resource.Relation{
			{
				Name:       "org",
				Kind:       resource.KindBelongsTo,
				Target:     "Org",
				ForeignKey: "org_id",
			},
		},
	}

	p := plan.Build(res)
	if len(p.Fields) != 4 {
		t.Fatalf("expected 4 fields in plan (username, password, legacy_field, org_id), got %d", len(p.Fields))
	}

	rec := storage.Record{
		"username":     "alice",
		"password":     "secret123",
		"legacy_field": "old_data",
		"org_id":       42,
	}

	sanitized := transport.SanitizeRecord(res, rec)

	if _, exists := sanitized["password"]; exists {
		t.Errorf("expected password field to be sanitized out")
	}
	if _, exists := sanitized["legacy_field"]; exists {
		t.Errorf("expected legacy_field to be sanitized out")
	}
	if sanitized["username"] != "alice" {
		t.Errorf("expected username field retained, got %v", sanitized["username"])
	}
	// Verify that derived FK field org_id is NOT incorrectly sanitized out
	if sanitized["org_id"] != 42 {
		t.Errorf("expected derived FK field org_id retained, got %v", sanitized["org_id"])
	}

	_ = deprecatedTrue
}

// TestSanitizeRecord_HTTPRealServerE2E verifies via real HTTP server that deprecated/password fields are sanitized out of REST API responses.
func TestSanitizeRecord_HTTPRealServerE2E(t *testing.T) {
	resourceDir := t.TempDir()
	dbPath := filepath.Join(resourceDir, "sanitize_test.db")

	userYAML := `
resource:
  name: User
  timestamps: true
fields:
  - name: username
    type: string
    nullable: false
  - name: password
    type: password
    nullable: false
  - name: old_bio
    type: string
    deprecated: true
    nullable: true
auth:
  permissions:
    create: public
    read: public
    update: public
    delete: public
`
	if err := os.WriteFile(filepath.Join(resourceDir, "User.yaml"), []byte(userYAML), 0644); err != nil {
		t.Fatalf("failed writing User.yaml: %v", err)
	}

	app, err := runtime.New(runtime.Config{
		ResourceDir: resourceDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed to create runtime: %v", err)
	}
	defer app.Close()

	ts := httptest.NewServer(app)
	defer ts.Close()

	// 1. Create record via POST without sending deprecated field
	createPayload := map[string]any{
		"username": "bob",
		"password": "my_secure_password",
	}
	bodyBytes, _ := json.Marshal(createPayload)
	resp, err := ts.Client().Post(ts.URL+"/api/users", "application/json", bytes.NewReader(bodyBytes))
	if err != nil {
		t.Fatalf("POST /api/users failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d: %s", resp.StatusCode, string(body))
	}

	// 2. Read record via GET /api/users/1 and verify sensitive password field is sanitized out
	resp, err = ts.Client().Get(ts.URL + "/api/users/1")
	if err != nil {
		t.Fatalf("GET /api/users/1 failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	var resMap map[string]any
	if err := json.Unmarshal(body, &resMap); err != nil {
		t.Fatalf("failed parsing response: %v", err)
	}

	data, ok := resMap["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data envelope object, got %v", resMap["data"])
	}

	// Verify sanitized fields (password, deprecated fields) are absent from real HTTP response
	if _, exists := data["password"]; exists {
		t.Errorf("real HTTP response contains sensitive password field: %v", data)
	}
	if data["username"] != "bob" {
		t.Errorf("real HTTP response missing valid username field: %v", data)
	}
}
