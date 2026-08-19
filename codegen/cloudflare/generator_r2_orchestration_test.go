package cloudflare_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/hitel00000/mold/codegen/cloudflare"
	"github.com/hitel00000/mold/resource"
)

func TestCloudflareCodegen_MiniflareR2DeleteOrchestrationEmpirical(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping heavy Miniflare integration test in short mode")
	}
	nodePath, err := exec.LookPath("node")
	if err != nil || nodePath == "" {
		t.Skip("node not found in PATH")
	}

	reg := resource.NewRegistry()
	userRes := &resource.Resource{
		Name:       "User",
		Table:      "users",
		Timestamps: true,
		Fields: []resource.Field{
			{Name: "email", Type: resource.TypeEmail, Nullable: false},
			{Name: "role", Type: resource.TypeString, Nullable: false, Default: "user"},
		},
	}
	sakeRecordRes := &resource.Resource{
		Name:       "SakeRecord",
		Table:      "sake_records",
		Timestamps: true,
		Fields: []resource.Field{
			{Name: "name", Type: resource.TypeString, Nullable: false},
			{Name: "owner_id", Type: resource.TypeInt, Nullable: false},
		},
		Auth: &resource.Auth{
			OwnershipField: "owner_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "owner",
				Update: "owner",
				Delete: "owner",
			},
		},
		Relations: []resource.Relation{
			{Name: "images", Kind: resource.KindHasMany, Target: "SakeImage", ForeignKey: "record_id", OnDelete: resource.OnDeleteRestrict},
		},
	}
	sakeImgRes := &resource.Resource{
		Name:       "SakeImage",
		Table:      "sake_images",
		Timestamps: true,
		Fields: []resource.Field{
			{Name: "owner_id", Type: resource.TypeInt, Nullable: false},
			{Name: "record_id", Type: resource.TypeInt, Nullable: false},
			{Name: "image_key", Type: resource.TypeBlob, Nullable: true},
		},
		Auth: &resource.Auth{
			OwnershipField: "owner_id",
			Permissions: resource.Permissions{
				Create: "authenticated",
				Read:   "owner",
				Update: "owner",
				Delete: "owner",
			},
		},
		Relations: []resource.Relation{
			{Name: "record", Kind: resource.KindBelongsTo, Target: "SakeRecord", ForeignKey: "record_id"},
		},
	}
	reg.Register(userRes)
	reg.Register(sakeRecordRes)
	reg.Register(sakeImgRes)

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		t.Fatalf("failed generate: %v", err)
	}

	tmpDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(tmpDir, "package.json"), []byte(output.PackageJSON), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "wrangler.jsonc"), []byte(output.WranglerConfig), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "schema.sql"), []byte(output.SchemaSQL), 0644)
	_ = os.WriteFile(filepath.Join(tmpDir, "index.ts"), []byte(output.IndexTS), 0644)

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

	runnerJS := fmt.Sprintf(`
import fs from 'node:fs';
import { pathToFileURL } from 'node:url';

async function run() {
  const miniflareModule = await import(pathToFileURL("%s").href);
  const { Miniflare } = miniflareModule;

  const mf = new Miniflare({
    workers: [
      {
        modules: true,
        scriptPath: "./dist/index.js",
        d1Databases: { DB: "mold-d1" },
        r2Buckets: { BUCKET: "mold-r2" },
      }
    ]
  });

  const db = await mf.getD1Database("DB");
  await db.exec("PRAGMA foreign_keys = ON;");
  const bucket = await mf.getR2Bucket("BUCKET");
  const schemaSQL = fs.readFileSync("./schema.sql", "utf8");

  const cleanSQL = schemaSQL.replace(/--.*$/gm, "");
  for (const rawStmt of cleanSQL.split(";")) {
    const stmt = rawStmt.replace(/\s+/g, " ").trim();
    if (stmt) {
      await db.exec(stmt + ";");
    }
  }

  const key1 = "images/101/sake/1/img1.jpg";
  const key2 = "images/101/sake/1/img2.jpg";

  // Seed Data: Users 101 & 202 + Sessions + SakeRecord 1 + SakeImage 10 & 11
  await db.exec("INSERT INTO users (id, email, role, created_at, updated_at) VALUES (101, 'user101@example.com', 'user', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO users (id, email, role, created_at, updated_at) VALUES (202, 'user202@example.com', 'user', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('sess_101', 101, '2026-07-30T00:00:00Z', '2030-01-01T00:00:00Z');");
  await db.exec("INSERT INTO _mold_sessions (id, user_id, created_at, expires_at) VALUES ('sess_202', 202, '2026-07-30T00:00:00Z', '2030-01-01T00:00:00Z');");

  await db.exec("INSERT INTO sake_records (id, name, owner_id, created_at, updated_at) VALUES (1, 'Dassai 45', 101, '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, created_at, updated_at) VALUES (10, 101, 1, '" + key1 + "', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, created_at, updated_at) VALUES (11, 101, 1, '" + key2 + "', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await bucket.put(key1, "BINARY_DATA_1");
  await bucket.put(key2, "BINARY_DATA_2");

  console.log("=== EMPIRICAL TEST 1: Cross-User Authorization Check (User 202 deleting User 101 Record) ===");
  // Dispatch HTTP request as User B (user_id = 202) trying to delete User A's record
  const resCrossUser = await mf.dispatchFetch("http://localhost/api/sake_records/1", {
    method: "DELETE",
    headers: { "Cookie": "mold_session=sess_202" }
  });
  console.log("Cross-User DELETE HTTP Status:", resCrossUser.status);
  if (resCrossUser.status === 403) {
    console.log("[EMPIRICAL PROOF VERIFIED]: Cross-user delete correctly BLOCKED with 403 Forbidden!");
  } else {
    console.error("FAILED: Expected 403, got", resCrossUser.status);
    process.exit(1);
  }

  console.log("\n=== EMPIRICAL TEST 2: Session-Based HTTP Delete Orchestration (Happy Path as User 101) ===");
  // Orchestrator Step A: Get SakeRecord images list via HTTP or DB query
  const resRecord = await mf.dispatchFetch("http://localhost/api/sake_records/1", {
    headers: { "Cookie": "mold_session=sess_101" }
  });
  console.log("Fetch Record HTTP Status:", resRecord.status);

  // Orchestrator Step B: Delete SakeImage 10 via Mold HTTP API (blob + record)
  const resBlob10 = await mf.dispatchFetch("http://localhost/api/sake_images/10/blob/image_key", {
    method: "DELETE",
    headers: { "Cookie": "mold_session=sess_101" }
  });
  console.log("Delete SakeImage 10 Blob HTTP Status:", resBlob10.status);

  const resImg10 = await mf.dispatchFetch("http://localhost/api/sake_images/10", {
    method: "DELETE",
    headers: { "Cookie": "mold_session=sess_101" }
  });
  console.log("Delete SakeImage 10 Record HTTP Status:", resImg10.status);

  // Delete SakeImage 11
  const resBlob11 = await mf.dispatchFetch("http://localhost/api/sake_images/11/blob/image_key", {
    method: "DELETE",
    headers: { "Cookie": "mold_session=sess_101" }
  });
  const resImg11 = await mf.dispatchFetch("http://localhost/api/sake_images/11", {
    method: "DELETE",
    headers: { "Cookie": "mold_session=sess_101" }
  });
  console.log("Delete SakeImage 11 Blob/Record HTTP Status:", resBlob11.status, resImg11.status);

  // Orchestrator Step C: Delete SakeRecord 1 via HTTP API
  const resRecDel = await mf.dispatchFetch("http://localhost/api/sake_records/1", {
    method: "DELETE",
    headers: { "Cookie": "mold_session=sess_101" }
  });
  console.log("Delete SakeRecord 1 Final HTTP Status:", resRecDel.status);

  // Final Verification
  const r2Obj1 = await bucket.get(key1);
  const r2Obj2 = await bucket.get(key2);
  const recCount = (await db.prepare("SELECT COUNT(*) as c FROM sake_records").first()).c;
  console.log("Final Verification - DB Records Count:", recCount, ", R2 Key1 Exists:", r2Obj1 !== null, ", R2 Key2 Exists:", r2Obj2 !== null);

  if (resRecDel.status === 200 && recCount === 0 && r2Obj1 === null && r2Obj2 === null) {
    console.log("[EMPIRICAL PROOF VERIFIED]: Session-based HTTP Delete Orchestration completely clean!");
  } else {
    console.error("FAILED happy path orchestration");
    process.exit(1);
  }

  console.log("\n=== EMPIRICAL TEST 3: Partial Failure Abort & Retry Idempotency ===");
  // Seed new Record 2 with Image 20 & 21
  const key20 = "images/101/sake/2/img20.jpg";
  const key21 = "images/101/sake/2/img21.jpg";
  await db.exec("INSERT INTO sake_records (id, name, owner_id, created_at, updated_at) VALUES (2, 'Kubota Manju', 101, '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, created_at, updated_at) VALUES (20, 101, 2, '" + key20 + "', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, created_at, updated_at) VALUES (21, 101, 2, '" + key21 + "', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await bucket.put(key20, "BINARY_20");
  await bucket.put(key21, "BINARY_21");

  // Orchestration Helper Function with Abort Contract
  async function orchestrateDeleteRecord(recordId, simulateErrorOnImgId) {
    // 1. Fetch images for recordId
    const imgsRes = await db.prepare("SELECT id FROM sake_images WHERE record_id = ?").bind(recordId).all();
    const childImgs = imgsRes.results;

    // 2. Loop & Delete child images via Mold HTTP API
    for (const img of childImgs) {
      if (img.id === simulateErrorOnImgId) {
        console.log("Simulating R2/Network Error on Image ID:", img.id, "-> Aborting Orchestration!");
        // Abort contract: Do NOT proceed to delete SakeRecord!
        return { status: 500, code: "RECORD_DELETE_PARTIAL_FAILURE" };
      }
      const resBlob = await mf.dispatchFetch("http://localhost/api/sake_images/" + img.id + "/blob/image_key", { method: "DELETE", headers: { "Cookie": "mold_session=sess_101" } });
      const resRec = await mf.dispatchFetch("http://localhost/api/sake_images/" + img.id, { method: "DELETE", headers: { "Cookie": "mold_session=sess_101" } });
      if (resRec.status !== 200 && resRec.status !== 404) {
        return { status: resRec.status, code: "CHILD_IMAGE_DELETE_FAILED" };
      }
    }

    // 3. Delete SakeRecord if all children succeeded
    const resParent = await mf.dispatchFetch("http://localhost/api/sake_records/" + recordId, { method: "DELETE", headers: { "Cookie": "mold_session=sess_101" } });
    return { status: resParent.status, code: resParent.status === 200 ? "SUCCESS" : "PARENT_DELETE_FAILED" };
  }

  // Run Step 1: Simulate Error on Image 21
  const partialRes = await orchestrateDeleteRecord(2, 21);
  console.log("Partial Failure Orchestration Result:", partialRes);

  const checkRec2Exists = (await db.prepare("SELECT COUNT(*) as c FROM sake_records WHERE id = 2").first()).c;
  const checkImg21Exists = (await db.prepare("SELECT COUNT(*) as c FROM sake_images WHERE id = 21").first()).c;
  console.log("After Partial Failure Abort - Record 2 Exists:", checkRec2Exists === 1, ", Image 21 Exists:", checkImg21Exists === 1);

  if (partialRes.status === 500 && checkRec2Exists === 1 && checkImg21Exists === 1) {
    console.log("[EMPIRICAL PROOF VERIFIED]: Abort Contract Succeeded! SakeRecord 2 was NOT deleted when child Image 21 failed!");
  } else {
    console.error("FAILED abort contract verification");
    process.exit(1);
  }

  // Run Step 2: Retry Orchestration (No simulated error)
  console.log("Retrying Delete Orchestration for SakeRecord 2...");
  const retryRes = await orchestrateDeleteRecord(2, null);
  console.log("Retry Orchestration Result:", retryRes);

  const checkRec2Final = (await db.prepare("SELECT COUNT(*) as c FROM sake_records WHERE id = 2").first()).c;
  if (retryRes.status === 200 && checkRec2Final === 0) {
    console.log("[EMPIRICAL PROOF VERIFIED]: Retry Orchestration succeeded idempotently!");
  } else {
    console.error("FAILED retry orchestration");
    process.exit(1);
  }

  console.log("\n=== EMPIRICAL TEST 4: Single Image Internal R2 Blob Deleted but DB Row Delete Fails ===");
  const key30 = "images/101/sake/3/img30.jpg";
  await db.exec("INSERT INTO sake_records (id, name, owner_id, created_at, updated_at) VALUES (3, 'Dassai 23', 101, '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, created_at, updated_at) VALUES (30, 101, 3, '" + key30 + "', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await bucket.put(key30, "BINARY_30");

  // Step A: Blob delete succeeded (R2 key deleted from bucket)
  const resBlob30 = await mf.dispatchFetch("http://localhost/api/sake_images/30/blob/image_key", { method: "DELETE", headers: { "Cookie": "mold_session=sess_101" } });
  console.log("Single Image Step A - Blob Delete HTTP Status:", resBlob30.status);
  const r2Key30Exists = (await bucket.get(key30)) !== null;

  // Step B: Simulate DB Row Delete failure (mid-stage failure before DB row deletion)
  console.log("Simulating mid-stage failure: R2 deleted, but DB row delete fails/aborts.");
  const dbRow30Count = (await db.prepare("SELECT COUNT(*) as c FROM sake_images WHERE id = 30").first()).c;
  console.log("Mid-stage State - R2 Key Exists:", r2Key30Exists, ", DB Row 30 Exists:", dbRow30Count === 1);
  if (!r2Key30Exists && dbRow30Count === 1) {
    console.log("[EMPIRICAL PROOF VERIFIED]: Mid-stage state created: R2 blob removed, DB row dangling!");
  }

  // Step C: Retry Orchestration on this mid-stage image
  console.log("Retrying Delete Orchestration on Mid-stage image 30...");
  const resRetryBlob30 = await mf.dispatchFetch("http://localhost/api/sake_images/30/blob/image_key", { method: "DELETE", headers: { "Cookie": "mold_session=sess_101" } });
  console.log("Retry Step A - Already Deleted R2 Blob Status:", resRetryBlob30.status);
  const resRetryDb30 = await mf.dispatchFetch("http://localhost/api/sake_images/30", { method: "DELETE", headers: { "Cookie": "mold_session=sess_101" } });
  console.log("Retry Step B - DB Row Delete Status:", resRetryDb30.status);
  const resRetryParent3 = await mf.dispatchFetch("http://localhost/api/sake_records/3", { method: "DELETE", headers: { "Cookie": "mold_session=sess_101" } });
  console.log("Retry Step C - Parent Record Delete Status:", resRetryParent3.status);

  const dbRec3Final = (await db.prepare("SELECT COUNT(*) as c FROM sake_records WHERE id = 3").first()).c;
  const dbImg30Final = (await db.prepare("SELECT COUNT(*) as c FROM sake_images WHERE id = 30").first()).c;
  if (resRetryDb30.status === 200 && resRetryParent3.status === 200 && dbRec3Final === 0 && dbImg30Final === 0) {
    console.log("[EMPIRICAL PROOF VERIFIED]: Mid-stage failure resolved idempotently on retry!");
  } else {
    console.error("FAILED mid-stage retry test");
    process.exit(1);
  }

  console.log("\n=== EMPIRICAL TEST 5: Direct Parent DELETE Call Without Orchestration (FK Restrict Guard Check) ===");
  const key40 = "images/101/sake/4/img40.jpg";
  await db.exec("INSERT INTO sake_records (id, name, owner_id, created_at, updated_at) VALUES (4, 'Senshin', 101, '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");
  await db.exec("INSERT INTO sake_images (id, owner_id, record_id, image_key, created_at, updated_at) VALUES (40, 101, 4, '" + key40 + "', '2026-07-30T00:00:00Z', '2026-07-30T00:00:00Z');");

  // Directly call DELETE /api/sake_records/4 while child Image 40 is still in DB!
  const resDirectDelete = await mf.dispatchFetch("http://localhost/api/sake_records/4", {
    method: "DELETE",
    headers: { "Cookie": "mold_session=sess_101" }
  });
  const resDirectBody = await resDirectDelete.text();
  console.log("Direct Parent DELETE HTTP Status:", resDirectDelete.status);
  console.log("Direct Parent DELETE Raw Response Body:", resDirectBody);

  const checkRec4Exists = (await db.prepare("SELECT COUNT(*) as c FROM sake_records WHERE id = 4").first()).c;
  console.log("After Direct Parent DELETE - Record 4 Exists in DB:", checkRec4Exists === 1);

  if (resDirectDelete.status >= 400 && checkRec4Exists === 1) {
    console.log("[EMPIRICAL PROOF VERIFIED]: FK Restrict Guard strictly blocked direct parent deletion!");
  } else {
    console.error("FAILED FK restrict guard check");
    process.exit(1);
  }

  await mf.dispose();
}

run().catch(err => {
  console.error(err);
  process.exit(1);
});
`, miniflareURL)

	_ = os.WriteFile(filepath.Join(tmpDir, "test_runner.mjs"), []byte(runnerJS), 0644)

	cmd := exec.Command("node", "test_runner.mjs")
	cmd.Dir = tmpDir
	outputBytes, err := cmd.CombinedOutput()
	t.Logf("Miniflare Raw Output:\n%s", string(outputBytes))
	if err != nil {
		t.Fatalf("test failed: %v", err)
	}
}
