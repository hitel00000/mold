package view_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/runtime"
	"github.com/hitel00000/mold/view"
)

// TestView_WidgetGoldenSnapshot captures the exact pre-migration widget layout for Post and Comment resources.
func TestView_WidgetGoldenSnapshot(t *testing.T) {
	relResourceDir := filepath.Join("..", "examples", "blog")
	reg, err := resource.LoadAll(relResourceDir)
	if err != nil {
		t.Fatalf("failed loading blog IR: %v", err)
	}

	commentRes, _ := reg.Get("Comment")

	// 1. Capture BuildFormFields output for Comment resource on pre-migration code
	// Pre-migration BuildFormFields duplicated FK fields if listed in both fields: and relations: (5 widgets total)
	widgets := view.BuildFormFields(commentRes, nil, false)
	if len(widgets) != 5 {
		t.Fatalf("expected 5 pre-migration widgets for Comment (body, post_id, author_id, post_id, author_id), got %d", len(widgets))
	}

	if widgets[0].Name != "body" || widgets[0].Kind != view.WidgetTextarea {
		t.Errorf("unexpected widget 0: %+v", widgets[0])
	}
}

// TestView_HTTPRealServerGoldenSnapshot verifies via real HTTP server that /view/comments/create renders correct HTML.
func TestView_HTTPRealServerGoldenSnapshot(t *testing.T) {
	resourceDir := t.TempDir()
	dbPath := filepath.Join(resourceDir, "view_golden.db")

	commentYAML := `
resource:
  name: Comment
  timestamps: true
fields:
  - name: body
    type: markdown
    nullable: false
  - name: legacy_note
    type: string
    deprecated: true
relations:
  - name: post
    kind: belongs_to
    target: Post
    foreign_key: post_id
auth:
  permissions:
    create: public
    read: public
    update: public
    delete: public
`
	postYAML := `
resource:
  name: Post
  timestamps: true
fields:
  - name: title
    type: string
    nullable: false
auth:
  permissions:
    create: public
    read: public
    update: public
    delete: public
`
	if err := os.WriteFile(filepath.Join(resourceDir, "Comment.yaml"), []byte(commentYAML), 0644); err != nil {
		t.Fatalf("failed writing Comment.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resourceDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed writing Post.yaml: %v", err)
	}

	app, err := runtime.New(runtime.Config{
		ResourceDir: resourceDir,
		DBPath:      dbPath,
	})
	if err != nil {
		t.Fatalf("failed starting runtime: %v", err)
	}
	defer app.Close()

	ts := httptest.NewServer(app)
	defer ts.Close()

	// 1. GET /view/comments/create
	resp, err := ts.Client().Get(ts.URL + "/view/comments/create")
	if err != nil {
		t.Fatalf("GET /view/comments/create failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d: %s", resp.StatusCode, string(body))
	}

	html := string(body)

	// Verify derived FK post_id input is rendered as type="number"
	if !strings.Contains(html, `name="post_id"`) || !strings.Contains(html, `type="number"`) {
		t.Errorf("expected HTML form to contain post_id number input, got:\n%s", html)
	}

	// Verify deprecated legacy_note is NOT rendered in form
	if strings.Contains(html, `name="legacy_note"`) {
		t.Errorf("expected deprecated field legacy_note to be omitted from HTML form, got:\n%s", html)
	}
}
