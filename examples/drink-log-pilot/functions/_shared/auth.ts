type CookieOptions = {
  expires?: Date;
  httpOnly?: boolean;
  maxAge?: number;
  path?: string;
  sameSite?: "Lax" | "Strict" | "None";
  secure?: boolean;
};

export type AppEnv = {
  GOOGLE_CLIENT_ID: string;
  GOOGLE_CLIENT_SECRET: string;
  GOOGLE_REDIRECT_URI?: string;
  SESSION_SECRET: string;
  DB?: D1Database;
  alcohol_log?: D1Database;
  IMAGES?: R2Bucket;
  alcohol_log_images?: R2Bucket;
};

export type SessionPayload = {
  userId: string;
  exp: number;
};

type SessionRow = {
  user_id: string;
  expires_at: string;
};

const REQUIRED_ENV_KEYS = [
  "GOOGLE_CLIENT_ID",
  "GOOGLE_CLIENT_SECRET",
  "SESSION_SECRET",
] as const;

const SESSION_COOKIE = "alcohol_log_session";
const OAUTH_STATE_COOKIE = "alcohol_log_oauth_state";
const SESSION_TTL_SECONDS = 60 * 60 * 24 * 30;

function base64UrlEncode(value: string) {
  return btoa(value).replace(/\+/g, "-").replace(/\//g, "_").replace(/=/g, "");
}

function base64UrlDecode(value: string) {
  const padded = value.replace(/-/g, "+").replace(/_/g, "/").padEnd(Math.ceil(value.length / 4) * 4, "=");
  return atob(padded);
}

function getCookie(request: Request, name: string) {
  const cookie = request.headers.get("Cookie");
  if (!cookie) {
    return null;
  }

  const match = cookie
    .split(";")
    .map((part) => part.trim())
    .find((part) => part.startsWith(`${name}=`));

  return match ? decodeURIComponent(match.slice(name.length + 1)) : null;
}

function serializeCookie(name: string, value: string, options: CookieOptions = {}) {
  const parts = [`${name}=${encodeURIComponent(value)}`];
  parts.push(`Path=${options.path ?? "/"}`);
  parts.push(`SameSite=${options.sameSite ?? "Lax"}`);

  if (options.maxAge !== undefined) {
    parts.push(`Max-Age=${options.maxAge}`);
  }
  if (options.expires) {
    parts.push(`Expires=${options.expires.toUTCString()}`);
  }
  if (options.httpOnly ?? true) {
    parts.push("HttpOnly");
  }
  if (options.secure ?? true) {
    parts.push("Secure");
  }

  return parts.join("; ");
}

export function getDatabase(env: AppEnv) {
  const database = env.DB ?? env.alcohol_log;
  if (!database) {
    throw new Error("D1 binding is missing. Expected DB or alcohol_log.");
  }
  return database;
}

export function getImagesBucket(env: AppEnv) {
  const bucket = env.IMAGES ?? env.alcohol_log_images;
  if (!bucket) {
    throw new Error("R2 binding is missing. Expected IMAGES or alcohol_log_images.");
  }
  return bucket;
}

export function validateAuthEnv(env: AppEnv) {
  const missing = REQUIRED_ENV_KEYS.filter((key) => !env[key]);
  if (missing.length === 0) {
    return null;
  }

  return new Response(
    JSON.stringify({
      error: "missing_auth_environment",
      missing,
      message:
        "Required OAuth secrets are missing from the Cloudflare Pages Functions runtime environment.",
    }),
    {
      status: 500,
      headers: {
        "Content-Type": "application/json; charset=utf-8",
      },
    },
  );
}

export function getOAuthState(request: Request) {
  return getCookie(request, OAUTH_STATE_COOKIE);
}

export function getSessionCookie(request: Request) {
  return getCookie(request, SESSION_COOKIE);
}

async function ensureSessionTable(env: AppEnv) {
  const db = getDatabase(env);
  await db
    .prepare(
      `CREATE TABLE IF NOT EXISTS oauth_sessions (
        id TEXT PRIMARY KEY,
        user_id TEXT NOT NULL,
        created_at TEXT NOT NULL,
        expires_at TEXT NOT NULL,
        FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
      )`,
    )
    .run();
  await db
    .prepare("CREATE INDEX IF NOT EXISTS idx_oauth_sessions_user_id ON oauth_sessions(user_id)")
    .run();
  await db
    .prepare("CREATE INDEX IF NOT EXISTS idx_oauth_sessions_expires_at ON oauth_sessions(expires_at)")
    .run();
}

export function createOAuthStateCookie(state: string) {
  return serializeCookie(OAUTH_STATE_COOKIE, state, {
    maxAge: 60 * 10,
    path: "/api/auth",
  });
}

export function clearOAuthStateCookie() {
  return serializeCookie(OAUTH_STATE_COOKIE, "", {
    maxAge: 0,
    path: "/api/auth",
  });
}

export async function createSessionCookie(env: AppEnv, userId: string) {
  await ensureSessionTable(env);

  const sessionId = crypto.randomUUID();
  const now = new Date();
  const expiresAt = new Date(now.getTime() + SESSION_TTL_SECONDS * 1000);

  await getDatabase(env)
    .prepare(
      "INSERT INTO oauth_sessions (id, user_id, created_at, expires_at) VALUES (?, ?, ?, ?)",
    )
    .bind(sessionId, userId, now.toISOString(), expiresAt.toISOString())
    .run();

  return serializeCookie(SESSION_COOKIE, sessionId, {
    expires: expiresAt,
    maxAge: SESSION_TTL_SECONDS,
  });
}

export function clearSessionCookie() {
  return serializeCookie(SESSION_COOKIE, "", {
    expires: new Date(0),
    maxAge: 0,
  });
}

export function clearSessionCookies() {
  const paths = ["/", "/api", "/api/auth"];
  return paths.flatMap((path) => [
    serializeCookie(SESSION_COOKIE, "", {
      expires: new Date(0),
      maxAge: 0,
      path,
    }),
    serializeCookie(SESSION_COOKIE, "", {
      expires: new Date(0),
      maxAge: 0,
      path,
      sameSite: "Strict",
    }),
  ]);
}

export async function readSession(request: Request, env: AppEnv) {
  const sessionId = getSessionCookie(request);
  if (!sessionId || sessionId.includes(".")) {
    return null;
  }

  await ensureSessionTable(env);
  const session = await getDatabase(env)
    .prepare("SELECT user_id, expires_at FROM oauth_sessions WHERE id = ?")
    .bind(sessionId)
    .first<SessionRow>();

  if (!session) {
    return null;
  }

  if (new Date(session.expires_at).getTime() <= Date.now()) {
    await getDatabase(env).prepare("DELETE FROM oauth_sessions WHERE id = ?").bind(sessionId).run();
    return null;
  }

  return {
    userId: session.user_id,
    exp: Math.floor(new Date(session.expires_at).getTime() / 1000),
  };
}

export async function revokeSession(request: Request, env: AppEnv) {
  const sessionId = getSessionCookie(request);
  if (!sessionId || sessionId.includes(".")) {
    return;
  }

  await ensureSessionTable(env);
  await getDatabase(env).prepare("DELETE FROM oauth_sessions WHERE id = ?").bind(sessionId).run();
}

export function redirect(location: string, init?: ResponseInit) {
  return new Response(null, {
    ...init,
    status: init?.status ?? 302,
    headers: {
      ...init?.headers,
      Location: location,
    },
  });
}
