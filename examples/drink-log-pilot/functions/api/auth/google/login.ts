import { createOAuthStateCookie, redirect, validateAuthEnv, type AppEnv } from "../../../_shared/auth";

export const onRequestGet: PagesFunction<AppEnv> = async ({ env, request }) => {
  const envError = validateAuthEnv(env);
  if (envError) {
    return envError;
  }

  const state = crypto.randomUUID();
  const redirectUri = env.GOOGLE_REDIRECT_URI ?? new URL("/api/auth/google-callback", request.url).toString();

  const authUrl = new URL("https://accounts.google.com/o/oauth2/v2/auth");
  authUrl.searchParams.set("client_id", env.GOOGLE_CLIENT_ID);
  authUrl.searchParams.set("redirect_uri", redirectUri);
  authUrl.searchParams.set("response_type", "code");
  authUrl.searchParams.set("scope", "openid email profile");
  authUrl.searchParams.set("state", state);
  authUrl.searchParams.set("prompt", "select_account");

  const response = redirect(authUrl.toString());
  response.headers.append("Set-Cookie", createOAuthStateCookie(state));
  return response;
};
