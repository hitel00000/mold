package cloudflare_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

var (
	sharedNodeOnce sync.Once
	sharedNodeDir  string
	sharedNodeErr  error
)

func getSharedNodeDir(t *testing.T) string {
	t.Helper()
	sharedNodeOnce.Do(func() {
		sharedNodeDir = filepath.Join(os.TempDir(), "mold_test_shared_node_modules")
		_ = os.MkdirAll(sharedNodeDir, 0755)

		nodeModulesPath := filepath.Join(sharedNodeDir, "node_modules")
		miniflarePkg := filepath.Join(nodeModulesPath, "miniflare", "package.json")
		honoPkg := filepath.Join(nodeModulesPath, "hono", "package.json")
		esbuildPkg := filepath.Join(nodeModulesPath, "esbuild", "package.json")

		// Check if already installed and healthy
		if _, err1 := os.Stat(miniflarePkg); err1 == nil {
			if _, err2 := os.Stat(honoPkg); err2 == nil {
				if _, err3 := os.Stat(esbuildPkg); err3 == nil {
					return
				}
			}
		}

		pkgJSON := `{"name":"mold-shared-test","type":"module","dependencies":{"miniflare":"^3.20241205.0","hono":"^4.7.0","esbuild":"^0.24.0"}}`
		if err := os.WriteFile(filepath.Join(sharedNodeDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
			sharedNodeErr = fmt.Errorf("failed writing shared package.json: %w", err)
			return
		}

		cmdNpm := exec.Command("npm.cmd", "install", "--no-audit", "--no-fund")
		if runtime.GOOS != "windows" {
			cmdNpm = exec.Command("npm", "install", "--no-audit", "--no-fund")
		}
		cmdNpm.Dir = sharedNodeDir
		if out, err := cmdNpm.CombinedOutput(); err != nil {
			sharedNodeErr = fmt.Errorf("shared npm install failed: %w, output: %s", err, string(out))
			return
		}
	})

	if sharedNodeErr != nil {
		t.Fatalf("failed initializing shared node_modules: %v", sharedNodeErr)
	}
	return sharedNodeDir
}

func linkSharedNodeModules(t *testing.T, targetDir string) {
	t.Helper()
	sharedDir := getSharedNodeDir(t)
	srcNodeModules := filepath.Join(sharedDir, "node_modules")
	dstNodeModules := filepath.Join(targetDir, "node_modules")

	_ = os.Remove(dstNodeModules)

	if runtime.GOOS == "windows" {
		cmd := exec.Command("cmd", "/C", "mklink", "/J", dstNodeModules, srcNodeModules)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("failed linking node_modules junction on windows: %v, output: %s", err, string(out))
		}
	} else {
		if err := os.Symlink(srcNodeModules, dstNodeModules); err != nil {
			t.Fatalf("failed creating symlink for node_modules: %v", err)
		}
	}
}
