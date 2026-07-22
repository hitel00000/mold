package resource_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/resource"
)

func TestValidate_LoadTimeStressFailureCases(t *testing.T) {
	tests := []struct {
		name          string
		yamlContent   string
		expectedErrSub string
	}{
		{
			name: "Invalid permissions action value",
			yamlContent: `
resource:
  name: BadPermPost
fields:
  - name: title
    type: string
auth:
  permissions:
    update: invalid_perm_value
`,
			expectedErrSub: "invalid permission spec 'invalid_perm_value'",
		},
		{
			name: "Ownership field missing in fields",
			yamlContent: `
resource:
  name: BadOwnerPost
fields:
  - name: title
    type: string
auth:
  ownership_field: non_existent_author_id
  permissions:
    update: owner
`,
			expectedErrSub: "auth ownership_field 'non_existent_author_id' does not exist in fields",
		},
		{
			name: "Password field with unique constraint",
			yamlContent: `
resource:
  name: BadPasswordUser
fields:
  - name: secret
    type: password
    constraints:
      unique: true
`,
			expectedErrSub: "type password cannot have unique constraint",
		},
		{
			name: "Enum field missing values constraint",
			yamlContent: `
resource:
  name: BadEnumPost
fields:
  - name: status
    type: enum
`,
			expectedErrSub: "enum field 'status' requires constraint values",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := resource.Load([]byte(tt.yamlContent))
			if err != nil {
				// Parse error
				if !strings.Contains(err.Error(), tt.expectedErrSub) {
					t.Fatalf("unexpected parse error: %v, expected containing %q", err, tt.expectedErrSub)
				}
				return
			}

			err = resource.Validate(res)
			if err == nil {
				t.Fatalf("expected validation error containing %q, got nil", tt.expectedErrSub)
			}
			if !strings.Contains(err.Error(), tt.expectedErrSub) {
				t.Errorf("expected error containing %q, got %q", tt.expectedErrSub, err.Error())
			}
		})
	}
}

func TestLoadAll_NonExistentRelationTarget(t *testing.T) {
	tmpDir := t.TempDir()

	yamlContent := `
resource:
  name: Comment
fields:
  - name: body
    type: text
relations:
  - name: post
    kind: belongs_to
    target: NonExistentPost
    foreign_key: post_id
`
	if err := os.WriteFile(filepath.Join(tmpDir, "Comment.yaml"), []byte(yamlContent), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	_, err := resource.LoadAll(tmpDir)
	if err == nil {
		t.Fatalf("expected error for non-existent relation target, got nil")
	}
	if !strings.Contains(err.Error(), "target resource 'NonExistentPost' does not exist") {
		t.Errorf("expected error mentioning target resource, got: %v", err)
	}
}
