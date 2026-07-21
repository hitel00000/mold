package resource_test

import (
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestLoadFromFile_Post(t *testing.T) {
	path := filepath.Join("..", "examples", "post.yaml")
	r, err := resource.LoadFromFile(path)
	if err != nil {
		t.Fatalf("unexpected error loading post.yaml: %v", err)
	}

	if r.Name != "Post" {
		t.Errorf("expected Name 'Post', got '%s'", r.Name)
	}
	if r.Table != "posts" {
		t.Errorf("expected Table 'posts', got '%s'", r.Table)
	}
	if r.SchemaVersion != 1 {
		t.Errorf("expected SchemaVersion 1, got %d", r.SchemaVersion)
	}
	if !r.Timestamps {
		t.Errorf("expected Timestamps true, got false")
	}
	if !r.SoftDelete {
		t.Errorf("expected SoftDelete true, got false")
	}
	if len(r.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(r.Fields))
	}

	fieldMap := make(map[string]resource.Field)
	for _, f := range r.Fields {
		fieldMap[f.Name] = f
	}

	title, ok := fieldMap["title"]
	if !ok {
		t.Errorf("expected field 'title' not found")
	} else if title.Type != resource.TypeString {
		t.Errorf("expected title type 'string', got '%s'", title.Type)
	}

	body, ok := fieldMap["body"]
	if !ok {
		t.Errorf("expected field 'body' not found")
	} else if body.Type != resource.TypeMarkdown {
		t.Errorf("expected body type 'markdown', got '%s'", body.Type)
	}
}
