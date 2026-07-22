package view_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/adapters/sqlite"
	"github.com/hitel00000/mold/resource"
	"github.com/hitel00000/mold/storage"
	"github.com/hitel00000/mold/transport"
	"github.com/hitel00000/mold/view"
)

func TestMarkdown_XSSSanitization(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		contains string
		rejected string
	}{
		{
			name:     "Script tag injection",
			input:    "# Hello\n<script>alert('xss')</script>",
			contains: "Hello",
			rejected: "<script>",
		},
		{
			name:     "Onerror attribute injection",
			input:    "<img src=\"x\" onerror=\"alert('xss')\">",
			contains: "img",
			rejected: "onerror",
		},
		{
			name:     "Javascript URI scheme injection",
			input:    "[Click me](javascript:alert('xss'))",
			contains: "Click me",
			rejected: "javascript:",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rendered := string(view.RenderMarkdown(tc.input))
			if tc.rejected != "" && strings.Contains(rendered, tc.rejected) {
				t.Errorf("expected rendered HTML to reject '%s', but found it in: %s", tc.rejected, rendered)
			}
			if tc.contains != "" && !strings.Contains(rendered, tc.contains) {
				t.Errorf("expected rendered HTML to contain '%s', but got: %s", tc.contains, rendered)
			}
		})
	}
}

func TestView_FKField_FormSubmission_E2E(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_view_e2e.db")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	postRes := &resource.Resource{
		Name:          "Post",
		Table:         "posts",
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false},
		},
	}

	commentRes := &resource.Resource{
		Name:          "Comment",
		Table:         "comments",
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "body", Type: resource.TypeText, Nullable: false},
		},
		Relations: []resource.Relation{
			{
				Name:       "post",
				Kind:       resource.KindBelongsTo,
				Target:     "Post",
				ForeignKey: "post_id",
			},
		},
	}

	ctx := t.Context()
	if err := store.EnsureSchema(ctx, postRes); err != nil {
		t.Fatalf("failed to ensure schema for Post: %v", err)
	}
	if err := store.EnsureSchema(ctx, commentRes); err != nil {
		t.Fatalf("failed to ensure schema for Comment: %v", err)
	}

	// 1. Create a parent Post record first
	postRecord, err := store.Create(ctx, postRes, map[string]any{"title": "Parent Post"})
	if err != nil {
		t.Fatalf("failed to create parent post: %v", err)
	}
	postID := postRecord["id"]

	reg := transport.NewRegistry()
	reg.Register(postRes, store)
	reg.Register(commentRes, store)

	router := transport.NewRouter(reg)
	vh, err := view.NewViewHandler(router)
	if err != nil {
		t.Fatalf("failed to create view handler: %v", err)
	}

	ts := httptest.NewServer(vh)
	defer ts.Close()

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	// 2. Submit Create Comment Form with post_id FK field
	formValues := url.Values{}
	formValues.Set("body", "This is a comment referencing post")
	formValues.Set("post_id", toStringVal(postID))

	resp, err := client.PostForm(ts.URL+"/view/comments/create", formValues)
	if err != nil {
		t.Fatalf("failed to submit comment create form: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("expected 303 SeeOther redirect after form submit, got %d", resp.StatusCode)
	}

	// 3. Verify Comment record actually exists in DB store with foreign_key post_id
	comments, err := store.List(ctx, commentRes, storage.Query{})
	if err != nil {
		t.Fatalf("failed to list comments from store: %v", err)
	}

	if len(comments) != 1 {
		t.Fatalf("expected 1 comment created in store, got %d", len(comments))
	}

	comm := comments[0]
	if comm["body"] != "This is a comment referencing post" {
		t.Errorf("expected comment body to match, got %v", comm["body"])
	}

	// Verify post_id FK field was preserved and properly parsed
	if toStringVal(comm["post_id"]) != toStringVal(postID) {
		t.Errorf("expected comment post_id to be %v, got %v", postID, comm["post_id"])
	}
}

func toStringVal(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
