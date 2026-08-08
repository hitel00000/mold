import {
  clearOAuthStateCookie,
  createSessionCookie,
  getDatabase,
  getOAuthState,
  redirect,
  validateAuthEnv,
  type AppEnv,
} from "../../_shared/auth";

type GoogleTokenResponse = {
  access_token?: string;
  error?: string;
};

type GoogleUserInfo = {
  sub: string;
  email?: string;
  name?: string;
  picture?: string;
};

export const onRequestGet: PagesFunction<AppEnv> = async ({ env, request }) => {
  const envError = validateAuthEnv(env);
  if (envError) {
    return envError;
  }

  const url = new URL(request.url);
  const code = url.searchParams.get("code");
  const state = url.searchParams.get("state");
  const storedState = getOAuthState(request);

  if (!code || !state || !storedState || state !== storedState) {
    return new Response("Invalid OAuth state.", { status: 400 });
  }

  const redirectUri = env.GOOGLE_REDIRECT_URI ?? new URL("/api/auth/google-callback", request.url).toString();
  const tokenResponse = await fetch("https://oauth2.googleapis.com/token", {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
    },
    body: new URLSearchParams({
      client_id: env.GOOGLE_CLIENT_ID,
      client_secret: env.GOOGLE_CLIENT_SECRET,
      code,
      grant_type: "authorization_code",
      redirect_uri: redirectUri,
    }),
  });

  const tokenData = (await tokenResponse.json()) as GoogleTokenResponse;
  if (!tokenResponse.ok || !tokenData.access_token) {
    return new Response(tokenData.error ?? "Could not exchange OAuth code.", { status: 400 });
  }

  const userInfoResponse = await fetch("https://openidconnect.googleapis.com/v1/userinfo", {
    headers: {
      Authorization: `Bearer ${tokenData.access_token}`,
    },
  });

  if (!userInfoResponse.ok) {
    return new Response("Could not load Google user profile.", { status: 400 });
  }

  const profile = (await userInfoResponse.json()) as GoogleUserInfo;
  const now = new Date().toISOString();
  const userId = `google:${profile.sub}`;

  await getDatabase(env)
    .prepare(
      `INSERT INTO users (
        id,
        provider,
        provider_user_id,
        email,
        display_name,
        avatar_url,
        created_at,
        last_login_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(provider, provider_user_id) DO UPDATE SET
        email = excluded.email,
        display_name = excluded.display_name,
        avatar_url = excluded.avatar_url,
        last_login_at = excluded.last_login_at`,
    )
    .bind(
      userId,
      "google",
      profile.sub,
      profile.email ?? null,
      profile.name ?? null,
      profile.picture ?? null,
      now,
      now,
    )
    .run();

  const response = redirect("/#/logs");
  response.headers.append("Set-Cookie", clearOAuthStateCookie());
  response.headers.append("Set-Cookie", await createSessionCookie(env, userId));
  return response;
};
