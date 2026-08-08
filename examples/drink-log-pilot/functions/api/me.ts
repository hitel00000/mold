import { getDatabase, readSession, type AppEnv } from "../_shared/auth";

type UserRow = {
  id: string;
  email: string | null;
  display_name: string | null;
  avatar_url: string | null;
};

export const onRequestGet: PagesFunction<AppEnv> = async ({ env, request }) => {
  const headers = {
    "Cache-Control": "no-store",
  };

  const session = await readSession(request, env);
  if (!session) {
    return Response.json({ authenticated: false }, { headers });
  }

  const user = await getDatabase(env)
    .prepare("SELECT id, email, display_name, avatar_url FROM users WHERE id = ?")
    .bind(session.userId)
    .first<UserRow>();

  if (!user) {
    return Response.json({ authenticated: false }, { headers });
  }

  return Response.json({
    authenticated: true,
    user: {
      id: user.id,
      email: user.email,
      displayName: user.display_name,
      avatarUrl: user.avatar_url,
    },
  }, { headers });
};
