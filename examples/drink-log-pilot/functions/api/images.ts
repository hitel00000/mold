import { getDatabase, getImagesBucket, readSession, type AppEnv } from "../_shared/auth";

export const onRequestGet: PagesFunction<AppEnv> = async ({ env, request }) => {
  const session = await readSession(request, env);
  if (!session) {
    return new Response("Unauthorized", { status: 401 });
  }

  const key = new URL(request.url).searchParams.get("key");
  if (!key) {
    return new Response("Missing key", { status: 400 });
  }

  const image = await getDatabase(env)
    .prepare(
      `SELECT owner_id, mime_type FROM sake_images
       WHERE image_key = ? OR thumbnail_key = ?`,
    )
    .bind(key, key)
    .first<{ owner_id: string; mime_type: string }>();

  if (!image) {
    return new Response("Not found", { status: 404 });
  }

  if (image.owner_id !== session.userId) {
    return new Response("Forbidden", { status: 403 });
  }

  const object = await getImagesBucket(env).get(key);
  if (!object) {
    return new Response("Not found", { status: 404 });
  }

  return new Response(object.body, {
    headers: {
      "Content-Type": object.httpMetadata?.contentType ?? image.mime_type,
      "Cache-Control": "private, max-age=3600",
    },
  });
};
