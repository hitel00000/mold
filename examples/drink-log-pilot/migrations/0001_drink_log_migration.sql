-- 0001_drink_log_migration.sql
-- Production D1 Migration Script for Drink-Log Service
-- Option C: INTEGER AUTOINCREMENT PK for sake_records, sake_images, record_tags
-- User.id remains TEXT ("google:<sub>"), default tags remain TEXT ("tag_taste_fresh"), custom tags remain TEXT ("id")

PRAGMA foreign_keys = OFF;

BEGIN TRANSACTION;

-- ============================================================================
-- 1. STEP 1: Temp mapping table for sake_records
-- ============================================================================
CREATE TABLE IF NOT EXISTS "_tmp_sake_map" (
    "old_id" TEXT PRIMARY KEY,
    "new_id" INTEGER NOT NULL
);

-- ============================================================================
-- 2. STEP 2: sake_records_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "sake_records_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "legacy_id" TEXT UNIQUE,
    "owner_id" TEXT NOT NULL,
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
    "place" TEXT,
    "consumed_date" TEXT NOT NULL,
    "companions" TEXT,
    "food_pairing" TEXT,
    "drink_again" TEXT CHECK ("drink_again" IS NULL OR "drink_again" IN ('no', 'unsure', 'yes')),
    "sweet_dry" INTEGER CHECK ("sweet_dry" IS NULL OR "sweet_dry" BETWEEN 1 AND 5),
    "aroma_intensity" INTEGER CHECK ("aroma_intensity" IS NULL OR "aroma_intensity" BETWEEN 1 AND 3),
    "acidity" INTEGER CHECK ("acidity" IS NULL OR "acidity" BETWEEN 1 AND 3),
    "clean_umami" INTEGER CHECK ("clean_umami" IS NULL OR "clean_umami" BETWEEN 1 AND 3),
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL,
    FOREIGN KEY ("owner_id") REFERENCES "users"("id") ON DELETE CASCADE
);

INSERT INTO "sake_records_new" (
    "legacy_id", "owner_id", "drink_type", "name", "region", "brewery", "rice", "sake_type",
    "sake_meter_value", "abv", "volume", "price", "one_line_note", "place", "consumed_date",
    "companions", "food_pairing", "drink_again", "sweet_dry", "aroma_intensity", "acidity",
    "clean_umami", "created_at", "updated_at"
)
SELECT 
    "id", "owner_id", COALESCE("drink_type", 'sake'), "name", "region", "brewery", "rice", "sake_type",
    "sake_meter_value", "abv", "volume", "price", "one_line_note", "place", "consumed_date",
    "companions", "food_pairing", "drink_again", "sweet_dry", "aroma_intensity", "acidity",
    "clean_umami", "created_at", COALESCE("updated_at", "created_at")
FROM "sake_records"
ORDER BY "created_at" ASC;

INSERT INTO "_tmp_sake_map" ("old_id", "new_id")
SELECT "legacy_id", "id" FROM "sake_records_new";

CREATE UNIQUE INDEX IF NOT EXISTS "idx_sake_records_legacy_id_unique" ON "sake_records_new"("legacy_id");
CREATE INDEX IF NOT EXISTS "idx_sake_records_owner_id" ON "sake_records_new"("owner_id");
CREATE INDEX IF NOT EXISTS "idx_sake_records_consumed_date" ON "sake_records_new"("consumed_date" DESC);
CREATE INDEX IF NOT EXISTS "idx_sake_records_updated_at" ON "sake_records_new"("updated_at" DESC);


-- ============================================================================
-- 3. STEP 3: sake_images_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "sake_images_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "legacy_id" TEXT UNIQUE,
    "owner_id" TEXT NOT NULL,
    "record_id" INTEGER NOT NULL,
    "image_key" TEXT NOT NULL,
    "thumbnail_key" TEXT,
    "mime_type" TEXT NOT NULL,
    "file_name" TEXT NOT NULL,
    "display_order" INTEGER NOT NULL DEFAULT 0,
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL,
    FOREIGN KEY ("owner_id") REFERENCES "users"("id") ON DELETE CASCADE,
    FOREIGN KEY ("record_id") REFERENCES "sake_records_new"("id") ON DELETE CASCADE
);

INSERT INTO "sake_images_new" (
    "legacy_id", "owner_id", "record_id", "image_key", "thumbnail_key", "mime_type", "file_name", "display_order", "created_at", "updated_at"
)
SELECT 
    i."id", i."owner_id", s."new_id", i."image_key", i."thumbnail_key", i."mime_type", i."file_name", COALESCE(i."display_order", 0), i."created_at", COALESCE(i."created_at", CURRENT_TIMESTAMP)
FROM "sake_images" i
JOIN "_tmp_sake_map" s ON i."record_id" = s."old_id"
ORDER BY i."created_at" ASC;

CREATE UNIQUE INDEX IF NOT EXISTS "idx_sake_images_legacy_id_unique" ON "sake_images_new"("legacy_id");
CREATE INDEX IF NOT EXISTS "idx_sake_images_owner_id" ON "sake_images_new"("owner_id");
CREATE INDEX IF NOT EXISTS "idx_sake_images_record_id" ON "sake_images_new"("record_id");


-- ============================================================================
-- 4. STEP 4: record_tags_new Table Creation and Mapping
-- ============================================================================
CREATE TABLE IF NOT EXISTS "record_tags_new" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "sake_record_id" INTEGER NOT NULL,
    "tag_id" TEXT NOT NULL,
    "created_at" TEXT NOT NULL,
    "updated_at" TEXT NOT NULL,
    FOREIGN KEY ("sake_record_id") REFERENCES "sake_records_new"("id") ON DELETE CASCADE,
    FOREIGN KEY ("tag_id") REFERENCES "tags"("id") ON DELETE CASCADE
);

INSERT INTO "record_tags_new" ("sake_record_id", "tag_id", "created_at", "updated_at")
SELECT 
    s."new_id", rt."tag_id", rt."created_at", COALESCE(rt."created_at", CURRENT_TIMESTAMP)
FROM "record_tags" rt
JOIN "_tmp_sake_map" s ON rt."record_id" = s."old_id"
ORDER BY rt."created_at" ASC;

CREATE UNIQUE INDEX IF NOT EXISTS "idx_record_tags_sake_record_id_tag_id_unique" ON "record_tags_new"("sake_record_id", "tag_id");
CREATE INDEX IF NOT EXISTS "idx_record_tags_tag_id" ON "record_tags_new"("tag_id");


-- ============================================================================
-- 5. STEP 5: Drop Legacy Tables in Reverse Dependency Order
-- ============================================================================
DROP TABLE IF EXISTS "record_tags";
DROP TABLE IF EXISTS "sake_images";
DROP TABLE IF EXISTS "sake_records";


-- ============================================================================
-- 6. STEP 6: Rename New Tables to Final Names
-- ============================================================================
ALTER TABLE "sake_records_new" RENAME TO "sake_records";
ALTER TABLE "sake_images_new" RENAME TO "sake_images";
ALTER TABLE "record_tags_new" RENAME TO "record_tags";

DROP TABLE IF EXISTS "_tmp_sake_map";

COMMIT;

PRAGMA foreign_keys = ON;
