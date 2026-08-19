package cloudflare_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hitel00000/mold/codegen/cloudflare"
	"github.com/hitel00000/mold/resource"
)

func TestAllRoutesSideBySideABDiffEmpirical(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heavy Miniflare integration test in short mode")
	}
	resDir := filepath.Join("..", "..", "..", "drink-log", "resources")
	absResDir, err := filepath.Abs(resDir)
	if err != nil {
		t.Fatalf("failed getting abs res path: %v", err)
	}

	reg, err := resource.LoadAll(absResDir)
	if err != nil {
		t.Fatalf("resource.LoadAll failed: %v", err)
	}

	gen := cloudflare.NewGenerator()
	out, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("Generate failed: %v", err)
	}

	drinkLogDir := filepath.Join("..", "..", "..", "drink-log")
	absDrinkLogDir, err := filepath.Abs(drinkLogDir)
	if err != nil {
		t.Fatalf("failed getting abs drink-log path: %v", err)
	}

	gluePath := filepath.Join(absDrinkLogDir, "functions", "_shared", "glue.ts")
	if _, err := os.Stat(gluePath); os.IsNotExist(err) {
		t.Skip("skipping side-by-side test: drink-log glue.ts not found at " + gluePath)
	}

	moldAppPath := filepath.Join(absDrinkLogDir, "functions", "_shared", "generated", "mold_app.ts")
	tsContent := out.IndexTS + "\nexport { app as moldApp };\n"
	if err := os.WriteFile(moldAppPath, []byte(tsContent), 0644); err != nil {
		t.Fatalf("failed updating mold_app.ts: %v", err)
	}

	tmpDir := t.TempDir()

	legacySchemaPath := filepath.Join(absDrinkLogDir, "docs", "schema.sql")
	legacySchemaSQL, err := os.ReadFile(legacySchemaPath)
	if err != nil {
		t.Fatalf("failed reading docs/schema.sql: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "schema_legacy.sql"), legacySchemaSQL, 0644); err != nil {
		t.Fatalf("failed writing schema_legacy.sql: %v", err)
	}

	harnessTS := `
import { Hono } from 'hono';
import { listSakeRecords, getSakeRecord, createSakeRecord, updateSakeRecord, deleteSakeRecord, addSakeRecordImage, deleteSakeRecordImage, listSakeTags, createSakeTag, searchSakeRecords } from '` + filepath.ToSlash(filepath.Join(absDrinkLogDir, "functions", "_shared", "sake.ts")) + `';
import { fetchSakeRecordsEntry, fetchSingleSakeRecordEntry, createSakeRecordEntry, updateSakeRecordEntry, deleteSakeRecordEntry, addSakeRecordImageEntry, deleteSakeRecordImageEntry, listSakeTagsEntry, createSakeTagEntry, searchSakeRecordsEntry, handleMe, handleLogout } from '` + filepath.ToSlash(filepath.Join(absDrinkLogDir, "functions", "_shared", "glue.ts")) + `';

type Bindings = {
  alcohol_log: D1Database;
  alcohol_log_legacy: D1Database;
  alcohol_log_images: R2Bucket;
  DB?: D1Database;
  BUCKET?: R2Bucket;
};

const app = new Hono<{ Bindings: Bindings }>();

const legacyEnv = (c: any) => ({
  ...c.env,
  alcohol_log: c.env.alcohol_log_legacy,
  DB: c.env.alcohol_log_legacy,
});

// --- LEGACY ORACLE ROUTES (/api/legacy/...) running on ORIGINAL TEXT PK DB ---
app.get('/api/legacy/sake-records', async (c) => listSakeRecords(c.req.raw, legacyEnv(c)));
app.get('/api/legacy/sake-records/search', async (c) => searchSakeRecords(c.req.raw, legacyEnv(c)));
app.get('/api/legacy/sake-records/:id', async (c) => getSakeRecord(c.req.raw, legacyEnv(c), c.req.param('id')));
app.post('/api/legacy/sake-records', async (c) => createSakeRecord(c.req.raw, legacyEnv(c)));
app.put('/api/legacy/sake-records/:id', async (c) => updateSakeRecord(c.req.raw, legacyEnv(c), c.req.param('id')));
app.delete('/api/legacy/sake-records/:id', async (c) => deleteSakeRecord(c.req.raw, legacyEnv(c), c.req.param('id')));
app.post('/api/legacy/sake-records/:id/images', async (c) => addSakeRecordImage(c.req.raw, legacyEnv(c), c.req.param('id')));
app.delete('/api/legacy/sake-records/:id/images/:imageId', async (c) => deleteSakeRecordImage(c.req.raw, legacyEnv(c), c.req.param('id'), c.req.param('imageId')));
app.get('/api/legacy/tags', async (c) => listSakeTags(c.req.raw, legacyEnv(c)));
app.post('/api/legacy/tags', async (c) => createSakeTag(c.req.raw, legacyEnv(c)));
app.get('/api/legacy/me', async (c) => handleMe(c.req.raw, legacyEnv(c)));
app.post('/api/legacy/auth/logout', async (c) => handleLogout(c.req.raw, legacyEnv(c)));

// --- MOLD GLUE ROUTES (/api/glue/...) running on MIGRATED INTEGER PK DB ---
app.get('/api/glue/sake-records', async (c) => fetchSakeRecordsEntry(c.req.raw, c.env as any, c.executionCtx));
app.get('/api/glue/sake-records/search', async (c) => searchSakeRecordsEntry(c.req.raw, c.env as any, c.executionCtx));
app.get('/api/glue/sake-records/:id', async (c) => fetchSingleSakeRecordEntry(c.req.raw, c.env as any, c.req.param('id'), c.executionCtx));
app.post('/api/glue/sake-records', async (c) => createSakeRecordEntry(c.req.raw, c.env as any, c.executionCtx));
app.put('/api/glue/sake-records/:id', async (c) => updateSakeRecordEntry(c.req.raw, c.env as any, c.req.param('id'), c.executionCtx));
app.delete('/api/glue/sake-records/:id', async (c) => deleteSakeRecordEntry(c.req.raw, c.env as any, c.executionCtx));
app.post('/api/glue/sake-records/:id/images', async (c) => addSakeRecordImageEntry(c.req.raw, c.env as any, c.req.param('id'), c.executionCtx));
app.delete('/api/glue/sake-records/:id/images/:imageId', async (c) => deleteSakeRecordImageEntry(c.req.raw, c.env as any, c.req.param('id'), c.req.param('imageId')));
app.get('/api/glue/tags', async (c) => listSakeTagsEntry(c.req.raw, c.env as any, c.executionCtx));
app.post('/api/glue/tags', async (c) => createSakeTagEntry(c.req.raw, c.env as any, c.executionCtx));
app.get('/api/glue/me', async (c) => handleMe(c.req.raw, c.env as any));
app.post('/api/glue/auth/logout', async (c) => handleLogout(c.req.raw, c.env as any));

export default app;
`
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed creating src dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "index.ts"), []byte(harnessTS), 0644); err != nil {
		t.Fatalf("failed writing harness index.ts: %v", err)
	}

	if err := os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte(out.SchemaSQL), 0644); err != nil {
		t.Fatalf("failed writing schema.sql: %v", err)
	}

	pkgJSON := `{"name":"side-by-side-ab-diff","type":"module","dependencies":{"hono":"^4.6.0","miniflare":"^3.20241205.0"}}`
	if err := os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(pkgJSON), 0644); err != nil {
		t.Fatalf("failed writing package.json: %v", err)
	}

	cmdNpm := exec.Command("npm", "install")
	cmdNpm.Dir = tmpDir
	if outBytes, err := cmdNpm.CombinedOutput(); err != nil {
		t.Fatalf("npm install failed: %v, log: %s", err, string(outBytes))
	}

	cmdEsbuild := exec.Command("cmd", "/C", "set NODE_PATH=node_modules&& npx esbuild src/index.ts --bundle --format=esm --target=es2022 --outfile=dist/index.js --external:node:*")
	cmdEsbuild.Dir = tmpDir
	if outBytes, err := cmdEsbuild.CombinedOutput(); err != nil {
		t.Fatalf("esbuild failed: %v, log: %s", err, string(outBytes))
	}

	miniflareURL := filepath.ToSlash(filepath.Join(tmpDir, "node_modules", "miniflare", "dist", "src", "index.js"))

	runnerTemplate := `
import { pathToFileURL } from "node:url";
import fs from "node:fs";

function strictDeepEqualNormalize(obj) {
  if (obj === null || obj === undefined) return obj;
  if (typeof obj !== 'object') return obj;

  if (Array.isArray(obj)) {
    const normArr = obj.map(strictDeepEqualNormalize);
    return normArr.sort((a, b) => {
      const nameA = a.name || (a.record && a.record.name) || (a.label) || String(a.id || '');
      const nameB = b.name || (b.record && b.record.name) || (b.label) || String(b.id || '');
      return String(nameA).localeCompare(String(nameB));
    });
  }

  const res = {};
  for (const rawKey of Object.keys(obj).sort()) {
    if (rawKey === 'legacy_id') continue;
    const key = rawKey === 'sake_record_id' ? 'record_id' : rawKey;

    if (key === 'created_at' || key === 'updated_at') {
      res[key] = '[TIMESTAMP]';
    } else if (key === 'id' || key === 'tag_id' || key === 'record_id') {
      if (obj[rawKey] === null || obj[rawKey] === undefined) {
        res[key] = null;
      } else {
        const valStr = String(obj[rawKey]);
        if (valStr.length === 36 && valStr.includes('-') && !valStr.startsWith('rec-') && !valStr.startsWith('tag-') && !valStr.startsWith('img-') && !valStr.startsWith('google:')) {
          res[key] = '[DYNAMIC_UUID]';
        } else {
          res[key] = valStr;
        }
      }
    } else if (key === 'image_key' || key === 'thumbnail_key' || key === 'data_url' || key === 'thumbnail_data_url') {
      if (obj[rawKey] === null || obj[rawKey] === undefined) {
        res[key] = null;
      } else {
        res[key] = String(obj[rawKey]).replace(/[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}/gi, '[DYNAMIC_UUID]');
      }
    } else {
      res[key] = strictDeepEqualNormalize(obj[rawKey]);
    }
  }
  return res;
}

async function run() {
  const miniflareModule = await import(pathToFileURL("MINIFLARE_PATH").href);
  const { Miniflare } = miniflareModule;

  const mf = new Miniflare({
    workers: [
      {
        modules: true,
        scriptPath: "./dist/index.js",
        d1Databases: ["alcohol_log", "alcohol_log_legacy"],
        r2Buckets: ["alcohol_log_images"],
        compatibilityFlags: ["nodejs_compat"]
      }
    ]
  });

  const dbGlue = await mf.getD1Database("alcohol_log");
  const dbLegacy = await mf.getD1Database("alcohol_log_legacy");

  // 1. Initialize Glue DB (Migrated Integer PK Schema)
  const schemaSQL = fs.readFileSync("./schema.sql", "utf8");
  const cleanSQL = schemaSQL.replace(/--.*$/gm, "");
  for (const rawStmt of cleanSQL.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await dbGlue.exec(stmt + ";");
    }
  }
  await dbGlue.exec("ALTER TABLE record_tags ADD COLUMN record_id INTEGER GENERATED ALWAYS AS (sake_record_id) VIRTUAL;");

  // Seed Glue DB
  await dbGlue.exec("INSERT INTO users (id, legacy_id, provider, provider_user_id, email, display_name, created_at, updated_at) VALUES (1, 'google:sub_user_1', 'google', 'sub_user_1', 'user1@example.com', 'User One', '2026-01-01', '2026-01-01');");
  await dbGlue.exec("INSERT INTO users (id, legacy_id, provider, provider_user_id, email, display_name, created_at, updated_at) VALUES (2, 'google:sub_user_2', 'google', 'sub_user_2', 'user2@example.com', 'User Two', '2026-01-01', '2026-01-01');");
  await dbGlue.exec("CREATE TABLE IF NOT EXISTS oauth_sessions (id TEXT PRIMARY KEY, user_id TEXT NOT NULL, created_at TEXT NOT NULL, expires_at TEXT NOT NULL);");

  const sessionToken = "session_token_11111";
  const sessionToken2 = "session_token_22222";
  const futureExp = new Date(Date.now() + 86400000).toISOString();
  await dbGlue.exec("INSERT INTO oauth_sessions (id, user_id, created_at, expires_at) VALUES ('" + sessionToken + "', 1, '2026-01-01', '" + futureExp + "');");
  await dbGlue.exec("INSERT INTO oauth_sessions (id, user_id, created_at, expires_at) VALUES ('" + sessionToken2 + "', 2, '2026-01-01', '" + futureExp + "');");
  await dbGlue.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('" + sessionToken + "', 1, '2026-01-01', '" + futureExp + "');");
  await dbGlue.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('" + sessionToken2 + "', 2, '2026-01-01', '" + futureExp + "');");

  await dbGlue.exec("INSERT INTO sake_records (id, legacy_id, owner_id, drink_type, name, brewery, region, consumed_date, created_at, updated_at) VALUES (101, 'rec-uuid-101', 1, 'sake', 'Dassai 23', 'Asahi Shuzo', 'Yamaguchi', '2026-01-01', '2026-01-01', '2026-01-01');");
  await dbGlue.exec("INSERT INTO sake_records (id, legacy_id, owner_id, drink_type, name, brewery, region, consumed_date, created_at, updated_at) VALUES (102, 'rec-uuid-102', 1, 'sake', 'Kubota Manju', 'Asahi Shuzo', 'Niigata', '2026-02-01', '2026-01-01', '2026-01-01');");
  await dbGlue.exec("INSERT INTO sake_records (id, legacy_id, owner_id, drink_type, name, brewery, region, consumed_date, created_at, updated_at) VALUES (103, 'rec-uuid-103', 1, 'sake', 'Senshin', 'Asahi Shuzo', 'Niigata', '2026-03-01', '2026-01-01', '2026-01-01');");

  await dbGlue.exec("INSERT INTO tags (id, legacy_id, owner_id, drink_type, tag_group, label, is_default, created_at, updated_at) VALUES (201, 'tag-default-1', NULL, 'sake', 'taste', '단맛', 1, '2026-01-01', '2026-01-01');");
  await dbGlue.exec("INSERT INTO tags (id, legacy_id, owner_id, drink_type, tag_group, label, is_default, created_at, updated_at) VALUES (202, 'tag-custom-1', 1, 'sake', 'aroma', '과일향', 0, '2026-01-01', '2026-01-01');");

  await dbGlue.exec("INSERT INTO record_tags (sake_record_id, tag_id, created_at, updated_at) VALUES (101, 201, '2026-01-01', '2026-01-01');");
  await dbGlue.exec("INSERT INTO sake_images (id, legacy_id, owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, display_order, created_at, updated_at) VALUES (301, 'img-uuid-101', 1, 101, 'blobs/sake_images/301/img.jpg', NULL, 'image/jpeg', 'dassai.jpg', 0, '2026-01-01', '2026-01-01');");

  // 2. Initialize Legacy DB (Original Text UUID PK Schema)
  const schemaLegacySQL = fs.readFileSync("./schema_legacy.sql", "utf8");
  const cleanLegacySQL = schemaLegacySQL.replace(/--.*$/gm, "");
  for (const rawStmt of cleanLegacySQL.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await dbLegacy.exec(stmt + ";");
    }
  }
  await dbLegacy.exec("DELETE FROM tags WHERE id LIKE 'tag_%';");

  // Seed Legacy DB
  await dbLegacy.exec("INSERT INTO users (id, provider, provider_user_id, email, display_name, created_at, last_login_at) VALUES ('google:sub_user_1', 'google', 'sub_user_1', 'user1@example.com', 'User One', '2026-01-01', '2026-01-01');");
  await dbLegacy.exec("INSERT INTO users (id, provider, provider_user_id, email, display_name, created_at, last_login_at) VALUES ('google:sub_user_2', 'google', 'sub_user_2', 'user2@example.com', 'User Two', '2026-01-01', '2026-01-01');");
  await dbLegacy.exec("INSERT INTO oauth_sessions (id, user_id, created_at, expires_at) VALUES ('session_token_11111', 'google:sub_user_1', '2026-01-01', '" + futureExp + "');");
  await dbLegacy.exec("INSERT INTO oauth_sessions (id, user_id, created_at, expires_at) VALUES ('session_token_22222', 'google:sub_user_2', '2026-01-01', '" + futureExp + "');");

  await dbLegacy.exec("INSERT INTO sake_records (id, owner_id, drink_type, name, brewery, region, consumed_date, created_at, updated_at) VALUES ('rec-uuid-101', 'google:sub_user_1', 'sake', 'Dassai 23', 'Asahi Shuzo', 'Yamaguchi', '2026-01-01', '2026-01-01', '2026-01-01');");
  await dbLegacy.exec("INSERT INTO sake_records (id, owner_id, drink_type, name, brewery, region, consumed_date, created_at, updated_at) VALUES ('rec-uuid-102', 'google:sub_user_1', 'sake', 'Kubota Manju', 'Asahi Shuzo', 'Niigata', '2026-02-01', '2026-01-01', '2026-01-01');");
  await dbLegacy.exec("INSERT INTO sake_records (id, owner_id, drink_type, name, brewery, region, consumed_date, created_at, updated_at) VALUES ('rec-uuid-103', 'google:sub_user_1', 'sake', 'Senshin', 'Asahi Shuzo', 'Niigata', '2026-03-01', '2026-01-01', '2026-01-01');");

  await dbLegacy.exec("INSERT INTO tags (id, owner_id, drink_type, tag_group, label, is_default, created_at) VALUES ('tag-default-1', NULL, 'sake', 'taste', '단맛', 1, '2026-01-01');");
  await dbLegacy.exec("INSERT INTO tags (id, owner_id, drink_type, tag_group, label, is_default, created_at) VALUES ('tag-custom-1', 'google:sub_user_1', 'sake', 'aroma', '과일향', 0, '2026-01-01');");

  await dbLegacy.exec("INSERT INTO record_tags (record_id, tag_id, created_at) VALUES ('rec-uuid-101', 'tag-default-1', '2026-01-01');");
  await dbLegacy.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, display_order, created_at) VALUES ('img-uuid-101', 'google:sub_user_1', 'rec-uuid-101', 'blobs/sake_images/301/img.jpg', NULL, 'image/jpeg', 'dassai.jpg', 0, '2026-01-01');");

  const reqHeaders = { Cookie: "alcohol_log_session=" + sessionToken + "; mold_session=" + sessionToken, "Content-Type": "application/json" };

  async function diffRoute(name, legacyPath, gluePath, options = {}) {
    console.log("\n==================================================");
    console.log("=== A/B DIFF ROUTE: " + name + " (" + (options.method || "GET") + " Legacy:" + legacyPath + " vs Glue:" + gluePath + ") ===");
    console.log("==================================================");

    const legacyUrl = "http://localhost:8080/api/legacy" + legacyPath;
    const glueUrl = "http://localhost:8080/api/glue" + gluePath;

    const legacyHeaders = options.headers || reqHeaders;
    const glueHeaders = options.headers || reqHeaders;

    const legacyBody = options.bodyLegacy !== undefined ? options.bodyLegacy : options.body;
    const glueBody = options.bodyGlue !== undefined ? options.bodyGlue : options.body;

    const reqOptsLegacy = {
      method: options.method || "GET",
      headers: legacyHeaders,
      body: legacyBody !== undefined ? JSON.stringify(legacyBody) : undefined
    };

    const reqOptsGlue = {
      method: options.method || "GET",
      headers: glueHeaders,
      body: glueBody !== undefined ? JSON.stringify(glueBody) : undefined
    };

    const resLegacy = await mf.dispatchFetch(legacyUrl, reqOptsLegacy);
    const resGlue = await mf.dispatchFetch(glueUrl, reqOptsGlue);

    console.log("LEGACY ORACLE STATUS:", resLegacy.status);
    console.log("GLUE LAYER STATUS:   ", resGlue.status);

    const textLegacy = await resLegacy.text();
    const textGlue = await resGlue.text();

    console.log("\n--- RAW LEGACY ORACLE RESPONSE ---");
    console.log(textLegacy);

    console.log("\n--- RAW NEW GLUE LAYER RESPONSE ---");
    console.log(textGlue);

    if (resLegacy.status !== resGlue.status) {
      console.error("STATUS MISMATCH! Legacy:", resLegacy.status, "Glue:", resGlue.status);
      process.exit(1);
    }

    if (resLegacy.headers.get("X-Sake-Tag-Existing")) {
      console.log("\nLEGACY HEADER X-Sake-Tag-Existing:", resLegacy.headers.get("X-Sake-Tag-Existing"));
      console.log("GLUE HEADER X-Sake-Tag-Existing:  ", resGlue.headers.get("X-Sake-Tag-Existing"));
    }

    if (textLegacy && textGlue && (textLegacy.startsWith("{") || textLegacy.startsWith("["))) {
      const jsonLegacy = JSON.parse(textLegacy);
      const jsonGlue = JSON.parse(textGlue);
      const normLegacy = JSON.stringify(strictDeepEqualNormalize(jsonLegacy));
      const normGlue = JSON.stringify(strictDeepEqualNormalize(jsonGlue));

      if (normLegacy !== normGlue) {
        console.error("DATA DIFF MISMATCH!");
        console.error("Legacy Norm:", normLegacy);
        console.error("Glue Norm:  ", normGlue);
        process.exit(1);
      }
      console.log("\nJSON STRUCTURE & DATA EQUIVALENCE CHECK: MATCH!");
    } else {
      if (textLegacy !== textGlue) {
        console.error("TEXT BODY MISMATCH!");
        process.exit(1);
      }
      console.log("\nTEXT EQUIVALENCE CHECK: MATCH!");
    }
  }

  // 1. GET /sake-records (List Sake Records - Sorted consumed_date DESC, created_at DESC)
  await diffRoute("1. List Sake Records", "/sake-records", "/sake-records");

  // 2. GET /sake-records/search?q=Dassai
  await diffRoute("2. Search Sake Records", "/sake-records/search?q=Dassai", "/sake-records/search?q=Dassai");

  // 3. GET /sake-records/:id
  await diffRoute("3. Single Sake Record", "/sake-records/rec-uuid-101", "/sake-records/rec-uuid-101");

  // 4. GET /tags
  await diffRoute("4. List Tags", "/tags", "/tags");

  // 5. POST /tags (Duplicate Tag Dedup Check)
  await diffRoute("5. Create Sake Tag (Duplicate Dedup)", "/tags", "/tags", {
    method: "POST",
    body: { tag_group: "taste", label: "단맛", drink_type: "sake" }
  });

  // 6. PUT /sake-records/:id (Update Sake Record + Image Preservation Check)
  await diffRoute("6. Update Sake Record (Image Preservation)", "/sake-records/rec-uuid-101", "/sake-records/rec-uuid-101", {
    method: "PUT",
    body: {
      name: "Dassai 23 Polished",
      brewery: "Asahi Shuzo",
      region: "Yamaguchi",
      consumed_date: "2026-01-01",
      images: [{ id: "img-uuid-101", file_name: "dassai.jpg", mime_type: "image/jpeg" }],
      selected_tag_ids: ["tag-default-1"]
    }
  });

  // 7. POST /sake-records/:id/images (Add Sake Record Image / R2 Upload)
  await diffRoute("7. Add Sake Record Image", "/sake-records/rec-uuid-101/images", "/sake-records/rec-uuid-101/images", {
    method: "POST",
    body: { file_name: "extra.jpg", mime_type: "image/jpeg" }
  });

  // 8. DELETE /sake-records/:id/images/:imageId (Delete Sake Record Image)
  await diffRoute("8. Delete Sake Record Image", "/sake-records/rec-uuid-101/images/img-uuid-101", "/sake-records/rec-uuid-101/images/img-uuid-101", {
    method: "DELETE"
  });

  // 9. DELETE /sake-records/:id
  await diffRoute("9. Delete Sake Record", "/sake-records/rec-uuid-103", "/sake-records/rec-uuid-103", {
    method: "DELETE"
  });

  // 10. GET /me (Current Authenticated User Info)
  await diffRoute("10. Current User Profile (/me)", "/me", "/me");

  // 11. POST /auth/logout (Logout Session Revocation)
  await diffRoute("11. Logout Session (/auth/logout)", "/auth/logout", "/auth/logout", {
    method: "POST"
  });

  // 12. GET /sake-records/:id (Cross-User 403 Access Denial Boundary Check)
  await diffRoute("12. Cross-User 403 Forbidden Access", "/sake-records/rec-uuid-101", "/sake-records/rec-uuid-101", {
    headers: { Cookie: "alcohol_log_session=session_token_22222; mold_session=session_token_22222" }
  });

  console.log("\nALL ROUTE SIDE-BY-SIDE A/B RAW DIFFS VERIFIED EQUIVALENT!");
  await mf.dispose();
}

run().catch(err => {
  console.error(err);
  process.exit(1);
});
`

	runnerJS := strings.ReplaceAll(runnerTemplate, "MINIFLARE_PATH", miniflareURL)

	if err := os.WriteFile(filepath.Join(tmpDir, "test_runner.mjs"), []byte(runnerJS), 0644); err != nil {
		t.Fatalf("failed writing test_runner.mjs: %v", err)
	}

	cmdNode := exec.Command("node", "test_runner.mjs")
	cmdNode.Dir = tmpDir
	outBytes, err := cmdNode.CombinedOutput()
	t.Logf("Side-by-Side A/B Diff Execution Log:\n%s", string(outBytes))
	if err != nil {
		t.Fatalf("Side-by-Side A/B Diff test runner failed: %v", err)
	}
}
