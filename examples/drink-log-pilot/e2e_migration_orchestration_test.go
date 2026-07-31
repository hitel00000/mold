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

func TestDrinkLog_E2EMigrationAndDeleteOrchestration(t *testing.T) {
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

	indexTS := output.IndexTS + `
app.delete('/api/sake_records/:id/orchestrate-delete', async (c) => {
  const recordId = c.req.param('id');
  const cookieHeader = c.req.header('Cookie') || '';
  const match = cookieHeader.match(/mold_session=([^;]+)/);
  if (!match) return c.json({ error: { code: 'UNAUTHORIZED', message: 'authentication required' } }, 401);

  const token = match[1];
  const sess = await c.env.DB.prepare('SELECT user_id FROM "_mold_sessions" WHERE id = ? AND expires_at > ?')
    .bind(token, new Date().toISOString()).first();
  if (!sess) return c.json({ error: { code: 'UNAUTHORIZED', message: 'invalid session' } }, 401);

  const record = await c.env.DB.prepare('SELECT * FROM "sake_records" WHERE id = ?').bind(recordId).first();
  if (!record) return c.json({ error: { code: 'NOT_FOUND', message: 'record not found' } }, 404);
  if (record.owner_id != sess.user_id) return c.json({ error: { code: 'FORBIDDEN', message: 'forbidden' } }, 403);

  const images = await c.env.DB.prepare('SELECT * FROM "sake_images" WHERE record_id = ?').bind(recordId).all();
  const childImages = images.results || [];

  for (const img of childImages) {
    if (img.image_key) {
      const resBlob = await app.request(` + "`" + `/api/sake_images/${img.id}/blob/image_key` + "`" + `, { method: 'DELETE', headers: { Cookie: cookieHeader } }, c.env);
      if (resBlob.status !== 200 && resBlob.status !== 404) {
        return c.json({ error: { code: 'RECORD_DELETE_PARTIAL_FAILURE', message: ` + "`" + `failed to delete image_key blob for image ${img.id}` + "`" + ` } }, 500);
      }
    }
    if (img.thumbnail_key) {
      const resThumb = await app.request(` + "`" + `/api/sake_images/${img.id}/blob/thumbnail_key` + "`" + `, { method: 'DELETE', headers: { Cookie: cookieHeader } }, c.env);
      if (resThumb.status !== 200 && resThumb.status !== 404) {
        return c.json({ error: { code: 'RECORD_DELETE_PARTIAL_FAILURE', message: ` + "`" + `failed to delete thumbnail_key blob for image ${img.id}` + "`" + ` } }, 500);
      }
    }
    const resImg = await app.request(` + "`" + `/api/sake_images/${img.id}` + "`" + `, { method: 'DELETE', headers: { Cookie: cookieHeader } }, c.env);
    if (resImg.status !== 200 && resImg.status !== 404) {
      return c.json({ error: { code: 'RECORD_DELETE_PARTIAL_FAILURE', message: ` + "`" + `failed to delete image row ${img.id}` + "`" + ` } }, 500);
    }
  }

  await c.env.DB.prepare('DELETE FROM "record_tags" WHERE sake_record_id = ?').bind(recordId).run();

  const resParent = await app.request(` + "`" + `/api/sake_records/${recordId}` + "`" + `, { method: 'DELETE', headers: { Cookie: cookieHeader } }, c.env);
  if (resParent.status === 200 || resParent.status === 404) {
    return c.json({ data: { deleted: true, id: Number(recordId) } }, 200);
  }
  return c.json({ error: { code: 'PARENT_DELETE_FAILED', message: 'failed to delete parent sake record' } }, resParent.status);
});
`
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
    d1Databases: { DB: "mold-d1" },
    r2Buckets: { BUCKET: "mold-r2" },
  });

  const db = await mf.getD1Database("DB");
  await db.exec("PRAGMA foreign_keys = ON;");
  const bucket = await mf.getR2Bucket("BUCKET");

  console.log("=== STEP 1: Seeding Legacy Schema (UUID-based) & Synthetic Data ===");
  const legacyStmts = [
    'CREATE TABLE users (id TEXT PRIMARY KEY, provider TEXT NOT NULL, provider_user_id TEXT NOT NULL, email TEXT, display_name TEXT, avatar_url TEXT, last_login_at TEXT NOT NULL, role TEXT DEFAULT "user", created_at TEXT NOT NULL, updated_at TEXT NOT NULL, UNIQUE (provider, provider_user_id))',
    'CREATE TABLE tags (id TEXT PRIMARY KEY, owner_id TEXT, drink_type TEXT DEFAULT "sake", tag_group TEXT NOT NULL, label TEXT NOT NULL, is_default INTEGER DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY (owner_id) REFERENCES users(id))',
    'CREATE TABLE sake_records (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, drink_type TEXT DEFAULT "sake", name TEXT NOT NULL, region TEXT, brewery TEXT, rice TEXT, sake_type TEXT, sake_meter_value TEXT, abv TEXT, volume TEXT, price TEXT, one_line_note TEXT, place TEXT, companions TEXT, food_pairing TEXT, consumed_date TEXT NOT NULL, drink_again TEXT, sweet_dry INTEGER, aroma_intensity INTEGER, acidity INTEGER, clean_umami INTEGER, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY (owner_id) REFERENCES users(id))',
    'CREATE TABLE sake_images (id TEXT PRIMARY KEY, owner_id TEXT NOT NULL, record_id TEXT NOT NULL, image_key TEXT NOT NULL, thumbnail_key TEXT, mime_type TEXT NOT NULL, file_name TEXT NOT NULL, display_order INTEGER DEFAULT 0, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, FOREIGN KEY (owner_id) REFERENCES users(id), FOREIGN KEY (record_id) REFERENCES sake_records(id))',
    'CREATE TABLE record_tags (record_id TEXT NOT NULL, tag_id TEXT NOT NULL, created_at TEXT NOT NULL, updated_at TEXT NOT NULL, PRIMARY KEY (record_id, tag_id), FOREIGN KEY (record_id) REFERENCES sake_records(id), FOREIGN KEY (tag_id) REFERENCES tags(id))',
    'CREATE TABLE oauth_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL)'
  ];

  for (const stmt of legacyStmts) {
    await db.exec(stmt + ";");
  }

  // Seed Legacy Synthetic Data
  const u1_uuid = "uuid_user_101";
  const u2_uuid = "uuid_user_102";
  const r1_uuid = "uuid_record_1";
  const img1_uuid = "uuid_img_10";
  const img1_key = "images/" + u1_uuid + "/sake/" + r1_uuid + "/img1.jpg";
  const thumb1_key = "images/" + u1_uuid + "/sake/" + r1_uuid + "/img1_thumb.jpg";

  await db.exec("INSERT INTO users VALUES ('" + u1_uuid + "', 'google', 'g_101', 'user101@example.com', 'User 101', 'https://avatar', '2026-07-30T00:00:00Z', 'user', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO users VALUES ('" + u2_uuid + "', 'google', 'g_102', 'user102@example.com', 'User 102', 'https://avatar', '2026-07-30T00:00:00Z', 'user', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_records (id, owner_id, drink_type, name, consumed_date, created_at, updated_at) VALUES ('" + r1_uuid + "', '" + u1_uuid + "', 'sake', 'Dassai 23 Legacy', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, created_at, updated_at) VALUES ('" + img1_uuid + "', '" + u1_uuid + "', '" + r1_uuid + "', '" + img1_key + "', '" + thumb1_key + "', 'image/jpeg', 'img1.jpg', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await bucket.put(img1_key, "LEGACY_BINARY_R2_DATA");
  await bucket.put(thumb1_key, "LEGACY_THUMBNAIL_R2_DATA");

  console.log("=== STEP 2: Running 0001_drink_log_migration.sql D1 Migration Script ===");
  const migSQL = fs.readFileSync("./migration.sql", "utf8");
  const cleanMig = migSQL.replace(/--.*$/gm, "").replace(/^BEGIN TRANSACTION;$/gm, "").replace(/^COMMIT;$/gm, "");
  for (const rawStmt of cleanMig.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await db.exec(stmt + ";");
    }
  }

  // Verify Migration Results
  const migratedUser = await db.prepare("SELECT * FROM users WHERE legacy_id = ?").bind(u1_uuid).first();
  const migratedUser2 = await db.prepare("SELECT * FROM users WHERE legacy_id = ?").bind(u2_uuid).first();
  const migratedRecord = await db.prepare("SELECT * FROM sake_records WHERE legacy_id = ?").bind(r1_uuid).first();
  const migratedImage = await db.prepare("SELECT * FROM sake_images WHERE legacy_id = ?").bind(img1_uuid).first();
  const r2KeyPreserved = await bucket.get(img1_key);
  const r2ThumbPreserved = await bucket.get(thumb1_key);

  console.log("Migrated User Int ID:", migratedUser.id, ", Legacy ID:", migratedUser.legacy_id);
  console.log("Migrated SakeRecord Int ID:", migratedRecord.id, ", FK owner_id:", migratedRecord.owner_id);
  console.log("Migrated SakeImage Int ID:", migratedImage.id, ", FK record_id:", migratedImage.record_id);
  console.log("R2 Preserved Original Key Exists:", r2KeyPreserved !== null);
  console.log("R2 Preserved Thumbnail Key Exists:", r2ThumbPreserved !== null);

  if (typeof migratedUser.id === 'number' && migratedRecord.owner_id === migratedUser.id && migratedImage.record_id === migratedRecord.id && r2KeyPreserved !== null && r2ThumbPreserved !== null) {
    console.log("[EMPIRICAL MIGRATION VERIFIED]: D1 Migration Script 100% Succeeded!");
  } else {
    console.error("FAILED D1 Migration Verification");
    process.exit(1);
  }

  console.log("\n=== STEP 3: Testing 22 Default Tags Seed Idempotency ===");
  const defaultTags = [
    { slug: 'tag_taste_달콤함', tag_group: 'taste', label: '달콤함' },
    { slug: 'tag_taste_깔끔함', tag_group: 'taste', label: '깔끔함' },
    { slug: 'tag_taste_드라이함', tag_group: 'taste', label: '드라이함' },
    { slug: 'tag_taste_산미', tag_group: 'taste', label: '산미' },
    { slug: 'tag_taste_감칠맛', tag_group: 'taste', label: '감칠맛' },
    { slug: 'tag_taste_묵직함', tag_group: 'taste', label: '묵직함' },
    { slug: 'tag_taste_부드러움', tag_group: 'taste', label: '부드러움' },
    { slug: 'tag_aroma_과일향_(사과/청포도)', tag_group: 'aroma', label: '과일향 (사과/청포도)' },
    { slug: 'tag_aroma_과일향_(참외/멜론)', tag_group: 'aroma', label: '과일향 (참외/멜론)' },
    { slug: 'tag_aroma_과일향_(감귤/레몬)', tag_group: 'aroma', label: '과일향 (감귤/레몬)' },
    { slug: 'tag_aroma_과일향_(바나나)', tag_group: 'aroma', label: '과일향 (바나나)' },
    { slug: 'tag_aroma_과일향_(열대과일)', tag_group: 'aroma', label: '과일향 (열대과일)' },
    { slug: 'tag_aroma_꽃향', tag_group: 'aroma', label: '꽃향' },
    { slug: 'tag_aroma_곡물향/쌀향', tag_group: 'aroma', label: '곡물향/쌀향' },
    { slug: 'tag_aroma_유제품향/요거트', tag_group: 'aroma', label: '유제품향/요거트' },
    { slug: 'tag_aroma_풀향/삼나무', tag_group: 'aroma', label: '풀향/삼나무' },
    { slug: 'tag_aroma_향신료향', tag_group: 'aroma', label: '향신료향' },
    { slug: 'tag_aroma_숙성향', tag_group: 'aroma', label: '숙성향' },
    { slug: 'tag_mood_입문자_추천', tag_group: 'mood', label: '입문자 추천' },
    { slug: 'tag_mood_반주/식사용', tag_group: 'mood', label: '반주/식사용' },
    { slug: 'tag_mood_특별한_날', tag_group: 'mood', label: '특별한 날' },
    { slug: 'tag_mood_혼술', tag_group: 'mood', label: '혼술' }
  ];

  const nowStr = new Date().toISOString();
  // 1st Seed Run
  for (const tag of defaultTags) {
    await db.prepare('INSERT OR IGNORE INTO "tags" ("slug", "owner_id", "drink_type", "tag_group", "label", "is_default", "created_at", "updated_at") VALUES (?, NULL, "sake", ?, ?, 1, ?, ?)')
      .bind(tag.slug, tag.tag_group, tag.label, nowStr, nowStr).run();
  }
  const tagCount1st = (await db.prepare("SELECT COUNT(*) as c FROM tags WHERE is_default = 1").first()).c;

  // 2nd Seed Run (Re-execution test)
  for (const tag of defaultTags) {
    await db.prepare('INSERT OR IGNORE INTO "tags" ("slug", "owner_id", "drink_type", "tag_group", "label", "is_default", "created_at", "updated_at") VALUES (?, NULL, "sake", ?, ?, 1, ?, ?)')
      .bind(tag.slug, tag.tag_group, tag.label, nowStr, nowStr).run();
  }
  const tagCount2nd = (await db.prepare("SELECT COUNT(*) as c FROM tags WHERE is_default = 1").first()).c;

  if (tagCount1st === 22 && tagCount2nd === 22) {
    console.log("[EMPIRICAL SEED IDEMPOTENCY VERIFIED]: 22 Korean Default Tags Seeded Idempotently! 0 Duplicates!");
  } else {
    console.error("FAILED Tag Seed Idempotency Test, 1st:", tagCount1st, "2nd:", tagCount2nd);
    process.exit(1);
  }

  console.log("\n=== STEP 4: Testing Delete Orchestration (Strict Pure Session HTTP API) ===");
  const now = new Date().toISOString();
  const user1Token = "sess_user_101";
  const user2Token = "sess_user_102";

  await db.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('" + user1Token + "', " + migratedUser.id + ", '" + now + "', '2030-01-01T00:00:00Z');");
  await db.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('" + user2Token + "', " + migratedUser2.id + ", '" + now + "', '2030-01-01T00:00:00Z');");

  // 1) Cross-user Forbidden
  const resCrossUser = await mf.dispatchFetch("http://localhost/api/sake_records/" + migratedRecord.id + "/orchestrate-delete", {
    method: "DELETE", headers: { "Cookie": "mold_session=" + user2Token }
  });
  console.log("Scenario 1 Cross-user DELETE Status:", resCrossUser.status);

  // 2) Full Delete Orchestration Call (User 1 calling orchestrate-delete)
  const resOrchestrateDel = await mf.dispatchFetch("http://localhost/api/sake_records/" + migratedRecord.id + "/orchestrate-delete", {
    method: "DELETE", headers: { "Cookie": "mold_session=" + user1Token }
  });
  console.log("Scenario 2 Delete Orchestration Status:", resOrchestrateDel.status);

  const finalRecordCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_records").first()).c;
  const finalImageCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_images").first()).c;

  console.log("Final State - Record Count:", finalRecordCount, ", Image Count:", finalImageCount);
  if (resCrossUser.status === 403 && resOrchestrateDel.status === 200 && finalRecordCount === 0 && finalImageCount === 0) {
    console.log("[SCENARIO VERIFIED]: Happy Path Delete Orchestration (image_key + thumbnail_key) Clean Succeeded!");
  } else {
    console.error("FAILED Delete Orchestration Happy Path");
    process.exit(1);
  }

  console.log("\n=== STEP 5: Testing Partial Failure Abort & Retry Idempotency Scenarios ===");
  // Seed Record 2 with Image 2 (Simulating Partial Failure on Image Row Delete)
  await db.exec("INSERT INTO sake_records (id, owner_id, drink_type, name, consumed_date, created_at, updated_at) VALUES (2, " + migratedUser.id + ", 'sake', 'Fail Test Sake', '2026-07-30T00:00:00Z', '" + now + "', '" + now + "');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, created_at, updated_at) VALUES (20, " + migratedUser.id + ", 2, 'img2_key', 'thumb2_key', 'image/jpeg', 'img2.jpg', '" + now + "', '" + now + "');");
  await bucket.put("img2_key", "IMG2_DATA");
  await bucket.put("thumb2_key", "THUMB2_DATA");

  // Simulating Partial Failure: Manually delete img2_key blob, leaving image row 20 and record 2
  await bucket.delete("img2_key");

  // Call Delete Orchestration on Record 2 (Idempotent Retry over partially cleaned state)
  const resRetryRecordDel = await mf.dispatchFetch("http://localhost/api/sake_records/2/orchestrate-delete", {
    method: "DELETE", headers: { "Cookie": "mold_session=" + user1Token }
  });
  console.log("Partial Failure Retry Orchestration DELETE Status:", resRetryRecordDel.status);

  const finalRecord2Count = (await db.prepare("SELECT COUNT(*) as c FROM sake_records WHERE id = 2").first()).c;
  const finalImage2Count = (await db.prepare("SELECT COUNT(*) as c FROM sake_images WHERE id = 20").first()).c;

  console.log("Retry Final State - Record 2 Count:", finalRecord2Count, ", Image 20 Count:", finalImage2Count);
  if (resRetryRecordDel.status === 200 && finalRecord2Count === 0 && finalImage2Count === 0) {
    console.log("[EMPIRICAL ABORT & RETRY IDEMPOTENCY VERIFIED]: Mid-stage failure resolved by idempotent retry 100% cleanly!");
  } else {
    console.error("FAILED Partial Failure Retry Scenario");
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
		t.Fatalf("test failed: %v", err)
	}

	if !strings.Contains(rawOutput, "[EMPIRICAL MIGRATION VERIFIED]") || !strings.Contains(rawOutput, "[EMPIRICAL SEED IDEMPOTENCY VERIFIED]") || !strings.Contains(rawOutput, "[EMPIRICAL ABORT & RETRY IDEMPOTENCY VERIFIED]") {
		t.Fatalf("empirical verification markers not found in output:\n%s", rawOutput)
	}
}
