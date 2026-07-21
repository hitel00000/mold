package resource_test

import (
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := resource.NewRegistry()
	res := &resource.Resource{Name: "Post", Table: "posts"}

	err := reg.Register(res)
	if err != nil {
		t.Fatalf("unexpected error registering resource: %v", err)
	}

	got, ok := reg.Get("Post")
	if !ok {
		t.Fatalf("expected resource 'Post' to be found")
	}
	if got.Table != "posts" {
		t.Errorf("expected Table 'posts', got '%s'", got.Table)
	}

	// Registering duplicate should fail
	err = reg.Register(res)
	if err == nil {
		t.Errorf("expected error when registering duplicate resource, got nil")
	}
}

func TestLoadAll_ExamplesDir(t *testing.T) {
	dir := filepath.Join("..", "examples")
	reg, err := resource.LoadAll(dir)
	if err != nil {
		t.Fatalf("unexpected error running LoadAll on examples dir: %v", err)
	}

	res, ok := reg.Get("Post")
	if !ok {
		t.Fatalf("expected resource 'Post' in registry")
	}
	if res.Name != "Post" {
		t.Errorf("expected resource name 'Post', got '%s'", res.Name)
	}
}
