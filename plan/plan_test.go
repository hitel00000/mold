package plan_test

import (
	"testing"

	"github.com/hitel00000/mold/plan"
	"github.com/hitel00000/mold/resource"
)

func TestPlan_BuildExplicitAndDerivedFK(t *testing.T) {
	res := &resource.Resource{
		Name:       "Comment",
		Table:      "comments",
		Timestamps: true,
		SoftDelete: true,
		Fields: []resource.Field{
			{
				Name:     "body",
				Type:     resource.TypeText,
				Nullable: false,
			},
		},
		Relations: []resource.Relation{
			{
				Name:       "post",
				Kind:       resource.KindBelongsTo,
				Target:     "Post",
				ForeignKey: "post_id",
			},
			{
				Name:       "replies",
				Kind:       resource.KindHasMany,
				Target:     "Comment",
				ForeignKey: "parent_comment_id",
			},
		},
	}

	p := plan.Build(res)
	if p == nil {
		t.Fatalf("expected non-nil plan")
	}

	if p.ResourceName != "Comment" || p.Table != "comments" {
		t.Errorf("unexpected plan name/table: %s / %s", p.ResourceName, p.Table)
	}

	// Should have 2 fields: body (explicit) and post_id (derived from belongs_to)
	if len(p.Fields) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(p.Fields))
	}

	if p.Fields[0].Name != "body" || p.Fields[0].IsDerivedFK {
		t.Errorf("expected explicit body field, got %+v", p.Fields[0])
	}

	if p.Fields[1].Name != "post_id" || !p.Fields[1].IsDerivedFK || p.Fields[1].Type != resource.TypeInt {
		t.Errorf("expected derived post_id FK field, got %+v", p.Fields[1])
	}
}

func TestPlan_BuildOmitDuplicateExplicitFK(t *testing.T) {
	res := &resource.Resource{
		Name:  "Post",
		Table: "posts",
		Fields: []resource.Field{
			{Name: "title", Type: resource.TypeString},
			{Name: "author_id", Type: resource.TypeInt},
		},
		Relations: []resource.Relation{
			{
				Name:       "author",
				Kind:       resource.KindBelongsTo,
				Target:     "User",
				ForeignKey: "author_id",
			},
		},
	}

	p := plan.Build(res)
	if len(p.Fields) != 2 {
		t.Fatalf("expected 2 fields (author_id not duplicated), got %d", len(p.Fields))
	}
	if p.Fields[1].Name != "author_id" || p.Fields[1].IsDerivedFK {
		t.Errorf("expected explicit author_id field retained, got %+v", p.Fields[1])
	}
}
