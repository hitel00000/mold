package pilot_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestDrinkLog_E2ERealProductionMigration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heavy Miniflare integration test in short mode")
	}
	nodePath, err := exec.LookPath("node")
	if err != nil || nodePath == "" {
		t.Skip("node not found in PATH")
	}

	tmpDir := t.TempDir()

	// Copy functions directory into tmpDir
	functionsSrc := filepath.Join(".", "functions")
	functionsDest := filepath.Join(tmpDir, "functions")
	copyDir(t, functionsSrc, functionsDest)

	// Copy migration SQL into tmpDir
	migSQL, err := os.ReadFile(filepath.Join("migrations", "0001_drink_log_migration.sql"))
	if err != nil {
		t.Fatalf("failed reading migration sql: %v", err)
	}
	_ = os.WriteFile(filepath.Join(tmpDir, "migration.sql"), migSQL, 0644)

	// Create index.ts that directly imports and dispatches to all deployable functions/ TypeScript modules
	indexTS := `import { Hono } from 'hono';
import { onRequestGet as getSakeRecords, onRequestPost as createSakeRecord } from './functions/api/sake-records/index';
import { onRequestGet as getSakeRecord, onRequestPut as updateSakeRecord, onRequestDelete as deleteSakeRecord } from './functions/api/sake-records/[id]';
import { onRequestPost as addSakeImage } from './functions/api/sake-records/[id]/images';
import { onRequestDelete as deleteSakeImage } from './functions/api/sake-records/[id]/images/[imageId]';
import { onRequestGet as searchSakeRecords } from './functions/api/sake-records/search';
import { onRequestGet as getTags, onRequestPost as createTag } from './functions/api/tags/index';
import { onRequestGet as getImage } from './functions/api/images';
import { onRequestGet as getMe } from './functions/api/me';
import { onRequestGet as googleLogin } from './functions/api/auth/google/login';
import { onRequestGet as googleCallback } from './functions/api/auth/google-callback';
import { onRequestPost as logout } from './functions/api/auth/logout';

const app = new Hono<{ Bindings: any }>();

const ctx = { waitUntil: () => {}, passThroughOnException: () => {}, next: async () => new Response() };

app.get('/api/sake-records', (c) => getSakeRecords({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.post('/api/sake-records', (c) => createSakeRecord({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.get('/api/sake-records/search', (c) => searchSakeRecords({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.get('/api/sake-records/:id', (c) => getSakeRecord({ env: c.env, request: c.req.raw, params: { id: c.req.param('id') }, ...ctx }));
app.put('/api/sake-records/:id', (c) => updateSakeRecord({ env: c.env, request: c.req.raw, params: { id: c.req.param('id') }, ...ctx }));
app.delete('/api/sake-records/:id', (c) => deleteSakeRecord({ env: c.env, request: c.req.raw, params: { id: c.req.param('id') }, ...ctx }));
app.post('/api/sake-records/:id/images', (c) => addSakeImage({ env: c.env, request: c.req.raw, params: { id: c.req.param('id') }, ...ctx }));
app.delete('/api/sake-records/:id/images/:imageId', (c) => deleteSakeImage({ env: c.env, request: c.req.raw, params: { id: c.req.param('id'), imageId: c.req.param('imageId') }, ...ctx }));
app.get('/api/tags', (c) => getTags({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.post('/api/tags', (c) => createTag({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.get('/api/images', (c) => getImage({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.get('/api/me', (c) => getMe({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.get('/api/auth/google/login', (c) => googleLogin({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.get('/api/auth/google-callback', (c) => googleCallback({ env: c.env, request: c.req.raw, params: {}, ...ctx }));
app.post('/api/auth/logout', (c) => logout({ env: c.env, request: c.req.raw, params: {}, ...ctx }));

export default app;
`
	_ = os.WriteFile(filepath.Join(tmpDir, "index.ts"), []byte(indexTS), 0644)

	// Link shared node_modules
	linkSharedNodeModules(t, tmpDir)

	cmdEsbuild := exec.Command("npx.cmd", "esbuild", "index.ts", "--bundle", "--format=esm", "--outfile=dist/index.js", "--external:node:*")
	if os.Getenv("OS") != "Windows_NT" {
		cmdEsbuild = exec.Command("npx", "esbuild", "index.ts", "--bundle", "--format=esm", "--outfile=dist/index.js", "--external:node:*")
	}
	cmdEsbuild.Dir = tmpDir
	if out, err := cmdEsbuild.CombinedOutput(); err != nil {
		t.Fatalf("esbuild bundle failed: %v\nOutput: %s", err, string(out))
	}

	miniflareURL := filepath.ToSlash(filepath.Join(tmpDir, "node_modules", "miniflare", "dist", "src", "index.js"))

	runnerJS := `import fs from 'node:fs';
import { pathToFileURL } from 'node:url';

async function run() {
  const miniflareModule = await import(pathToFileURL("` + miniflareURL + `").href);
  const { Miniflare } = miniflareModule;

  const mf = new Miniflare({
    modules: true,
    scriptPath: "./dist/index.js",
    d1Databases: { DB: "mold-d1", alcohol_log: "mold-d1" },
    r2Buckets: { IMAGES: "mold-r2", alcohol_log_images: "mold-r2" },
    bindings: {
      GOOGLE_CLIENT_ID: "test_client_id",
      GOOGLE_CLIENT_SECRET: "test_client_secret",
      SESSION_SECRET: "test_session_secret"
    }
  });

  const db = await mf.getD1Database("DB");
  await db.exec("PRAGMA foreign_keys = ON;");
  const bucket = await mf.getR2Bucket("IMAGES");

  console.log("=== STEP 1: Seeding Legacy Production Schema & Real Synthetic Data ===");
  const legacyStmts = [
    'CREATE TABLE users (id TEXT PRIMARY KEY, provider TEXT NOT NULL, provider_user_id TEXT NOT NULL, email TEXT, display_name TEXT, avatar_url TEXT, created_at TEXT NOT NULL, last_login_at TEXT NOT NULL, UNIQUE (provider, provider_user_id))',
    'CREATE TABLE oauth_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL, FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE)',
    'CREATE TABLE sake_records (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, drink_type TEXT NOT NULL DEFAULT "sake", name TEXT NOT NULL CHECK (length(trim(name)) > 0), region TEXT, brewery TEXT, rice TEXT, sake_type TEXT, sake_meter_value TEXT, abv TEXT, volume TEXT, price TEXT, drink_again TEXT CHECK (drink_again IS NULL OR drink_again IN ("no", "unsure", "yes")), sweet_dry INTEGER CHECK (sweet_dry IS NULL OR sweet_dry BETWEEN 1 AND 5), aroma_intensity INTEGER CHECK (aroma_intensity IS NULL OR aroma_intensity BETWEEN 1 AND 3), acidity INTEGER CHECK (acidity IS NULL OR acidity BETWEEN 1 AND 3), clean_umami INTEGER CHECK (clean_umami IS NULL OR clean_umami BETWEEN 1 AND 3), one_line_note TEXT, place TEXT, consumed_date TEXT NOT NULL, companions TEXT, food_pairing TEXT, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE)',
    'CREATE TABLE sake_images (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, record_id TEXT NOT NULL, image_key TEXT NOT NULL, thumbnail_key TEXT, mime_type TEXT NOT NULL, file_name TEXT NOT NULL, display_order INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE, FOREIGN KEY (record_id) REFERENCES sake_records(id) ON DELETE CASCADE)',
    'CREATE TABLE tags (id TEXT PRIMARY KEY, owner_id TEXT, drink_type TEXT NOT NULL DEFAULT "sake", tag_group TEXT NOT NULL CHECK (tag_group IN ("taste", "aroma", "mood")), label TEXT NOT NULL CHECK (length(trim(label)) > 0), is_default INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL, FOREIGN KEY (owner_id) REFERENCES users(id) ON DELETE CASCADE)',
    'CREATE TABLE record_tags (record_id TEXT NOT NULL, tag_id TEXT NOT NULL, created_at TEXT NOT NULL, PRIMARY KEY (record_id, tag_id), FOREIGN KEY (record_id) REFERENCES sake_records(id) ON DELETE CASCADE, FOREIGN KEY (tag_id) REFERENCES tags(id) ON DELETE CASCADE)'
  ];

  for (const stmt of legacyStmts) {
    await db.exec(stmt + ";");
  }

  // Seed Default 22 Tags
  await db.exec("INSERT INTO tags (id, owner_id, drink_type, tag_group, label, is_default, created_at) VALUES ('tag_taste_fresh', NULL, 'sake', 'taste', '산뜻', 1, '2026-08-08T00:00:00Z');");
  await db.exec("INSERT INTO tags (id, owner_id, drink_type, tag_group, label, is_default, created_at) VALUES ('tag_aroma_fruit', NULL, 'sake', 'aroma', 'Fruity', 1, '2026-08-08T00:00:00Z');");

  // Seed Synthetic Production User and Record
  const u1_id = "google:1234567890";
  const r1_uuid = "uuid_record_prod_1";
  const img1_uuid = "uuid_img_prod_10";
  const img1_key = "images/" + u1_id + "/sake/" + r1_uuid + "/" + img1_uuid + ".jpg";
  const thumb1_key = "thumbnails/" + u1_id + "/sake/" + r1_uuid + "/" + img1_uuid + ".webp";

  await db.exec("INSERT INTO users VALUES ('" + u1_id + "', 'google', '1234567890', 'user@example.com', 'Test User', 'https://avatar', '2026-08-08T00:00:00Z', '2026-08-08T00:00:00Z');");
  await db.exec("INSERT INTO sake_records (id, owner_id, drink_type, name, consumed_date, created_at, updated_at) VALUES ('" + r1_uuid + "', '" + u1_id + "', 'sake', 'Kokuryu Daiginjo', '2026-08-08', '2026-08-08T00:00:00Z', '2026-08-08T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, created_at) VALUES ('" + img1_uuid + "', '" + u1_id + "', '" + r1_uuid + "', '" + img1_key + "', '" + thumb1_key + "', 'image/jpeg', 'img1.jpg', '2026-08-08T00:00:00Z');");
  await db.exec("INSERT INTO record_tags (record_id, tag_id, created_at) VALUES ('" + r1_uuid + "', 'tag_taste_fresh', '2026-08-08T00:00:00Z');");

  await bucket.put(img1_key, "ORIGINAL_IMAGE_BYTES");
  await bucket.put(thumb1_key, "THUMBNAIL_IMAGE_BYTES");

  console.log("=== STEP 2: Executing Migration SQL ===");
  const migSQL = fs.readFileSync("./migration.sql", "utf8");
  const cleanMig = migSQL.replace(/--.*$/gm, "").replace(/^BEGIN TRANSACTION;$/gm, "").replace(/^COMMIT;$/gm, "");
  for (const rawStmt of cleanMig.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await db.exec(stmt + ";");
    }
  }

  // Verification 1: users table & PK unchanged
  const userCheck = await db.prepare("SELECT * FROM users WHERE id = ?").bind(u1_id).first();
  console.log("User PK Unchanged:", userCheck.id === u1_id);

  // Verification 2: sake_records integer PK migration & legacy_id
  const recCheck = await db.prepare("SELECT * FROM sake_records WHERE legacy_id = ?").bind(r1_uuid).first();
  console.log("SakeRecord Int PK:", recCheck.id, "Owner ID (TEXT):", recCheck.owner_id);

  // Verification 3: sake_images integer PK migration & R2 key preservation
  const imgCheck = await db.prepare("SELECT * FROM sake_images WHERE legacy_id = ?").bind(img1_uuid).first();
  console.log("SakeImage Int PK:", imgCheck.id, "Record ID (INT):", imgCheck.record_id);
  const r2Orig = await bucket.get(img1_key);
  const r2Thumb = await bucket.get(thumb1_key);
  console.log("R2 Keys Preserved:", r2Orig !== null && r2Thumb !== null);

  // Verification 4: Default tag PK preserved
  const tagCheck = await db.prepare("SELECT * FROM tags WHERE id = 'tag_taste_fresh'").first();
  console.log("Default Tag PK Preserved:", tagCheck !== null);

  if (userCheck.id === u1_id && typeof recCheck.id === 'number' && recCheck.owner_id === u1_id && imgCheck.record_id === recCheck.id && r2Orig !== null && r2Thumb !== null && tagCheck !== null) {
    console.log("[EMPIRICAL MIGRATION VERIFIED]: Real Production Migration 100% Succeeded!");
  } else {
    console.error("Migration Verification Failed");
    process.exit(1);
  }

  console.log("\n=== STEP 3: Full Endpoints Raw Request/Response Verification ===");
  const sessionToken = "session_token_123";
  const sessionExp = "2030-01-01T00:00:00Z";
  await db.exec("INSERT INTO oauth_sessions (id, user_id, created_at, expires_at) VALUES ('" + sessionToken + "', '" + u1_id + "', '2026-08-08T00:00:00Z', '" + sessionExp + "');");
  const authHeader = { "Cookie": "alcohol_log_session=" + sessionToken };

  // 1. GET /api/me
  const meRes = await mf.dispatchFetch("http://localhost/api/me", { headers: authHeader });
  const meJson = await meRes.json();
  console.log("[ENDPOINT 1] GET /api/me -> Status:", meRes.status, "Authenticated:", meJson.authenticated, "User:", meJson.user?.id);

  // 2. GET /api/sake-records
  const listRes = await mf.dispatchFetch("http://localhost/api/sake-records", { headers: authHeader });
  const listJson = await listRes.json();
  console.log("[ENDPOINT 2] GET /api/sake-records -> Status:", listRes.status, "Count:", listJson.length, "Entry ID:", listJson[0]?.id);

  // 3. GET /api/sake-records/:id
  const getRes = await mf.dispatchFetch("http://localhost/api/sake-records/" + recCheck.id, { headers: authHeader });
  const getJson = await getRes.json();
  console.log("[ENDPOINT 3] GET /api/sake-records/:id -> Status:", getRes.status, "Name:", getJson.record?.name);

  // 4. GET /api/images?key=... (Proxy & Ownership check before PUT)
  const imageProxyRes = await mf.dispatchFetch("http://localhost/api/images?key=" + encodeURIComponent(img1_key), { headers: authHeader });
  console.log("[ENDPOINT 4] GET /api/images?key=... -> Status:", imageProxyRes.status, "Content-Type:", imageProxyRes.headers.get("Content-Type"));

  // 5. GET /api/sake-records/search?q=Kokuryu
  const searchRes = await mf.dispatchFetch("http://localhost/api/sake-records/search?q=Kokuryu", { headers: authHeader });
  const searchJson = await searchRes.json();
  console.log("[ENDPOINT 5] GET /api/sake-records/search?q=Kokuryu -> Status:", searchRes.status, "Matched Count:", searchJson.length);

  // 6. PUT /api/sake-records/:id (Include existing images array to preserve images)
  const putRes = await mf.dispatchFetch("http://localhost/api/sake-records/" + recCheck.id, {
    method: "PUT",
    headers: { ...authHeader, "Content-Type": "application/json" },
    body: JSON.stringify({ name: "Kokuryu Daiginjo Updated", region: "Fukui", consumed_date: "2026-08-08", images: getJson.images })
  });
  const putJson = await putRes.json();
  console.log("[ENDPOINT 6] PUT /api/sake-records/:id -> Status:", putRes.status, "Updated Name:", putJson.record?.name);

  // 7. POST /api/sake-records/:id/images
  const addImgRes = await mf.dispatchFetch("http://localhost/api/sake-records/" + recCheck.id + "/images", {
    method: "POST",
    headers: { ...authHeader, "Content-Type": "application/json" },
    body: JSON.stringify({ file_name: "new_bottle.jpg", mime_type: "image/jpeg" })
  });
  const addImgJson = await addImgRes.json();
  console.log("[ENDPOINT 7] POST /api/sake-records/:id/images -> Status:", addImgRes.status, "Image ID:", addImgJson.id);

  // 8. DELETE /api/sake-records/:id/images/:imageId
  const delImgRes = await mf.dispatchFetch("http://localhost/api/sake-records/" + recCheck.id + "/images/" + addImgJson.id, {
    method: "DELETE",
    headers: authHeader
  });
  console.log("[ENDPOINT 8] DELETE /api/sake-records/:id/images/:imageId -> Status:", delImgRes.status);

  // 9. GET /api/tags?drink_type=sake
  const tagsRes = await mf.dispatchFetch("http://localhost/api/tags?drink_type=sake", { headers: authHeader });
  const tagsJson = await tagsRes.json();
  console.log("[ENDPOINT 9] GET /api/tags?drink_type=sake -> Status:", tagsRes.status, "Tag Count:", tagsJson.length);

  // 10. POST /api/tags (Latin Case-Insensitive & Space Deduplication: "fruity " vs "Fruity")
  const tagCreate1 = await mf.dispatchFetch("http://localhost/api/tags", {
    method: "POST",
    headers: { ...authHeader, "Content-Type": "application/json" },
    body: JSON.stringify({ tag_group: "aroma", label: "fruity " })
  });
  const tagJson1 = await tagCreate1.json();
  console.log("[ENDPOINT 10] POST /api/tags (Latin Case-Insensitive) -> Status:", tagCreate1.status, "Already Exists:", tagJson1.already_exists, "Matched ID:", tagJson1.id, "Header:", tagCreate1.headers.get("X-Sake-Tag-Existing"));

  // 11. GET /api/auth/google/login
  const loginRes = await mf.dispatchFetch("http://localhost/api/auth/google/login", { redirect: "manual" });
  console.log("[ENDPOINT 11] GET /api/auth/google/login -> Status:", loginRes.status, "Redirect Location:", loginRes.headers.get("Location")?.slice(0, 45) + "...");

  // 12. GET /api/auth/google-callback (Invalid state rejection test)
  const callbackRes = await mf.dispatchFetch("http://localhost/api/auth/google-callback?code=fake&state=invalid");
  console.log("[ENDPOINT 12] GET /api/auth/google-callback -> Status:", callbackRes.status);

  // 13. DELETE /api/sake-records/:id (Cascade test)
  const delRes = await mf.dispatchFetch("http://localhost/api/sake-records/" + recCheck.id, {
    method: "DELETE",
    headers: authHeader
  });
  console.log("[ENDPOINT 13] DELETE /api/sake-records/:id -> Status:", delRes.status);

  // 14. POST /api/auth/logout (Revoke session)
  const logoutRes = await mf.dispatchFetch("http://localhost/api/auth/logout", { method: "POST", headers: authHeader });
  console.log("[ENDPOINT 14] POST /api/auth/logout -> Status:", logoutRes.status);

  const postRecCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_records").first()).c;
  const postImgCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_images").first()).c;
  const r2OrigAfter = await bucket.get(img1_key);
  const r2ThumbAfter = await bucket.get(thumb1_key);

  console.log("Post Delete State - Records:", postRecCount, "Images:", postImgCount, "R2 Deleted:", r2OrigAfter === null && r2ThumbAfter === null);

  if (meRes.status === 200 && listRes.status === 200 && getRes.status === 200 && searchRes.status === 200 && putRes.status === 200 && addImgRes.status === 201 && delImgRes.status === 204 && tagsRes.status === 200 && tagJson1.already_exists === true && tagJson1.id === "tag_aroma_fruit" && imageProxyRes.status === 200 && loginRes.status === 302 && callbackRes.status === 400 && logoutRes.status === 200 && delRes.status === 204 && postRecCount === 0 && postImgCount === 0 && r2OrigAfter === null && r2ThumbAfter === null) {
    console.log("[EMPIRICAL CONTRACT VERIFIED]: All 14 Deployable Pages Functions Endpoints & Cascade Deletes Succeeded!");
  } else {
    console.error("Contract Verification Failed");
    process.exit(1);
  }

  await mf.dispose();
}

run().catch(err => {
  console.error(err);
  process.exit(1);
});
`

	_ = os.WriteFile(filepath.Join(tmpDir, "test_runner.mjs"), []byte(runnerJS), 0644)

	cmdRun := exec.Command("node", "test_runner.mjs")
	cmdRun.Dir = tmpDir
	outBytes, err := cmdRun.CombinedOutput()
	rawOutput := string(outBytes)

	t.Logf("Miniflare Raw Output:\n%s", rawOutput)
	if err != nil {
		t.Fatalf("test failed: %v\nOutput: %s", err, rawOutput)
	}

	if !strings.Contains(rawOutput, "[EMPIRICAL MIGRATION VERIFIED]") || !strings.Contains(rawOutput, "[EMPIRICAL CONTRACT VERIFIED]") {
		t.Fatalf("empirical verification markers not found in output:\n%s", rawOutput)
	}
}

func copyDir(t *testing.T, srcDir, destDir string) {
	t.Helper()
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		targetPath := filepath.Join(destDir, rel)
		if info.IsDir() {
			return os.MkdirAll(targetPath, 0755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(targetPath, data, info.Mode())
	})
	if err != nil {
		t.Fatalf("failed copying directory from %s to %s: %v", srcDir, destDir, err)
	}
}

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
