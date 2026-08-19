package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestMoldCLI_VersionAndHelp(t *testing.T) {
	// 1. Test version flag
	cmdVer := exec.Command("go", "run", "./cmd/mold", "version")
	cmdVer.Dir = filepath.Join("..", "..")
	outVer, err := cmdVer.CombinedOutput()
	if err != nil {
		t.Fatalf("mold version failed: %v, output: %s", err, string(outVer))
	}
	if !strings.Contains(string(outVer), "Mold v") {
		t.Errorf("expected version string, got: %s", string(outVer))
	}

	// 2. Test help flag
	cmdHelp := exec.Command("go", "run", "./cmd/mold", "help")
	cmdHelp.Dir = filepath.Join("..", "..")
	outHelp, err := cmdHelp.CombinedOutput()
	if err != nil {
		t.Fatalf("mold help failed: %v, output: %s", err, string(outHelp))
	}
	if !strings.Contains(string(outHelp), "Usage:") || !strings.Contains(string(outHelp), "codegen") {
		t.Errorf("expected usage instructions in help, got: %s", string(outHelp))
	}
}

func TestMoldCLI_Codegen_CloudflareE2E(t *testing.T) {
	tmpDir := t.TempDir()
	resDir := filepath.Join(tmpDir, "resources")
	outDir := filepath.Join(tmpDir, "dist")
	_ = os.MkdirAll(resDir, 0755)

	// Create sample Post.yaml and Comment.yaml
	postYAML := `resource:
  name: Post

fields:
  - name: title
    type: string
  - name: content
    type: text
  - name: author_id
    type: int
  - name: cover_image
    type: blob

auth:
  ownership_field: author_id
  permissions:
    create: authenticated
    read: public
    update: owner
    delete: owner
`

	commentYAML := `resource:
  name: Comment

fields:
  - name: body
    type: text
  - name: post_id
    type: int

relations:
  - name: post
    kind: belongs_to
    target: Post
    foreign_key: post_id
`

	if err := os.WriteFile(filepath.Join(resDir, "Post.yaml"), []byte(postYAML), 0644); err != nil {
		t.Fatalf("failed to write Post.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(resDir, "Comment.yaml"), []byte(commentYAML), 0644); err != nil {
		t.Fatalf("failed to write Comment.yaml: %v", err)
	}

	outTS := filepath.Join(outDir, "mold_app.ts")
	outSQL := filepath.Join(outDir, "schema.sql")

	// Execute: mold codegen -d <resDir> -o <outTS> --schema-out <outSQL>
	cmdCodegen := exec.Command(
		"go", "run", "./cmd/mold",
		"codegen",
		"--target", "cloudflare",
		"--dir", resDir,
		"--out", outTS,
		"--schema-out", outSQL,
	)
	cmdCodegen.Dir = filepath.Join("..", "..")
	outCodegen, err := cmdCodegen.CombinedOutput()
	if err != nil {
		t.Fatalf("mold codegen command failed: %v, output: %s", err, string(outCodegen))
	}
	t.Logf("Codegen output:\n%s", string(outCodegen))

	// Verify TS file was generated and contains expected endpoints and imports
	tsBytes, err := os.ReadFile(outTS)
	if err != nil {
		t.Fatalf("failed to read generated TS file: %v", err)
	}
	tsCode := string(tsBytes)

	if !strings.Contains(tsCode, "import { Hono } from 'hono'") {
		t.Errorf("expected Hono import in generated TS code")
	}
	if !strings.Contains(tsCode, "app.get('/api/posts'") || !strings.Contains(tsCode, "app.post('/api/posts'") {
		t.Errorf("expected /api/posts endpoints in generated TS code")
	}
	if !strings.Contains(tsCode, "app.get('/api/comments'") {
		t.Errorf("expected /api/comments endpoint in generated TS code")
	}

	// Verify Schema SQL was generated
	sqlBytes, err := os.ReadFile(outSQL)
	if err != nil {
		t.Fatalf("failed to read generated Schema SQL: %v", err)
	}
	sqlCode := string(sqlBytes)
	if !strings.Contains(sqlCode, `CREATE TABLE IF NOT EXISTS "posts"`) {
		t.Errorf("expected CREATE TABLE posts in generated schema SQL")
	}
	if !strings.Contains(sqlCode, `CREATE TABLE IF NOT EXISTS "comments"`) {
		t.Errorf("expected CREATE TABLE comments in generated schema SQL")
	}
}

func TestMoldCLI_Codegen_InvalidDirectory(t *testing.T) {
	cmd := exec.Command("go", "run", "./cmd/mold", "codegen", "--dir", "./non_existent_directory_12345")
	cmd.Dir = filepath.Join("..", "..")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected codegen to fail on non-existent dir, but succeeded, output: %s", string(out))
	}
	if !strings.Contains(string(out), "Error loading resources") && !strings.Contains(string(out), "No YAML resources") {
		t.Logf("Output as expected: %s", string(out))
	}
}
