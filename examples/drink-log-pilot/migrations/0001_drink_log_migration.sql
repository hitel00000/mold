-- 0001_drink_log_migration.sql
-- Production D1 Migration Script for Drink-Log Service
-- Option C: INTEGER AUTOINCREMENT PK + Legacy UUID Preservation + R2 Key Preservation

PRAGMA foreign_keys = OFF;

BEGIN TRANSACTION;

-- ============================================================================
-- 1. STEP 1: users_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "_tmp_user_map" (
    "old_id" TEXT PRIMARY KEY,
    "new_id" INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS "users_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "legacy_id" TEXT UNIQUE,
    "provider" TEXT NOT NULL,
    "provider_user_id" TEXT NOT NULL,
    "email" TEXT,
    "display_name" TEXT,
    "avatar_url" TEXT,
    "last_login_at" TEXT,
    "role" TEXT CHECK ("role" IN ('admin', 'user')) NOT NULL DEFAULT 'user',
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL
);

INSERT INTO "users_new" ("legacy_id", "provider", "provider_user_id", "email", "display_name", "avatar_url", "last_login_at", "role", "created_at", "updated_at")
SELECT "id", "provider", "provider_user_id", "email", "display_name", "avatar_url", "last_login_at", COALESCE("role", 'user'), "created_at", COALESCE("updated_at", "created_at")
FROM "users" ORDER BY "created_at" ASC;

INSERT INTO "_tmp_user_map" ("old_id", "new_id")
SELECT "legacy_id", "id" FROM "users_new";

CREATE UNIQUE INDEX IF NOT EXISTS "idx_users_provider_provider_user_id_unique" ON "users_new"("provider", "provider_user_id");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_users_legacy_id_unique" ON "users_new"("legacy_id");


-- ============================================================================
-- 2. STEP 2: tags_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "_tmp_tag_map" (
    "old_id" TEXT PRIMARY KEY,
    "new_id" INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS "tags_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "legacy_id" TEXT UNIQUE,
    "slug" TEXT UNIQUE,
    "owner_id" INTEGER,
    "drink_type" TEXT NOT NULL DEFAULT 'sake',
    "tag_group" TEXT CHECK ("tag_group" IN ('taste', 'aroma', 'mood')) NOT NULL,
    "label" TEXT NOT NULL,
    "is_default" INTEGER NOT NULL DEFAULT 0,
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL
);

INSERT INTO "tags_new" ("legacy_id", "slug", "owner_id", "drink_type", "tag_group", "label", "is_default", "created_at", "updated_at")
SELECT 
    t."id",
    CASE WHEN t."is_default" = 1 THEN 'tag_' || t."tag_group" || '_' || lower(replace(t."label", ' ', '_')) ELSE NULL END,
    u."new_id",
    COALESCE(t."drink_type", 'sake'),
    t."tag_group",
    t."label",
    t."is_default",
    t."created_at",
    COALESCE(t."updated_at", t."created_at")
FROM "tags" t
LEFT JOIN "_tmp_user_map" u ON t."owner_id" = u."old_id"
ORDER BY t."created_at" ASC;

INSERT INTO "_tmp_tag_map" ("old_id", "new_id")
SELECT "legacy_id", "id" FROM "tags_new";

CREATE UNIQUE INDEX IF NOT EXISTS "idx_tags_owner_id_drink_type_tag_group_label_unique" ON "tags_new"("owner_id", "drink_type", "tag_group", "label");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_tags_slug_unique" ON "tags_new"("slug");
CREATE UNIQUE INDEX IF NOT EXISTS "idx_tags_legacy_id_unique" ON "tags_new"("legacy_id");


-- ============================================================================
-- 3. STEP 3: sake_records_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "_tmp_sake_map" (
    "old_id" TEXT PRIMARY KEY,
    "new_id" INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS "sake_records_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "legacy_id" TEXT UNIQUE,
    "owner_id" INTEGER NOT NULL,
    "drink_type" TEXT NOT NULL DEFAULT 'sake',
    "name" TEXT NOT NULL,
    "region" TEXT,
    "brewery" TEXT,
    "rice" TEXT,
    "sake_type" TEXT,
    "sake_meter_value" TEXT,
    "abv" TEXT,
    "volume" TEXT,
    "price" TEXT,
    "one_line_note" TEXT,
    "notes" TEXT,
    "rating" REAL,
    "place" TEXT,
    "companions" TEXT,
    "food_pairing" TEXT,
    "consumed_date" TEXT NOT NULL,
    "drink_again" TEXT CHECK ("drink_again" IN ('no', 'unsure', 'yes')),
    "sweet_dry" INTEGER CHECK ("sweet_dry" BETWEEN 1 AND 5),
    "aroma_intensity" INTEGER CHECK ("aroma_intensity" BETWEEN 1 AND 3),
    "acidity" INTEGER CHECK ("acidity" BETWEEN 1 AND 3),
    "clean_umami" INTEGER CHECK ("clean_umami" BETWEEN 1 AND 3),
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL
);

INSERT INTO "sake_records_new" (
    "legacy_id", "owner_id", "drink_type", "name", "region", "brewery", "rice", "sake_type",
    "sake_meter_value", "abv", "volume", "price", "one_line_note", "notes", "rating", "place", "companions",
    "food_pairing", "consumed_date", "drink_again", "sweet_dry", "aroma_intensity", "acidity",
    "clean_umami", "created_at", "updated_at"
)
SELECT 
    r."id", u."new_id", COALESCE(r."drink_type", 'sake'), r."name", r."region", r."brewery", r."rice", r."sake_type",
    r."sake_meter_value", r."abv", r."volume", r."price", r."one_line_note", NULL, NULL, r."place", r."companions",
    r."food_pairing", r."consumed_date", r."drink_again", r."sweet_dry", r."aroma_intensity", r."acidity",
    r."clean_umami", r."created_at", COALESCE(r."updated_at", r."created_at")
FROM "sake_records" r
JOIN "_tmp_user_map" u ON r."owner_id" = u."old_id"
ORDER BY r."created_at" ASC;

INSERT INTO "_tmp_sake_map" ("old_id", "new_id")
SELECT "legacy_id", "id" FROM "sake_records_new";

CREATE UNIQUE INDEX IF NOT EXISTS "idx_sake_records_legacy_id_unique" ON "sake_records_new"("legacy_id");


-- ============================================================================
-- 4. STEP 4: sake_images_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "sake_images_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "legacy_id" TEXT UNIQUE,
    "owner_id" INTEGER NOT NULL,
    "record_id" INTEGER NOT NULL,
    "image_key" TEXT,
    "thumbnail_key" TEXT,
    "mime_type" TEXT NOT NULL,
    "file_name" TEXT NOT NULL,
    "display_order" INTEGER NOT NULL DEFAULT 0,
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL
);

INSERT INTO "sake_images_new" (
    "legacy_id", "owner_id", "record_id", "image_key", "thumbnail_key", "mime_type", "file_name", "display_order", "created_at", "updated_at"
)
SELECT 
    i."id", u."new_id", s."new_id", i."image_key", i."thumbnail_key", i."mime_type", i."file_name", COALESCE(i."display_order", 0), i."created_at", COALESCE(i."updated_at", i."created_at")
FROM "sake_images" i
JOIN "_tmp_user_map" u ON i."owner_id" = u."old_id"
JOIN "_tmp_sake_map" s ON i."record_id" = s."old_id"
ORDER BY i."created_at" ASC;

CREATE UNIQUE INDEX IF NOT EXISTS "idx_sake_images_legacy_id_unique" ON "sake_images_new"("legacy_id");


-- ============================================================================
-- 5. STEP 5: record_tags_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "record_tags_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "sake_record_id" INTEGER NOT NULL,
    "tag_id" INTEGER NOT NULL,
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL
);

INSERT INTO "record_tags_new" ("sake_record_id", "tag_id", "created_at", "updated_at")
SELECT 
    s."new_id", t."new_id", rt."created_at", COALESCE(rt."updated_at", rt."created_at")
FROM "record_tags" rt
JOIN "_tmp_sake_map" s ON rt."record_id" = s."old_id"
JOIN "_tmp_tag_map" t ON rt."tag_id" = t."old_id"
ORDER BY rt."created_at" ASC;

CREATE UNIQUE INDEX IF NOT EXISTS "idx_record_tags_sake_record_id_tag_id_unique" ON "record_tags_new"("sake_record_id", "tag_id");


-- ============================================================================
-- 6. STEP 6: Drop Legacy Tables in Reverse Dependency Order
-- ============================================================================
DROP TABLE IF EXISTS "record_tags";
DROP TABLE IF EXISTS "sake_images";
DROP TABLE IF EXISTS "sake_records";
DROP TABLE IF EXISTS "tags";
DROP TABLE IF EXISTS "users";
DROP TABLE IF EXISTS "oauth_sessions";


-- ============================================================================
-- 7. STEP 7: Rename New Tables to Final Names
-- ============================================================================
ALTER TABLE "users_new" RENAME TO "users";
ALTER TABLE "tags_new" RENAME TO "tags";
ALTER TABLE "sake_records_new" RENAME TO "sake_records";
ALTER TABLE "sake_images_new" RENAME TO "sake_images";
ALTER TABLE "record_tags_new" RENAME TO "record_tags";

CREATE TABLE IF NOT EXISTS "_mold_sessions" (
    "id" TEXT PRIMARY KEY,
    "user_id" INTEGER NOT NULL,
    "created_at" TEXT NOT NULL,
    "expires_at" TEXT NOT NULL,
    FOREIGN KEY ("user_id") REFERENCES "users"("id") ON DELETE CASCADE
);

DROP TABLE IF EXISTS "_tmp_user_map";
DROP TABLE IF EXISTS "_tmp_tag_map";
DROP TABLE IF EXISTS "_tmp_sake_map";

COMMIT;

PRAGMA foreign_keys = ON;
