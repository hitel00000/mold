// Custom Tag Creation API Handler with validation and duplicate protection
// Validation: trim, non-empty, max 20 chars, unique_together conflict handling

interface Env {
  DB: D1Database;
}

export async function onRequestPost(context: { request: Request; env: Env }) {
  const { request, env } = context;
  const cookieHeader = request.headers.get('Cookie') || '';

  // 1. Authenticate User
  const match = cookieHeader.match(/mold_session=([^;]+)/);
  if (!match) {
    return Response.json({ error: { code: 'UNAUTHORIZED', message: 'authentication required' } }, { status: 401 });
  }
  const token = match[1];
  const sess = await env.DB.prepare('SELECT user_id FROM "_mold_sessions" WHERE id = ? AND expires_at > ?')
    .bind(token, new Date().toISOString())
    .first<{ user_id: any }>();

  if (!sess) {
    return Response.json({ error: { code: 'UNAUTHORIZED', message: 'invalid session' } }, { status: 401 });
  }

  let body: any = {};
  try {
    body = await request.json();
  } catch (e) {
    return Response.json({ error: { code: 'INVALID_JSON', message: 'failed to parse json body' } }, { status: 400 });
  }

  let { drink_type = 'sake', tag_group, label } = body;
  if (!tag_group || !label) {
    return Response.json({ error: { code: 'INVALID_INPUT', message: 'tag_group and label required' } }, { status: 400 });
  }

  if (!['taste', 'aroma', 'mood'].includes(tag_group)) {
    return Response.json({ error: { code: 'INVALID_INPUT', message: 'invalid tag_group' } }, { status: 400 });
  }

  // Trim and Validate Label
  const trimmedLabel = String(label).trim();
  if (trimmedLabel.length === 0) {
    return Response.json({ error: { code: 'INVALID_INPUT', message: 'label cannot be empty' } }, { status: 400 });
  }
  if (trimmedLabel.length > 20) {
    return Response.json({ error: { code: 'INVALID_INPUT', message: 'label cannot exceed 20 characters' } }, { status: 400 });
  }

  // 2. Check Existing Tag for (owner_id, drink_type, tag_group, label)
  const existing = await env.DB.prepare(
    'SELECT * FROM "tags" WHERE "owner_id" = ? AND "drink_type" = ? AND "tag_group" = ? AND "label" = ?'
  )
    .bind(sess.user_id, drink_type, tag_group, trimmedLabel)
    .first<any>();

  if (existing) {
    return Response.json(
      {
        data: existing,
        already_exists: true,
      },
      { status: 200 }
    );
  }

  // 3. Insert New Custom Tag
  const now = new Date().toISOString();
  try {
    const created = await env.DB.prepare(
      'INSERT INTO "tags" ("owner_id", "drink_type", "tag_group", "label", "is_default", "created_at", "updated_at") VALUES (?, ?, ?, ?, 0, ?, ?) RETURNING *'
    )
      .bind(sess.user_id, drink_type, tag_group, trimmedLabel, now, now)
      .first<any>();

    return Response.json({ data: created, already_exists: false }, { status: 201 });
  } catch (err: any) {
    return Response.json({ error: { code: 'INSERT_FAILED', message: String(err?.message || err) } }, { status: 400 });
  }
}
