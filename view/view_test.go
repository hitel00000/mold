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
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	commentRes := &resource.Resource{
		Name:          "Comment",
		Table:         "comments",
		SchemaVersion: 1,
		Timestamps:    true,
		SoftDelete:    true,
		Fields: []resource.Field{
			{Name: "body", Type: resource.TypeText, Nullable: false, ClientWritable: true},
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
	vh, err := view.NewViewHandler(router, nil)
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
				Name:           "title",
				Type:           resource.TypeString,
				Nullable:       false,
				ClientWritable: true,
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
			{Name: "body", Type: resource.TypeText, Nullable: false, ClientWritable: true},
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
	vh, err := view.NewViewHandler(router, nil)
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

func TestFormValue_ReflectedXSS_AutoEscaping(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_reflected_xss.db")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	minLen := 100
	postRes := &resource.Resource{
		Name:          "Post",
		Table:         "posts",
		SchemaVersion: 1,
		Fields: []resource.Field{
			{
				Name:           "title",
				Type:           resource.TypeString,
				Nullable:       false,
				ClientWritable: true,
				Constraints: resource.Constraints{
					MinLength: &minLen,
				},
			},
		},
	}

	ctx := t.Context()
	if err := store.EnsureSchema(ctx, postRes); err != nil {
		t.Fatalf("failed to ensure schema: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(postRes, store)

	router := transport.NewRouter(reg)
	vh, err := view.NewViewHandler(router, nil)
	if err != nil {
		t.Fatalf("failed to create view handler: %v", err)
	}

	ts := httptest.NewServer(vh)
	defer ts.Close()

	// Submit reflected XSS payload in title field
	xssPayload := `"><script>alert('reflected-xss')</script>`
	formValues := url.Values{}
	formValues.Set("title", xssPayload)

	resp, err := ts.Client().PostForm(ts.URL+"/view/posts/create", formValues)
	if err != nil {
		t.Fatalf("failed to submit form: %v", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response: %v", err)
	}
	htmlOutput := string(bodyBytes)

	// Verify raw <script> tag is NOT present unescaped in HTML
	if strings.Contains(htmlOutput, "<script>alert('reflected-xss')</script>") {
		t.Errorf("SECURITY RISK: Reflected XSS payload found unescaped in HTML output!")
	}

	// Verify contextual auto-escaping is applied (e.g. &#34; or &quot;)
	if !strings.Contains(htmlOutput, "&#34;&gt;&lt;script&gt;") && !strings.Contains(htmlOutput, "&quot;&gt;&lt;script&gt;") {
		t.Errorf("expected value attribute to contain auto-escaped XSS payload, got: %s", htmlOutput)
	}
}

func toStringVal(v any) string {
	if v == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprintf("%v", v))
}

func TestView_RelationInclude_E2E(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_view_include.db")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	tagRes := &resource.Resource{
		Name:  "Tag",
		Table: "tags",
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
	}

	recordTagRes := &resource.Resource{
		Name:  "RecordTag",
		Table: "record_tags",
		Fields: []resource.Field{
			{Name: "tag_id", Type: resource.TypeInt, Nullable: true, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "tag", Kind: resource.KindBelongsTo, Target: "Tag", ForeignKey: "tag_id"},
		},
	}

	ctx := t.Context()
	_ = store.EnsureSchema(ctx, tagRes)
	_ = store.EnsureSchema(ctx, recordTagRes)

	tagRec, err := store.Create(ctx, tagRes, storage.Record{"name": "Ginjo Premium Tag"})
	if err != nil {
		t.Fatalf("failed to create tag: %v", err)
	}

	_, err = store.Create(ctx, recordTagRes, storage.Record{"tag_id": tagRec["id"]})
	if err != nil {
		t.Fatalf("failed to create record_tag: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(tagRes, store)
	reg.Register(recordTagRes, store)

	router := transport.NewRouter(reg)
	vh, err := view.NewViewHandler(router, nil)
	if err != nil {
		t.Fatalf("failed to create view handler: %v", err)
	}

	ts := httptest.NewServer(vh)
	defer ts.Close()

	// 1. SSR View List with ?include=tag
	resp, err := ts.Client().Get(ts.URL + "/view/record_tags?include=tag")
	if err != nil {
		t.Fatalf("failed request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	listHtmlBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	listHtml := string(listHtmlBytes)

	// Verify SSR List HTML contains table and records without error
	if !strings.Contains(listHtml, "RecordTag List") {
		t.Errorf("expected list HTML to render title RecordTag List, got:\n%s", listHtml)
	}

	// 2. SSR View Detail with ?include=tag
	respDetail, err := ts.Client().Get(ts.URL + "/view/record_tags/1?include=tag")
	if err != nil {
		t.Fatalf("failed detail request: %v", err)
	}
	if respDetail.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for detail view, got %d", respDetail.StatusCode)
	}
	detailHtmlBytes, _ := io.ReadAll(respDetail.Body)
	respDetail.Body.Close()
	detailHtml := string(detailHtmlBytes)

	if !strings.Contains(detailHtml, "RecordTag #1") {
		t.Errorf("expected detail HTML to render RecordTag #1, got:\n%s", detailHtml)
	}

	// 3. Test SSR View custom template rendering embedded relation field {{.tag.name}}
	overrides := view.NewTemplateOverrides()
	_ = overrides.SetCustomTemplateString("record_tags", "list", `{{define "content"}}{{range .Records}}TAG:{{if .tag}}{{.tag.name}}{{end}}{{end}}{{end}}`)

	vhCustom, _ := view.NewViewHandler(router, overrides)
	tsCustom := httptest.NewServer(vhCustom)
	defer tsCustom.Close()

	respCustom, err := tsCustom.Client().Get(tsCustom.URL + "/view/record_tags?include=tag")
	if err != nil {
		t.Fatalf("failed custom request: %v", err)
	}
	defer respCustom.Body.Close()
	customHtmlBytes, _ := io.ReadAll(respCustom.Body)
	customHtml := string(customHtmlBytes)

	if !strings.Contains(customHtml, "TAG:Ginjo Premium Tag") {
		t.Errorf("expected custom SSR view to render embedded relation tag.name 'Ginjo Premium Tag', got: %s", customHtml)
	}
}

func TestViewHandler_RenderLogin_EmailLabel(t *testing.T) {
	userRes := &resource.Resource{
		Name:          "User",
		Table:         "users",
		SchemaVersion: 1,
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false, ClientWritable: true},
			{Name: "password", Type: resource.TypePassword, Nullable: false, ClientWritable: true},
		},
	}

	dbPath := filepath.Join(t.TempDir(), "login_label.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed sqlite.Open: %v", err)
	}
	defer store.Close()

	if err := store.EnsureSchema(t.Context(), userRes); err != nil {
		t.Fatalf("failed EnsureSchema: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(userRes, store)

	router := transport.NewRouter(reg)
	vh, err := view.NewViewHandler(router, nil)
	if err != nil {
		t.Fatalf("failed NewViewHandler: %v", err)
	}

	req, _ := http.NewRequest(http.MethodGet, "/login", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 OK for GET /login, got %d", w.Code)
	}

	htmlBody := w.Body.String()
	if !strings.Contains(htmlBody, `<label for="username">Email</label>`) {
		t.Errorf("expected HTML to contain '<label for=\"username\">Email</label>', got:\n%s", htmlBody)
	}
	if !strings.Contains(htmlBody, `placeholder="Enter your email address"`) {
		t.Errorf("expected HTML to contain email placeholder, got:\n%s", htmlBody)
	}
}

func TestBuildFormFields_ClientWritable(t *testing.T) {
	res := &resource.Resource{
		Name: "User",
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false, ClientWritable: true},
			{Name: "badge", Type: resource.TypeString, Nullable: false, Default: "bronze", ClientWritable: false},
		},
	}

	widgets := view.BuildFormFields(res, nil, false)
	if len(widgets) != 1 {
		t.Fatalf("expected 1 widget (badge excluded), got %d", len(widgets))
	}
	if widgets[0].Name != "email" {
		t.Errorf("expected widget for email, got %s", widgets[0].Name)
	}
}

func TestView_ClientWritable_FormSubmission_E2E(t *testing.T) {
	userRes := &resource.Resource{
		Name:          "User",
		Table:         "users",
		SchemaVersion: 1,
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false, ClientWritable: true},
			{Name: "badge", Type: resource.TypeString, Nullable: false, Default: "bronze", ClientWritable: false},
		},
	}

	dbPath := filepath.Join(t.TempDir(), "client_writable_form.db")
	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed sqlite.Open: %v", err)
	}
	defer store.Close()

	if err := store.EnsureSchema(t.Context(), userRes); err != nil {
		t.Fatalf("failed EnsureSchema: %v", err)
	}

	reg := transport.NewRegistry()
	reg.Register(userRes, store)

	router := transport.NewRouter(reg)
	vh, err := view.NewViewHandler(router, nil)
	if err != nil {
		t.Fatalf("failed NewViewHandler: %v", err)
	}

	ts := httptest.NewServer(vh)
	defer ts.Close()

	// 1. Submit form WITH badge=gold (simulating malicious form tampering)
	formValues := url.Values{}
	formValues.Set("email", "hacker@example.com")
	formValues.Set("badge", "gold")

	resp, err := ts.Client().PostForm(ts.URL+"/view/users/create", formValues)
	if err != nil {
		t.Fatalf("failed to post form: %v", err)
	}
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	htmlOutput := string(bodyBytes)

	t.Logf("[RAW VIEW FORM PROOF 1 - Tampered HTML Form Submission Rejection]:")
	t.Logf("  POST /view/users/create Form Data: %v", formValues)
	t.Logf("  Response Status: %d %s", resp.StatusCode, http.StatusText(resp.StatusCode))
	t.Logf("  Response HTML Snippet: %s", htmlOutput)

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected 400 Bad Request when badge is sent in form, got %d", resp.StatusCode)
	}

	if !strings.Contains(htmlOutput, "is not client-writable") {
		t.Errorf("expected HTML error to contain 'is not client-writable', got:\n%s", htmlOutput)
	}

	// 2. Submit form WITHOUT badge (normal user submission)
	normalValues := url.Values{}
	normalValues.Set("email", "normal@example.com")

	client := &http.Client{
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp2, err := client.PostForm(ts.URL+"/view/users/create", normalValues)
	if err != nil {
		t.Fatalf("failed normal post form: %v", err)
	}
	body2Bytes, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()

	t.Logf("[RAW VIEW FORM PROOF 2 - Normal HTML Form Submission Success]:")
	t.Logf("  POST /view/users/create Form Data: %v", normalValues)
	t.Logf("  Response Status: %d %s", resp2.StatusCode, http.StatusText(resp2.StatusCode))
	t.Logf("  Response Headers (Location): %s", resp2.Header.Get("Location"))
	_ = body2Bytes

	if resp2.StatusCode != http.StatusSeeOther {
		t.Errorf("expected 303 SeeOther redirect for normal submission, got %d", resp2.StatusCode)
	}

	// Verify in DB that badge defaulted to "bronze"
	records, err := store.List(t.Context(), userRes, storage.Query{})
	if err != nil || len(records) != 1 {
		t.Fatalf("expected 1 record created in DB, got %d (err: %v)", len(records), err)
	}
	if records[0]["badge"] != "bronze" {
		t.Errorf("expected badge to default to 'bronze', got %v", records[0]["badge"])
	}
}

func TestView_HasMany_IncludeE2E(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test_view_has_many.db")

	store, err := sqlite.Open(dbPath)
	if err != nil {
		t.Fatalf("failed to open sqlite store: %v", err)
	}
	defer store.Close()

	ctx := t.Context()

	postRes := &resource.Resource{
		Name:          "Post",
		Table:         "posts",
		SchemaVersion: 1,
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString, Nullable: false, ClientWritable: true},
		},
		Relations: []resource.Relation{
			{Name: "comments", Kind: resource.KindHasMany, Target: "Comment", ForeignKey: "post_id"},
		},
	}

	commentRes := &resource.Resource{
		Name:          "Comment",
		Table:         "comments",
		SchemaVersion: 1,
		Fields: []resource.Field{
			{Name: "post_id", Type: resource.TypeInt, Nullable: false, ClientWritable: true},
			{Name: "body", Type: resource.TypeText, Nullable: false, ClientWritable: true},
		},
	}

	_ = store.EnsureSchema(ctx, postRes)
	_ = store.EnsureSchema(ctx, commentRes)

	p1, _ := store.Create(ctx, postRes, storage.Record{"title": "View Post 1"})
	_, _ = store.Create(ctx, commentRes, storage.Record{"post_id": p1["id"], "body": "View Comment 101"})

	reg := transport.NewRegistry()
	reg.Register(postRes, store)
	reg.Register(commentRes, store)

	router := transport.NewRouter(reg)
	vh, err := view.NewViewHandler(router, nil)
	if err != nil {
		t.Fatalf("failed NewViewHandler: %v", err)
	}

	ts := httptest.NewServer(vh)
	defer ts.Close()

	// 1. Test GET /view/posts?include=comments
	resp, err := ts.Client().Get(ts.URL + "/view/posts?include=comments")
	if err != nil {
		t.Fatalf("failed GET /view/posts: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for SSR view with has_many include, got %d", resp.StatusCode)
	}

	// 2. Test GET /view/posts/:id?include=comments
	respDetail, err := ts.Client().Get(fmt.Sprintf("%s/view/posts/%v?include=comments", ts.URL, p1["id"]))
	if err != nil {
		t.Fatalf("failed GET detail: %v", err)
	}
	defer respDetail.Body.Close()
	if respDetail.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK for SSR detail view with has_many include, got %d", respDetail.StatusCode)
	}
}

