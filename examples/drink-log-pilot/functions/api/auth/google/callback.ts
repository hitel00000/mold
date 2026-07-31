// Google OAuth Callback Handler for Mold drink-log
// Issue Session for User (Integer user_id) via _mold_sessions

interface Env {
  DB: D1Database;
}

export async function onRequestPost(context: { request: Request; env: Env }) {
  const { request, env } = context;
  let body: any = {};
  try {
    body = await request.json();
  } catch (e) {
    return Response.json({ error: { code: 'INVALID_JSON', message: 'failed to parse json body' } }, { status: 400 });
  }

  const { provider, provider_user_id, email, display_name, avatar_url } = body;
  if (!provider || !provider_user_id) {
    return Response.json({ error: { code: 'INVALID_INPUT', message: 'provider and provider_user_id required' } }, { status: 400 });
  }

  const now = new Date().toISOString();

  // 1. Find or Create User
  let user = await env.DB.prepare('SELECT * FROM "users" WHERE "provider" = ? AND "provider_user_id" = ?')
    .bind(provider, provider_user_id)
    .first<any>();

  if (!user) {
    user = await env.DB.prepare(
      'INSERT INTO "users" ("provider", "provider_user_id", "email", "display_name", "avatar_url", "last_login_at", "role", "created_at", "updated_at") VALUES (?, ?, ?, ?, ?, ?, "user", ?, ?) RETURNING *'
    )
      .bind(provider, provider_user_id, email || null, display_name || null, avatar_url || null, now, now, now)
      .first<any>();
  } else {
    await env.DB.prepare('UPDATE "users" SET "last_login_at" = ?, "updated_at" = ? WHERE "id" = ?')
      .bind(now, now, user.id)
      .run();
  }

  if (!user || user.id == null) {
    return Response.json({ error: { code: 'INTERNAL_ERROR', message: 'failed to issue user account' } }, { status: 500 });
  }

  // 2. Issue Session Token (_mold_sessions)
  const sessionToken = 'sess_' + crypto.randomUUID();
  const expiresAt = new Date(Date.now() + 30 * 24 * 60 * 60 * 1000).toISOString(); // 30 days

  await env.DB.prepare('INSERT INTO "_mold_sessions" ("id", "user_id", "created_at", "expires_at") VALUES (?, ?, ?, ?)')
    .bind(sessionToken, user.id, now, expiresAt)
    .run();

  // 3. Set mold_session Cookie
  const cookieHeader = `mold_session=${sessionToken}; Path=/; HttpOnly; SameSite=Lax; Max-Age=2592000`;

  return Response.json(
    { data: { user, session_token: sessionToken } },
    {
      status: 200,
      headers: {
        'Set-Cookie': cookieHeader,
      },
    }
  );
}
