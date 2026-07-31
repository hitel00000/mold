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
		t.Fatalf("generation failed: %v", err)
	}

	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(output.PackageJSON), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "wrangler.jsonc"), []byte(output.WranglerConfig), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte(output.SchemaSQL), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "index.ts"), []byte(output.IndexTS), 0644)

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
  const r1_uuid = "uuid_record_1";
  const img1_uuid = "uuid_img_10";
  const img1_key = "images/" + u1_uuid + "/sake/" + r1_uuid + "/img1.jpg";

  await db.exec("INSERT INTO users VALUES ('" + u1_uuid + "', 'google', 'g_101', 'user101@example.com', 'User 101', 'https://avatar', '2026-07-30T00:00:00Z', 'user', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_records (id, owner_id, drink_type, name, consumed_date, created_at, updated_at) VALUES ('" + r1_uuid + "', '" + u1_uuid + "', 'sake', 'Dassai 23 Legacy', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, mime_type, file_name, created_at, updated_at) VALUES ('" + img1_uuid + "', '" + u1_uuid + "', '" + r1_uuid + "', '" + img1_key + "', 'image/jpeg', 'img1.jpg', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await bucket.put(img1_key, "LEGACY_BINARY_R2_DATA");

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
  const migratedRecord = await db.prepare("SELECT * FROM sake_records WHERE legacy_id = ?").bind(r1_uuid).first();
  const migratedImage = await db.prepare("SELECT * FROM sake_images WHERE legacy_id = ?").bind(img1_uuid).first();
  const r2KeyPreserved = await bucket.get(img1_key);

  console.log("Migrated User Int ID:", migratedUser.id, ", Legacy ID:", migratedUser.legacy_id);
  console.log("Migrated SakeRecord Int ID:", migratedRecord.id, ", FK owner_id:", migratedRecord.owner_id);
  console.log("Migrated SakeImage Int ID:", migratedImage.id, ", FK record_id:", migratedImage.record_id);
  console.log("R2 Preserved Key Exists:", r2KeyPreserved !== null);

  if (typeof migratedUser.id === 'number' && migratedRecord.owner_id === migratedUser.id && migratedImage.record_id === migratedRecord.id && r2KeyPreserved !== null) {
    console.log("[EMPIRICAL MIGRATION VERIFIED]: D1 Migration Script 100% Succeeded! INTEGER PKs and R2 Keys Preserved!");
  } else {
    console.error("FAILED D1 Migration Verification");
    process.exit(1);
  }

  console.log("\n=== STEP 3: Issuing Session & Testing Session Cookie Authentication ===");
  const now = new Date().toISOString();
  const sessToken = "sess_e2e_101";
  await db.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('" + sessToken + "', " + migratedUser.id + ", '" + now + "', '2030-01-01T00:00:00Z');");

  const resFetchRec = await mf.dispatchFetch("http://localhost/api/sake_records/" + migratedRecord.id, {
    headers: { "Cookie": "mold_session=" + sessToken }
  });
  console.log("Fetch SakeRecord via mold_session Cookie Status:", resFetchRec.status);

  if (resFetchRec.status === 200) {
    console.log("[EMPIRICAL SESSION VERIFIED]: Session issued and authenticated successfully!");
  } else {
    console.error("FAILED session authentication");
    process.exit(1);
  }

  console.log("\n=== STEP 4: Testing Delete Orchestration on Migrated Record ===");
  // Step A: Delete Image Blob
  const resBlobDel = await mf.dispatchFetch("http://localhost/api/sake_images/" + migratedImage.id + "/blob/image_key", {
    method: "DELETE", headers: { "Cookie": "mold_session=" + sessToken }
  });
  console.log("Delete SakeImage Blob Status:", resBlobDel.status);

  // Step B: Delete Image Row
  const resImgRowDel = await mf.dispatchFetch("http://localhost/api/sake_images/" + migratedImage.id, {
    method: "DELETE", headers: { "Cookie": "mold_session=" + sessToken }
  });
  console.log("Delete SakeImage Row Status:", resImgRowDel.status);

  // Step C: Delete SakeRecord
  const resRecordDel = await mf.dispatchFetch("http://localhost/api/sake_records/" + migratedRecord.id, {
    method: "DELETE", headers: { "Cookie": "mold_session=" + sessToken }
  });
  console.log("Delete SakeRecord Final Status:", resRecordDel.status);

  const finalRecordCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_records").first()).c;
  const finalImageCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_images").first()).c;

  console.log("Final State - Record Count:", finalRecordCount, ", Image Count:", finalImageCount);
  if (resRecordDel.status === 200 && finalRecordCount === 0 && finalImageCount === 0) {
    console.log("[EMPIRICAL ORCHESTRATION VERIFIED]: Delete Orchestration on Migrated Data 100% Clean!");
  } else {
    console.error("FAILED delete orchestration");
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

	if !strings.Contains(rawOutput, "[EMPIRICAL MIGRATION VERIFIED]") || !strings.Contains(rawOutput, "[EMPIRICAL ORCHESTRATION VERIFIED]") {
		t.Fatalf("empirical verification markers not found in output:\n%s", rawOutput)
	}
}
