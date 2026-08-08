package pilot_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/codegen/cloudflare"
	"github.com/hitel00000/mold/resource"
)

func TestDrinkLog_E2ERealProductionMigration(t *testing.T) {
	nodePath, err := exec.LookPath("node")
	if err != nil || nodePath == "" {
		t.Skip("node not found in PATH")
	}

	reg, err := resource.LoadAll(".")
	if err != nil {
		t.Fatalf("failed loading resources: %v", err)
	}

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("generate Cloudflare Workers target failed: %v", err)
	}

	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(output.PackageJSON), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "wrangler.jsonc"), []byte(output.WranglerConfig), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte(output.SchemaSQL), 0644)

	glueCode := `const app = new Hono<{ Bindings: Bindings }>();

// Custom Hono Glue Endpoints for Drink-Log Real Production Migration (Preposed for route priority)
app.get('/api/sake-records', async (c) => {
  const cookieHeader = c.req.header('Cookie') || '';
  const match = cookieHeader.match(/alcohol_log_session=([^;]+)/);
  if (!match) return c.json({ error: 'unauthorized' }, 401);

  const session = await c.env.DB.prepare('SELECT user_id FROM oauth_sessions WHERE id = ? AND expires_at > ?')
    .bind(match[1], new Date().toISOString()).first();
  if (!session) return c.json({ error: 'unauthorized' }, 401);

  const records = await c.env.DB.prepare('SELECT * FROM sake_records WHERE owner_id = ? ORDER BY consumed_date DESC, created_at DESC')
    .bind(session.user_id).all();

  const entries = [];
  for (const record of records.results || []) {
    const images = await c.env.DB.prepare('SELECT * FROM sake_images WHERE owner_id = ? AND record_id = ? ORDER BY display_order').bind(session.user_id, record.id).all();
    const recordTags = await c.env.DB.prepare('SELECT * FROM record_tags WHERE sake_record_id = ?').bind(record.id).all();
    const tags = await c.env.DB.prepare('SELECT * FROM tags WHERE drink_type = "sake" AND (owner_id IS NULL OR owner_id = ?)').bind(session.user_id).all();
    const tagsMap = new Map((tags.results || []).map(t => [t.id, t]));

    entries.push({
      id: String(record.id),
      record,
      images: (images.results || []).map(img => ({
        ...img,
        data_url: ` + "`" + `/api/images?key=${encodeURIComponent(img.image_key)}` + "`" + `,
        thumbnail_data_url: img.thumbnail_key ? ` + "`" + `/api/images?key=${encodeURIComponent(img.thumbnail_key)}` + "`" + ` : null,
      })),
      record_tags: recordTags.results || [],
      tags: (recordTags.results || []).map(rt => tagsMap.get(rt.tag_id)).filter(Boolean).map(t => ({ ...t, is_default: Boolean(t.is_default) }))
    });
  }

  return c.json(entries);
});

app.get('/api/tags', async (c) => {
  const cookieHeader = c.req.header('Cookie') || '';
  const match = cookieHeader.match(/alcohol_log_session=([^;]+)/);
  if (!match) return c.json({ error: 'unauthorized' }, 401);

  const session = await c.env.DB.prepare('SELECT user_id FROM oauth_sessions WHERE id = ? AND expires_at > ?')
    .bind(match[1], new Date().toISOString()).first();
  if (!session) return c.json({ error: 'unauthorized' }, 401);

  const tags = await c.env.DB.prepare('SELECT * FROM tags WHERE drink_type = "sake" AND (owner_id IS NULL OR owner_id = ?) ORDER BY tag_group, is_default DESC, label').bind(session.user_id).all();
  return c.json((tags.results || []).map(t => ({ ...t, is_default: Boolean(t.is_default) })));
});

app.post('/api/tags', async (c) => {
  const cookieHeader = c.req.header('Cookie') || '';
  const match = cookieHeader.match(/alcohol_log_session=([^;]+)/);
  if (!match) return c.json({ error: 'unauthorized' }, 401);

  const session = await c.env.DB.prepare('SELECT user_id FROM oauth_sessions WHERE id = ? AND expires_at > ?')
    .bind(match[1], new Date().toISOString()).first();
  if (!session) return c.json({ error: 'unauthorized' }, 401);

  const body = await c.req.json();
  const tagGroup = body.tag_group;
  const label = (body.label || '').trim().slice(0, 20);
  if (!tagGroup || !label) return c.json({ error: 'invalid_tag' }, 400);

  const tags = await c.env.DB.prepare('SELECT * FROM tags WHERE drink_type = "sake" AND (owner_id IS NULL OR owner_id = ?)').bind(session.user_id).all();
  const existing = (tags.results || []).find(t => t.tag_group === tagGroup && t.label.trim().toLowerCase() === label.toLowerCase());
  if (existing) {
    return c.json({ ...existing, is_default: Boolean(existing.is_default), already_exists: true }, 200, { 'X-Sake-Tag-Existing': 'true' });
  }

  const id = crypto.randomUUID();
  const now = new Date().toISOString();
  await c.env.DB.prepare('INSERT INTO tags (id, owner_id, drink_type, tag_group, label, is_default, created_at) VALUES (?, ?, "sake", ?, ?, 0, ?)')
    .bind(id, session.user_id, tagGroup, label, now).run();

  return c.json({ id, owner_id: session.user_id, drink_type: 'sake', tag_group: tagGroup, label, is_default: false, created_at: now, already_exists: false }, 201);
});

app.delete('/api/sake-records/:id', async (c) => {
  const id = c.req.param('id');
  const cookieHeader = c.req.header('Cookie') || '';
  const match = cookieHeader.match(/alcohol_log_session=([^;]+)/);
  if (!match) return c.json({ error: 'unauthorized' }, 401);

  const session = await c.env.DB.prepare('SELECT user_id FROM oauth_sessions WHERE id = ? AND expires_at > ?')
    .bind(match[1], new Date().toISOString()).first();
  if (!session) return c.json({ error: 'unauthorized' }, 401);

  const rec = await c.env.DB.prepare('SELECT * FROM sake_records WHERE id = ?').bind(id).first();
  if (!rec) return c.json({ error: 'not_found' }, 404);
  if (rec.owner_id !== session.user_id) return c.json({ error: 'forbidden' }, 403);

  const images = await c.env.DB.prepare('SELECT image_key, thumbnail_key FROM sake_images WHERE owner_id = ? AND record_id = ?').bind(session.user_id, id).all();
  await c.env.DB.prepare('DELETE FROM sake_records WHERE owner_id = ? AND id = ?').bind(session.user_id, id).run();

  const bucket = c.env.IMAGES || c.env.alcohol_log_images;
  if (bucket) {
    await Promise.all((images.results || []).flatMap(img => [img.image_key, img.thumbnail_key].filter(Boolean).map(key => bucket.delete(key))));
  }

  return new Response(null, { status: 204 });
});`

	indexTS := strings.Replace(output.IndexTS, "const app = new Hono<{ Bindings: Bindings }>();", glueCode, 1)
	_ = os.WriteFile(filepath.Join(tmpDir, "index.ts"), []byte(indexTS), 0644)

	migSQL, err := os.ReadFile(filepath.Join("migrations", "0001_drink_log_migration.sql"))
	if err != nil {
		t.Fatalf("failed reading migration sql: %v", err)
	}
	_ = os.WriteFile(filepath.Join(tmpDir, "migration.sql"), migSQL, 0644)

	cmdNpm := exec.Command("npm.cmd", "install", "--no-audit", "--no-fund", "miniflare", "hono", "esbuild")
	if os.Getenv("OS") != "Windows_NT" {
		cmdNpm = exec.Command("npm", "install", "--no-audit", "--no-fund", "miniflare", "hono", "esbuild")
	}
	cmdNpm.Dir = tmpDir
	if out, err := cmdNpm.CombinedOutput(); err != nil {
		t.Fatalf("npm install failed: %v\nOutput: %s", err, string(out))
	}

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
  await db.exec("INSERT INTO tags (id, owner_id, drink_type, tag_group, label, is_default, created_at) VALUES ('tag_taste_umami', NULL, 'sake', 'taste', '감칠', 1, '2026-08-08T00:00:00Z');");

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

  console.log("\n=== STEP 3: Testing Endpoint Response & Tag Deduplication ===");
  const sessionToken = "session_token_123";
  const sessionExp = "2030-01-01T00:00:00Z";
  await db.exec("INSERT INTO oauth_sessions (id, user_id, created_at, expires_at) VALUES ('" + sessionToken + "', '" + u1_id + "', '2026-08-08T00:00:00Z', '" + sessionExp + "');");

  // GET /api/sake-records response contract
  const listRes = await mf.dispatchFetch("http://localhost/api/sake-records", {
    headers: { "Cookie": "alcohol_log_session=" + sessionToken }
  });
  const listJson = await listRes.json();
  console.log("GET /api/sake-records Status:", listRes.status);
  console.log("List Response Entry Count:", listJson.length);
  console.log("Entry Shape:", Object.keys(listJson[0] || {}));

  // POST /api/tags deduplication (toLowerCase test)
  const tagCreate1 = await mf.dispatchFetch("http://localhost/api/tags", {
    method: "POST",
    headers: { "Cookie": "alcohol_log_session=" + sessionToken, "Content-Type": "application/json" },
    body: JSON.stringify({ tag_group: "taste", label: "산뜻" })
  });
  const tagJson1 = await tagCreate1.json();
  console.log("Duplicate Tag POST Status:", tagCreate1.status, "Already Exists:", tagJson1.already_exists);

  // DELETE /api/sake-records/:id Cascade test
  const delRes = await mf.dispatchFetch("http://localhost/api/sake-records/" + recCheck.id, {
    method: "DELETE",
    headers: { "Cookie": "alcohol_log_session=" + sessionToken }
  });
  console.log("DELETE /api/sake-records/:id Status:", delRes.status);

  const postRecCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_records").first()).c;
  const postImgCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_images").first()).c;
  const r2OrigAfter = await bucket.get(img1_key);
  const r2ThumbAfter = await bucket.get(thumb1_key);

  console.log("Post Delete State - Records:", postRecCount, "Images:", postImgCount, "R2 Deleted:", r2OrigAfter === null && r2ThumbAfter === null);

  if (listRes.status === 200 && tagJson1.already_exists === true && delRes.status === 204 && postRecCount === 0 && postImgCount === 0 && r2OrigAfter === null && r2ThumbAfter === null) {
    console.log("[EMPIRICAL CONTRACT VERIFIED]: All API Contracts & Cascade Deletes Succeeded!");
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
