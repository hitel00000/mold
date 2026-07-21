package resource_test

import (
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestRelationStressTest_PostAndComment(t *testing.T) {
	dir := filepath.Join("..", "examples")
	reg, err := resource.LoadAll(dir)
	if err != nil {
		t.Fatalf("failed to LoadAll from examples dir: %v", err)
	}

	postRes, ok := reg.Get("Post")
	if !ok {
		t.Fatalf("expected resource 'Post' in registry")
	}

	commentRes, ok := reg.Get("Comment")
	if !ok {
		t.Fatalf("expected resource 'Comment' in registry")
	}

	// Verify Post relations
	if len(postRes.Relations) != 1 {
		t.Fatalf("expected 1 relation in Post, got %d", len(postRes.Relations))
	}
	postRel := postRes.Relations[0]
	if postRel.Name != "comments" {
		t.Errorf("expected relation name 'comments', got '%s'", postRel.Name)
	}
	if postRel.Kind != resource.KindHasMany {
		t.Errorf("expected relation kind 'has_many', got '%s'", postRel.Kind)
	}
	if postRel.Target != "Comment" {
		t.Errorf("expected relation target 'Comment', got '%s'", postRel.Target)
	}
	if postRel.ForeignKey != "post_id" {
		t.Errorf("expected foreign_key 'post_id', got '%s'", postRel.ForeignKey)
	}
	if postRel.OnDelete != resource.OnDeleteRestrict {
		t.Errorf("expected on_delete 'restrict', got '%s'", postRel.OnDelete)
	}

	// Verify Comment relations
	if len(commentRes.Relations) != 1 {
		t.Fatalf("expected 1 relation in Comment, got %d", len(commentRes.Relations))
	}
	commentRel := commentRes.Relations[0]
	if commentRel.Name != "post" {
		t.Errorf("expected relation name 'post', got '%s'", commentRel.Name)
	}
	if commentRel.Kind != resource.KindBelongsTo {
		t.Errorf("expected relation kind 'belongs_to', got '%s'", commentRel.Kind)
	}
	if commentRel.Target != "Post" {
		t.Errorf("expected relation target 'Post', got '%s'", commentRel.Target)
	}
	if commentRel.ForeignKey != "post_id" {
		t.Errorf("expected foreign_key 'post_id', got '%s'", commentRel.ForeignKey)
	}

	// Cross-reference checks
	targetFromPost, exists := reg.Get(postRel.Target)
	if !exists || targetFromPost.Name != "Comment" {
		t.Errorf("target resource '%s' from Post not correctly resolved in registry", postRel.Target)
	}

	targetFromComment, exists := reg.Get(commentRel.Target)
	if !exists || targetFromComment.Name != "Post" {
		t.Errorf("target resource '%s' from Comment not correctly resolved in registry", commentRel.Target)
	}
}
