import { clearSessionCookies, revokeSession, type AppEnv } from "../../_shared/auth";

export const onRequestPost: PagesFunction<AppEnv> = async ({ env, request }) => {
  await revokeSession(request, env);
  const response = Response.json({ ok: true });
  for (const cookie of clearSessionCookies()) {
    response.headers.append("Set-Cookie", cookie);
  }
  return response;
};
