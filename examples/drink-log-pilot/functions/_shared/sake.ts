import { getDatabase, getImagesBucket, readSession, type AppEnv } from "./auth";

type DrinkAgainValue = "no" | "unsure" | "yes";
type SakeTagGroup = "taste" | "aroma" | "mood";

type SakeRecord = {
  id: string | number;
  owner_id: string;
  drink_type: "sake";
  name: string;
  region: string | null;
  brewery: string | null;
  rice: string | null;
  sake_type: string | null;
  sake_meter_value: string | null;
  abv: string | null;
  volume: string | null;
  price: string | null;
  drink_again: DrinkAgainValue | null;
  sweet_dry: number | null;
  aroma_intensity: number | null;
  acidity: number | null;
  clean_umami: number | null;
  one_line_note: string | null;
  place: string | null;
  consumed_date: string;
  companions: string | null;
  food_pairing: string | null;
  created_at: string;
  updated_at: string;
};

type SakeImage = {
  id: string | number;
  owner_id: string;
  record_id: string | number;
  image_key: string;
  thumbnail_key: string | null;
  data_url?: string;
  thumbnail_data_url?: string | null;
  mime_type: string;
  file_name: string;
  display_order: number;
  created_at: string;
};

type SakeTag = {
  id: string;
  owner_id: string | null;
  drink_type: "sake";
  tag_group: SakeTagGroup;
  label: string;
  is_default: boolean | number;
  created_at: string;
};

type SakeRecordTag = {
  sake_record_id?: string | number;
  record_id?: string | number;
  tag_id: string;
  created_at: string;
};

type SakeRecordEntry = {
  id: string;
  record: SakeRecord;
  images: SakeImage[];
  tags: SakeTag[];
  record_tags: SakeRecordTag[];
};

type ImageRow = Omit<SakeImage, "data_url" | "thumbnail_data_url">;

const MAX_CUSTOM_TAG_LABEL_LENGTH = 20;
const DEFAULT_DRINK_TYPE = "sake";

function json(data: unknown, init?: ResponseInit) {
  const response = Response.json(data, init);
  response.headers.set("Cache-Control", "no-store");
  response.headers.set("CDN-Cache-Control", "no-store");
  response.headers.set("Cloudflare-CDN-Cache-Control", "no-store");
  response.headers.set("Pragma", "no-cache");
  response.headers.set("Expires", "0");
  return response;
}

function imageUrl(key: string) {
  return `/api/images?key=${encodeURIComponent(key)}`;
}

function normalizeOptionalText(value: unknown) {
  if (typeof value !== "string") {
    return null;
  }
  const trimmed = value.trim();
  return trimmed ? trimmed : null;
}

function normalizeRequiredText(value: unknown) {
  return typeof value === "string" ? value.trim() : "";
}

function normalizeDrinkAgain(value: unknown): DrinkAgainValue | null {
  return value === "no" || value === "unsure" || value === "yes" ? value : null;
}

function normalizeRating(value: unknown, min: number, max: number) {
  return typeof value === "number" && Number.isInteger(value) && value >= min && value <= max
    ? value
    : null;
}

function normalizeTagGroup(value: unknown): SakeTagGroup | null {
  return value === "taste" || value === "aroma" || value === "mood" ? value : null;
}

function parseDataUrl(value: string) {
  const match = /^data:([^;,]+)?(;base64)?,(.*)$/s.exec(value);
  if (!match) {
    return null;
  }

  const mimeType = match[1] || "application/octet-stream";
  const payload = match[3];
  if (match[2]) {
    const binary = atob(payload);
    const bytes = new Uint8Array(binary.length);
    for (let index = 0; index < binary.length; index += 1) {
      bytes[index] = binary.charCodeAt(index);
    }
    return { bytes, mimeType };
  }

  return { bytes: new TextEncoder().encode(decodeURIComponent(payload)), mimeType };
}

async function requireSession(
  request: Request,
  env: AppEnv,
): Promise<{ userId: string } | { response: Response }> {
  const session = await readSession(request, env);
  if (!session) {
    return { response: json({ error: "unauthorized" }, { status: 401 }) };
  }
  return { userId: session.userId };
}

function getImageExtension(image: Pick<SakeImage, "file_name" | "mime_type">) {
  const fileExtension = image.file_name.split(".").pop();
  if (fileExtension) {
    return fileExtension;
  }
  return image.mime_type.split("/").pop() || "jpg";
}

function normalizeRecord(input: unknown, ownerId: string, existing?: SakeRecord): SakeRecord | { error: Response } {
  const source = (input && typeof input === "object" && "record" in input
    ? (input as { record?: unknown }).record
    : input) as Partial<SakeRecord> | null;

  const name = normalizeRequiredText(source?.name);
  if (!name) {
    return { error: json({ error: "name_required" }, { status: 400 }) };
  }

  const now = new Date().toISOString();
  const id = source?.id || existing?.id || crypto.randomUUID();
  const consumedDate = normalizeRequiredText(source?.consumed_date) || now.slice(0, 10);

  return {
    id,
    owner_id: ownerId,
    drink_type: DEFAULT_DRINK_TYPE,
    name,
    region: normalizeOptionalText(source?.region),
    brewery: normalizeOptionalText(source?.brewery),
    rice: normalizeOptionalText(source?.rice),
    sake_type: normalizeOptionalText(source?.sake_type),
    sake_meter_value: normalizeOptionalText(source?.sake_meter_value),
    abv: normalizeOptionalText(source?.abv),
    volume: normalizeOptionalText(source?.volume),
    price: normalizeOptionalText(source?.price),
    drink_again: normalizeDrinkAgain(source?.drink_again),
    sweet_dry: normalizeRating(source?.sweet_dry, 1, 5),
    aroma_intensity: normalizeRating(source?.aroma_intensity, 1, 3),
    acidity: normalizeRating(source?.acidity, 1, 3),
    clean_umami: normalizeRating(source?.clean_umami, 1, 3),
    one_line_note: normalizeOptionalText(source?.one_line_note),
    place: normalizeOptionalText(source?.place),
    consumed_date: consumedDate,
    companions: normalizeOptionalText(source?.companions),
    food_pairing: normalizeOptionalText(source?.food_pairing),
    created_at: existing?.created_at ?? (normalizeRequiredText(source?.created_at) || now),
    updated_at: now,
  };
}

function normalizeImage(
  input: Partial<SakeImage>,
  ownerId: string,
  recordId: string | number,
  displayOrder: number,
  existing?: SakeImage,
): SakeImage {
  const id = input.id || crypto.randomUUID();
  const mimeType = normalizeRequiredText(input.mime_type) || existing?.mime_type || "image/jpeg";
  const fileName = normalizeRequiredText(input.file_name) || existing?.file_name || `${id}.jpg`;
  const base = { id, file_name: fileName, mime_type: mimeType };
  const imageKey =
    existing?.image_key ??
    `images/${ownerId}/sake/${recordId}/${id}.${getImageExtension(base)}`;

  return {
    id,
    owner_id: ownerId,
    record_id: recordId,
    image_key: imageKey,
    thumbnail_key: existing?.thumbnail_key ?? `thumbnails/${ownerId}/sake/${recordId}/${id}.webp`,
    data_url: input.data_url,
    thumbnail_data_url: input.thumbnail_data_url ?? existing?.thumbnail_data_url ?? null,
    mime_type: mimeType,
    file_name: fileName,
    display_order: displayOrder,
    created_at: existing?.created_at ?? (normalizeRequiredText(input.created_at) || new Date().toISOString()),
  };
}

async function uploadImagePayload(env: AppEnv, image: SakeImage) {
  const bucket = getImagesBucket(env);
  if (image.data_url && !image.data_url.startsWith("/api/images?key=")) {
    const original = parseDataUrl(image.data_url);
    if (original) {
      await bucket.put(image.image_key, original.bytes, {
        httpMetadata: { contentType: image.mime_type || original.mimeType },
      });
    }
  }

  if (image.thumbnail_key && image.thumbnail_data_url && !image.thumbnail_data_url.startsWith("/api/images?key=")) {
    const thumbnail = parseDataUrl(image.thumbnail_data_url);
    if (thumbnail) {
      await bucket.put(image.thumbnail_key, thumbnail.bytes, {
        httpMetadata: { contentType: thumbnail.mimeType },
      });
    }
  }
}

export function buildEntry(record: SakeRecord, images: ImageRow[], recordTags: SakeRecordTag[], tags: SakeTag[]) {
  const tagsById = new Map(tags.map((tag) => [tag.id, tag]));
  return {
    id: String(record.id),
    record,
    images: images
      .sort((left, right) => left.display_order - right.display_order)
      .map((image) => ({
        ...image,
        data_url: imageUrl(image.image_key),
        thumbnail_data_url: image.thumbnail_key ? imageUrl(image.thumbnail_key) : null,
      })),
    record_tags: recordTags,
    tags: recordTags
      .map((recordTag) => tagsById.get(recordTag.tag_id))
      .filter((tag): tag is SakeTag => Boolean(tag))
      .map((tag) => ({ ...tag, is_default: Boolean(tag.is_default) })),
  } satisfies SakeRecordEntry;
}

export async function loadTagsForOwner(env: AppEnv, ownerId: string) {
  const tags = await getDatabase(env)
    .prepare(
      `SELECT id, owner_id, drink_type, tag_group, label, is_default, created_at
       FROM tags
       WHERE drink_type = 'sake' AND (owner_id IS NULL OR owner_id = ?)
       ORDER BY tag_group, is_default DESC, created_at, label`,
    )
    .bind(ownerId)
    .all<SakeTag>();
  return tags.results;
}

export async function loadEntries(env: AppEnv, ownerId: string, recordId?: string | number, query?: string) {
  const db = getDatabase(env);
  const params: unknown[] = [ownerId];
  const clauses = ["record.owner_id = ?", "record.drink_type = 'sake'"];
  if (recordId !== undefined && recordId !== null) {
    clauses.push("record.id = ?");
    params.push(recordId);
  }
  if (query) {
    const search = `%${query.toLowerCase()}%`;
    clauses.push(
      `(lower(record.name) LIKE ? OR lower(coalesce(record.region, '')) LIKE ? OR lower(coalesce(record.brewery, '')) LIKE ? OR lower(coalesce(record.sake_type, '')) LIKE ? OR lower(coalesce(record.rice, '')) LIKE ? OR lower(coalesce(record.place, '')) LIKE ? OR lower(coalesce(record.one_line_note, '')) LIKE ? OR EXISTS (
        SELECT 1 FROM record_tags rt
        JOIN tags tag ON tag.id = rt.tag_id
        WHERE rt.sake_record_id = record.id AND lower(tag.label) LIKE ?
      ))`,
    );
    params.push(search, search, search, search, search, search, search, search);
  }

  const records = await db
    .prepare(
      `SELECT record.*
       FROM sake_records record
       WHERE ${clauses.join(" AND ")}
       ORDER BY record.consumed_date DESC, record.created_at DESC`,
    )
    .bind(...params)
    .all<SakeRecord>();

  const recordIds = records.results.map((record) => record.id);
  if (recordIds.length === 0) {
    return [];
  }

  const images = await db
    .prepare(
      `SELECT id, owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, display_order, created_at
       FROM sake_images
       WHERE owner_id = ? ${recordId !== undefined && recordId !== null ? "AND record_id = ?" : ""}
       ORDER BY display_order, created_at`,
    )
    .bind(...(recordId !== undefined && recordId !== null ? [ownerId, recordId] : [ownerId]))
    .all<ImageRow>();

  const recordTags = await db
    .prepare(
      `SELECT rt.sake_record_id, rt.tag_id, rt.created_at
       FROM record_tags rt
       JOIN sake_records record ON record.id = rt.sake_record_id
       WHERE record.owner_id = ? ${recordId !== undefined && recordId !== null ? "AND record.id = ?" : ""}`,
    )
    .bind(...(recordId !== undefined && recordId !== null ? [ownerId, recordId] : [ownerId]))
    .all<SakeRecordTag>();

  const tags = await loadTagsForOwner(env, ownerId);
  const imagesByRecordId = new Map<string | number, ImageRow[]>();
  images.results.forEach((image) => {
    const group = imagesByRecordId.get(image.record_id) ?? [];
    group.push(image);
    imagesByRecordId.set(image.record_id, group);
  });

  const tagsByRecordId = new Map<string | number, SakeRecordTag[]>();
  recordTags.results.forEach((recordTag) => {
    const recId = recordTag.sake_record_id ?? recordTag.record_id;
    if (recId !== undefined && recId !== null) {
      const group = tagsByRecordId.get(recId) ?? [];
      group.push(recordTag);
      tagsByRecordId.set(recId, group);
    }
  });

  return records.results.map((record) =>
    buildEntry(
      record,
      imagesByRecordId.get(record.id) ?? [],
      tagsByRecordId.get(record.id) ?? [],
      tags,
    ),
  );
}

async function getRecordOwner(env: AppEnv, recordId: string | number) {
  return getDatabase(env)
    .prepare("SELECT owner_id FROM sake_records WHERE id = ? AND drink_type = 'sake'")
    .bind(recordId)
    .first<{ owner_id: string }>();
}

async function authorizeRecordAccess(env: AppEnv, ownerId: string, recordId: string | number) {
  const record = await getRecordOwner(env, recordId);
  if (!record) {
    return json({ error: "not_found" }, { status: 404 });
  }
  if (record.owner_id !== ownerId) {
    return json({ error: "forbidden" }, { status: 403 });
  }
  return null;
}

async function assertTagsAreUsable(env: AppEnv, ownerId: string, tagIds: string[]) {
  if (tagIds.length === 0) {
    return [];
  }

  const tags = await loadTagsForOwner(env, ownerId);
  const usableIds = new Set(tags.map((tag) => tag.id));
  return Array.from(new Set(tagIds.filter((tagId) => usableIds.has(tagId))));
}

async function saveRecordTags(env: AppEnv, ownerId: string, recordId: string | number, tagIds: string[]) {
  const db = getDatabase(env);
  const usableTagIds = await assertTagsAreUsable(env, ownerId, tagIds);
  const now = new Date().toISOString();
  await db.prepare("DELETE FROM record_tags WHERE sake_record_id = ?").bind(recordId).run();

  for (const tagId of usableTagIds) {
    await db
      .prepare("INSERT OR IGNORE INTO record_tags (sake_record_id, tag_id, created_at, updated_at) VALUES (?, ?, ?, ?)")
      .bind(recordId, tagId, now, now)
      .run();
  }
}

async function saveImages(env: AppEnv, ownerId: string, recordId: string | number, images: SakeImage[]) {
  const db = getDatabase(env);

  for (const image of images) {
    await uploadImagePayload(env, image);
  }

  await db.prepare("DELETE FROM sake_images WHERE owner_id = ? AND record_id = ?").bind(ownerId, recordId).run();

  for (const image of images) {
    await db
      .prepare(
        `INSERT INTO sake_images (
          owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, display_order, created_at, updated_at
        ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
      )
      .bind(
        ownerId,
        recordId,
        image.image_key,
        image.thumbnail_key,
        image.mime_type,
        image.file_name,
        image.display_order,
        image.created_at,
        image.created_at,
      )
      .run();
  }
}

function getPayloadImages(payload: unknown): Partial<SakeImage>[] {
  if (payload && typeof payload === "object" && Array.isArray((payload as { images?: unknown }).images)) {
    return (payload as { images: Partial<SakeImage>[] }).images;
  }
  return [];
}

function getPayloadTagIds(payload: unknown) {
  const rawRecordTags =
    payload && typeof payload === "object" && Array.isArray((payload as { record_tags?: unknown }).record_tags)
      ? (payload as { record_tags: Partial<SakeRecordTag>[] }).record_tags
      : [];
  const rawSelectedIds =
    payload && typeof payload === "object" && Array.isArray((payload as { selected_tag_ids?: unknown }).selected_tag_ids)
      ? (payload as { selected_tag_ids: unknown[] }).selected_tag_ids
      : [];

  return Array.from(
    new Set(
      [
        ...rawRecordTags.map((recordTag) => recordTag.tag_id),
        ...rawSelectedIds,
      ].filter((tagId): tagId is string => typeof tagId === "string" && Boolean(tagId.trim())),
    ),
  );
}

export async function listSakeRecords(request: Request, env: AppEnv): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  return json(await loadEntries(env, session.userId));
}

export async function searchSakeRecords(request: Request, env: AppEnv): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const query = new URL(request.url).searchParams.get("q")?.trim();
  return json(await loadEntries(env, session.userId, undefined, query || undefined));
}

export async function getSakeRecord(request: Request, env: AppEnv, id: string | number): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const accessError = await authorizeRecordAccess(env, session.userId, id);
  if (accessError) {
    return accessError;
  }

  const entry = (await loadEntries(env, session.userId, id))[0];
  return entry ? json(entry) : json({ error: "not_found" }, { status: 404 });
}

export async function createSakeRecord(request: Request, env: AppEnv): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const payload = await request.json();
  const record = normalizeRecord(payload, session.userId);
  if ("error" in record) {
    return record.error;
  }

  const res = await getDatabase(env)
    .prepare(
      `INSERT INTO sake_records (
        owner_id, drink_type, name, region, brewery, rice, sake_type, sake_meter_value,
        abv, volume, price, drink_again, sweet_dry, aroma_intensity, acidity, clean_umami,
        one_line_note, place, consumed_date, companions, food_pairing, created_at, updated_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
    )
    .bind(
      record.owner_id,
      record.drink_type,
      record.name,
      record.region,
      record.brewery,
      record.rice,
      record.sake_type,
      record.sake_meter_value,
      record.abv,
      record.volume,
      record.price,
      record.drink_again,
      record.sweet_dry,
      record.aroma_intensity,
      record.acidity,
      record.clean_umami,
      record.one_line_note,
      record.place,
      record.consumed_date,
      record.companions,
      record.food_pairing,
      record.created_at,
      record.updated_at,
    )
    .first<{ id: number }>();

  const newRecordId = res?.id || record.id;

  const images = getPayloadImages(payload).map((image, index) =>
    normalizeImage(image, session.userId, newRecordId, index),
  );
  const tagIds = getPayloadTagIds(payload);

  await saveImages(env, session.userId, newRecordId, images);
  await saveRecordTags(env, session.userId, newRecordId, tagIds);
  return getSakeRecord(request, env, newRecordId);
}

export async function updateSakeRecord(request: Request, env: AppEnv, id: string | number): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const accessError = await authorizeRecordAccess(env, session.userId, id);
  if (accessError) {
    return accessError;
  }

  const existing = await getDatabase(env)
    .prepare("SELECT * FROM sake_records WHERE id = ?")
    .bind(id)
    .first<SakeRecord>();
  if (!existing) {
    return json({ error: "not_found" }, { status: 404 });
  }

  const payload = await request.json();
  const record = normalizeRecord(payload, session.userId, existing);
  if ("error" in record) {
    return record.error;
  }

  const existingImages = await getDatabase(env)
    .prepare(
      `SELECT id, owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, display_order, created_at
       FROM sake_images WHERE owner_id = ? AND record_id = ?`,
    )
    .bind(session.userId, id)
    .all<ImageRow>();
  const existingById = new Map(existingImages.results.map((image) => [String(image.id), image as SakeImage]));
  const images = getPayloadImages(payload).map((image, index) =>
    normalizeImage(image, session.userId, id, index, existingById.get(String(image.id))),
  );
  const nextImageIds = new Set(images.map((image) => String(image.id)));
  const deletedImages = existingImages.results.filter((image) => !nextImageIds.has(String(image.id)));

  await getDatabase(env)
    .prepare(
      `UPDATE sake_records SET
        name = ?, region = ?, brewery = ?, rice = ?, sake_type = ?, sake_meter_value = ?,
        abv = ?, volume = ?, price = ?, drink_again = ?, sweet_dry = ?, aroma_intensity = ?,
        acidity = ?, clean_umami = ?, one_line_note = ?, place = ?, consumed_date = ?,
        companions = ?, food_pairing = ?, updated_at = ?
       WHERE owner_id = ? AND id = ?`,
    )
    .bind(
      record.name,
      record.region,
      record.brewery,
      record.rice,
      record.sake_type,
      record.sake_meter_value,
      record.abv,
      record.volume,
      record.price,
      record.drink_again,
      record.sweet_dry,
      record.aroma_intensity,
      record.acidity,
      record.clean_umami,
      record.one_line_note,
      record.place,
      record.consumed_date,
      record.companions,
      record.food_pairing,
      record.updated_at,
      session.userId,
      id,
    )
    .run();

  await saveImages(env, session.userId, id, images);
  await saveRecordTags(env, session.userId, id, getPayloadTagIds(payload));
  await Promise.all(
    deletedImages.flatMap((image) =>
      [image.image_key, image.thumbnail_key].filter(Boolean).map((key) => getImagesBucket(env).delete(String(key))),
    ),
  );

  return getSakeRecord(request, env, id);
}

export async function deleteSakeRecord(request: Request, env: AppEnv, id: string | number): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const db = getDatabase(env);
  const accessError = await authorizeRecordAccess(env, session.userId, id);
  if (accessError) {
    return accessError;
  }

  const images = await db
    .prepare("SELECT image_key, thumbnail_key FROM sake_images WHERE owner_id = ? AND record_id = ?")
    .bind(session.userId, id)
    .all<{ image_key: string; thumbnail_key: string | null }>();
  await db.prepare("DELETE FROM record_tags WHERE sake_record_id = ?").bind(id).run();
  await db.prepare("DELETE FROM sake_images WHERE owner_id = ? AND record_id = ?").bind(session.userId, id).run();
  await db.prepare("DELETE FROM sake_records WHERE owner_id = ? AND id = ?").bind(session.userId, id).run();
  await Promise.all(
    images.results.flatMap((image) =>
      [image.image_key, image.thumbnail_key].filter(Boolean).map((key) => getImagesBucket(env).delete(String(key))),
    ),
  );

  return new Response(null, { status: 204 });
}

export async function addSakeRecordImage(request: Request, env: AppEnv, recordId: string | number): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const accessError = await authorizeRecordAccess(env, session.userId, recordId);
  if (accessError) {
    return accessError;
  }

  const payload = (await request.json()) as Partial<SakeImage>;
  const maxOrder = await getDatabase(env)
    .prepare("SELECT MAX(display_order) AS max_order FROM sake_images WHERE owner_id = ? AND record_id = ?")
    .bind(session.userId, recordId)
    .first<{ max_order: number | null }>();
  const image = normalizeImage(payload, session.userId, recordId, (maxOrder?.max_order ?? -1) + 1);
  await uploadImagePayload(env, image);
  const res = await getDatabase(env)
    .prepare(
      `INSERT INTO sake_images (
        owner_id, record_id, image_key, thumbnail_key, mime_type, file_name, display_order, created_at, updated_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) RETURNING id`,
    )
    .bind(
      session.userId,
      recordId,
      image.image_key,
      image.thumbnail_key,
      image.mime_type,
      image.file_name,
      image.display_order,
      image.created_at,
      image.created_at,
    )
    .first<{ id: number }>();

  const newImgId = res?.id || image.id;
  return json({ ...image, id: newImgId, data_url: imageUrl(image.image_key), thumbnail_data_url: image.thumbnail_key ? imageUrl(image.thumbnail_key) : null }, { status: 201 });
}

export async function deleteSakeRecordImage(
  request: Request,
  env: AppEnv,
  recordId: string | number,
  imageId: string | number,
): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const accessError = await authorizeRecordAccess(env, session.userId, recordId);
  if (accessError) {
    return accessError;
  }

  const image = await getDatabase(env)
    .prepare(
      "SELECT image_key, thumbnail_key FROM sake_images WHERE owner_id = ? AND record_id = ? AND id = ?",
    )
    .bind(session.userId, recordId, imageId)
    .first<{ image_key: string; thumbnail_key: string | null }>();
  if (!image) {
    return json({ error: "not_found" }, { status: 404 });
  }

  await getDatabase(env)
    .prepare("DELETE FROM sake_images WHERE owner_id = ? AND record_id = ? AND id = ?")
    .bind(session.userId, recordId, imageId)
    .run();
  await Promise.all(
    [image.image_key, image.thumbnail_key].filter(Boolean).map((key) => getImagesBucket(env).delete(String(key))),
  );

  return new Response(null, { status: 204 });
}

export async function listSakeTags(request: Request, env: AppEnv): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const drinkType = new URL(request.url).searchParams.get("drink_type") || DEFAULT_DRINK_TYPE;
  if (drinkType !== DEFAULT_DRINK_TYPE) {
    return json([]);
  }

  const tags = await loadTagsForOwner(env, session.userId);
  return json(tags.map((tag) => ({ ...tag, is_default: Boolean(tag.is_default) })));
}

export async function createSakeTag(request: Request, env: AppEnv): Promise<Response> {
  const session = await requireSession(request, env);
  if ("response" in session) {
    return session.response;
  }

  const payload = (await request.json()) as { tag_group?: unknown; label?: unknown; drink_type?: unknown };
  const tagGroup = normalizeTagGroup(payload.tag_group);
  const label = normalizeRequiredText(payload.label).slice(0, MAX_CUSTOM_TAG_LABEL_LENGTH);
  if (!tagGroup || !label || (payload.drink_type && payload.drink_type !== DEFAULT_DRINK_TYPE)) {
    return json({ error: "invalid_tag" }, { status: 400 });
  }

  const existing = (await loadTagsForOwner(env, session.userId)).find(
    (tag) => tag.tag_group === tagGroup && tag.label.trim().toLowerCase() === label.toLowerCase(),
  );
  if (existing) {
    const response = json({
      ...existing,
      is_default: Boolean(existing.is_default),
      already_exists: true,
    });
    response.headers.set("X-Sake-Tag-Existing", "true");
    return response;
  }

  const tag: SakeTag = {
    id: crypto.randomUUID(),
    owner_id: session.userId,
    drink_type: DEFAULT_DRINK_TYPE,
    tag_group: tagGroup,
    label,
    is_default: false,
    created_at: new Date().toISOString(),
  };

  await getDatabase(env)
    .prepare(
      "INSERT INTO tags (id, owner_id, drink_type, tag_group, label, is_default, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)",
    )
    .bind(tag.id, tag.owner_id, tag.drink_type, tag.tag_group, tag.label, 0, tag.created_at)
    .run();

  return json({ ...tag, already_exists: false }, { status: 201 });
}
