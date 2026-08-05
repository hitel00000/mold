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

func TestLoad_RejectsNonCanonicalSyntax(t *testing.T) {
	shortFormResource := []byte(`
resource: Post
fields:
  - name: title
    type: string
`)
	_, err := resource.Load(shortFormResource)
	if err == nil {
		t.Errorf("expected error when resource is a scalar string instead of mapping, got nil")
	}

	shortFormFields := []byte(`
resource:
  name: Post
fields:
  title:
    type: string
`)
	_, err = resource.Load(shortFormFields)
	if err == nil {
		t.Errorf("expected error when fields is a map instead of sequence, got nil")
	}
}

func TestLoad_ResourceConstraints(t *testing.T) {
	yamlData := []byte(`
resource:
  name: RecordTag
  table: record_tags

fields:
  - name: sake_record_id
    type: int
  - name: tag_id
    type: int

constraints:
  unique_together:
    - [sake_record_id, tag_id]
`)
	r, err := resource.Load(yamlData)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	if r.Constraints == nil || len(r.Constraints.UniqueTogether) != 1 {
		t.Fatalf("expected 1 unique_together constraint group, got: %v", r.Constraints)
	}

	group := r.Constraints.UniqueTogether[0]
	if len(group) != 2 || group[0] != "sake_record_id" || group[1] != "tag_id" {
		t.Errorf("unexpected unique_together content: %v", group)
	}
}

func TestLoad_ClientWritableDefaultAndExplicit(t *testing.T) {
	yamlData := []byte(`
resource:
  name: User

fields:
  - name: email
    type: email
  - name: role
    type: enum
    nullable: false
    default: "user"
    client_writable: false
    constraints:
      values: ["admin", "user"]
`)
	r, err := resource.Load(yamlData)
	if err != nil {
		t.Fatalf("unexpected load error: %v", err)
	}

	if len(r.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(r.Fields))
	}

	emailField := r.Fields[0]
	if emailField.Name != "email" || !emailField.ClientWritable {
		t.Errorf("expected field 'email' to have ClientWritable true by default, got %v", emailField.ClientWritable)
	}

	roleField := r.Fields[1]
	if roleField.Name != "role" || roleField.ClientWritable {
		t.Errorf("expected field 'role' to have ClientWritable false, got %v", roleField.ClientWritable)
	}
}

