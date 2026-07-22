package view_test

import (
	"fmt"
	"io"
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

func TestView_FormValidationErrorHandling_E2E(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_view_validation.db")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	minLen := 3
	postRes := &resource.Resource{
		Name:          "Post",
		Table:         "posts",
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
		Fields: []resource.Field{
			{
				Name:     "title",
				Type:     resource.TypeString,
				Nullable: false,
				Constraints: resource.Constraints{
					MinLength: &minLen,
				},
			},
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

	client := ts.Client()

	// 1. Submit invalid short title ("ab" < min_length 3)
	formValues := url.Values{}
	formValues.Set("title", "ab")

	resp, err := client.PostForm(ts.URL+"/view/posts/create", formValues)
	if err != nil {
		t.Fatalf("failed to submit post create form: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for validation error, got %d", resp.StatusCode)
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}
	htmlOutput := string(bodyBytes)

	if !strings.Contains(htmlOutput, "Validation failed for field &#39;title&#39;") && !strings.Contains(htmlOutput, "Validation failed for field 'title'") {
		t.Errorf("expected HTML output to render field validation error summary, got: %s", htmlOutput)
	}

	if !strings.Contains(htmlOutput, "length 2 is less than min_length 3") {
		t.Errorf("expected HTML output to contain specific error message 'length 2 is less than min_length 3', got: %s", htmlOutput)
	}

	if !strings.Contains(htmlOutput, "value=\"ab\"") {
		t.Errorf("expected user input value 'ab' to be preserved in form value attribute, got: %s", htmlOutput)
	}

	// 2. Submit non-existent FK post_id = 9999 for Comment Create
	commValues := url.Values{}
	commValues.Set("body", "Comment for missing post")
	commValues.Set("post_id", "9999")

	resp2, err := client.PostForm(ts.URL+"/view/comments/create", commValues)
	if err != nil {
		t.Fatalf("failed to submit comment create form: %v", err)
	}
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request for foreign key error, got %d", resp2.StatusCode)
	}

	bodyBytes2, err := io.ReadAll(resp2.Body)
	if err != nil {
		t.Fatalf("failed to read response body 2: %v", err)
	}
	htmlOutput2 := string(bodyBytes2)

	if !strings.Contains(htmlOutput2, "Referenced foreign key target record does not exist") {
		t.Errorf("expected HTML output to contain foreign key error message, got: %s", htmlOutput2)
	}
}

func toStringVal(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}
