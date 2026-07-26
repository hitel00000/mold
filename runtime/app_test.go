package runtime_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/runtime"
	"github.com/hitel00000/mold/view"
)

func TestNew_ConfigValidationErrors(t *testing.T) {
	tests := []struct {
		name    string
		cfg     runtime.Config
		wantErr string
	}{
		{
			name: "missing ResourceDir",
			cfg: runtime.Config{
				ResourceDir: "",
				DBPath:      "test.db",
			},
			wantErr: "Config.ResourceDir is required",
		},
		{
			name: "missing DBPath",
			cfg: runtime.Config{
				ResourceDir: t.TempDir(),
				DBPath:      "",
			},
			wantErr: "Config.DBPath is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app, err := runtime.New(tt.cfg)
			if err == nil {
				if app != nil {
					_ = app.Close()
				}
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
		})
	}
}

func TestNew_InvalidResourceDirOrYAML(t *testing.T) {
	t.Run("non-existent resource dir", func(t *testing.T) {
		cfg := runtime.Config{
			ResourceDir: filepath.Join(t.TempDir(), "non_existent"),
			DBPath:      filepath.Join(t.TempDir(), "test.db"),
		}
		app, err := runtime.New(cfg)
		if err == nil {
			_ = app.Close()
			t.Fatal("expected error for non-existent resource dir, got nil")
		}
	})

	t.Run("invalid YAML in resource dir", func(t *testing.T) {
		resDir := t.TempDir()
		invalidYAML := `
resource:
  name: Invalid
fields:
  - name: title
    type: invalid_type
`
		if err := os.WriteFile(filepath.Join(resDir, "Invalid.yaml"), []byte(invalidYAML), 0644); err != nil {
			t.Fatalf("failed to write invalid YAML: %v", err)
		}

		cfg := runtime.Config{
			ResourceDir: resDir,
			DBPath:      filepath.Join(t.TempDir(), "test.db"),
		}
		app, err := runtime.New(cfg)
		if err == nil {
			_ = app.Close()
			t.Fatal("expected error for invalid YAML resource, got nil")
		}
	})
}

func TestNew_SuccessPaths(t *testing.T) {
	postYAML := `
resource:
  name: Post
  timestamps: true
  soft_delete: true
fields:
  - name: title
    type: string
    nullable: false
  - name: body
    type: markdown
    nullable: false
`
	resDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(resDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed to write Post.yaml: %v", err)
	}

	t.Run("success without BlobDir and Overrides", func(t *testing.T) {
		cfg := runtime.Config{
			ResourceDir: resDir,
			DBPath:      filepath.Join(t.TempDir(), "app.db"),
		}
		app, err := runtime.New(cfg)
		if err != nil {
			t.Fatalf("expected New() success, got: %v", err)
		}
		if app.Store() == nil {
			t.Error("expected store to be non-nil")
		}
		if err := app.Close(); err != nil {
			t.Errorf("failed to close app: %v", err)
		}
	})

	t.Run("success with BlobDir and Overrides", func(t *testing.T) {
		blobDir := t.TempDir()
		overrides := view.NewTemplateOverrides()
		cfg := runtime.Config{
			ResourceDir: resDir,
			DBPath:      filepath.Join(t.TempDir(), "app_blob.db"),
			BlobDir:     blobDir,
			Overrides:   overrides,
		}
		app, err := runtime.New(cfg)
		if err != nil {
			t.Fatalf("expected New() success, got: %v", err)
		}
		if err := app.Close(); err != nil {
			t.Errorf("failed to close app: %v", err)
		}
	})
}

func TestApp_CreateRecord(t *testing.T) {
	postYAML := `
resource:
  name: Post
  timestamps: true
  soft_delete: true
fields:
  - name: title
    type: string
    nullable: false
  - name: body
    type: markdown
    nullable: false
`
	resDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(resDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed to write Post.yaml: %v", err)
	}

	cfg := runtime.Config{
		ResourceDir: resDir,
		DBPath:      filepath.Join(t.TempDir(), "createrecord.db"),
	}
	app, err := runtime.New(cfg)
	if err != nil {
		t.Fatalf("failed to build runtime app: %v", err)
	}
	defer app.Close()

	ctx := t.Context()

	t.Run("success via resource name", func(t *testing.T) {
		record, err := app.CreateRecord(ctx, "Post", map[string]any{
			"title": "First Post",
			"body":  "Content of first post",
		})
		if err != nil {
			t.Fatalf("expected CreateRecord success, got error: %v", err)
		}
		if record["id"] == nil {
			t.Error("expected created record to have id")
		}
		if record["title"] != "First Post" {
			t.Errorf("expected title 'First Post', got %v", record["title"])
		}
	})

	t.Run("success via table name", func(t *testing.T) {
		record, err := app.CreateRecord(ctx, "posts", map[string]any{
			"title": "Second Post",
			"body":  "Content of second post",
		})
		if err != nil {
			t.Fatalf("expected CreateRecord success via table name, got error: %v", err)
		}
		if record["title"] != "Second Post" {
			t.Errorf("expected title 'Second Post', got %v", record["title"])
		}
	})

	t.Run("non-existent table error path (schema existence check)", func(t *testing.T) {
		_, err := app.CreateRecord(ctx, "NonExistent", map[string]any{
			"name": "foo",
		})
		if err == nil {
			t.Fatal("expected error for non-existent table, got nil")
		}
		expectedErr := `resource definition for table/resource "NonExistent" not found`
		if !strings.Contains(err.Error(), expectedErr) {
			t.Errorf("expected error containing %q, got: %v", expectedErr, err)
		}
	})

	t.Run("record validation failure path", func(t *testing.T) {
		// Missing non-nullable "body" field
		_, err := app.CreateRecord(ctx, "Post", map[string]any{
			"title": "Incomplete Post",
		})
		if err == nil {
			t.Fatal("expected validation error for missing body, got nil")
		}
	})
}
