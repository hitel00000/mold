// Delete Orchestration Function for SakeRecord and child SakeImages
// Guarantees Abort Contract, Session-based HTTP API calls, and Idempotent Retry

interface Env {
  DB: D1Database;
}

export async function onRequestDelete(context: { request: Request; params: { id: string }; env: Env }) {
  const { request, params, env } = context;
  const recordId = params.id;
  const cookieHeader = request.headers.get('Cookie') || '';

  // 1. Authenticate Session via mold_session Cookie
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

  // 2. Fetch Parent SakeRecord
  const record = await env.DB.prepare('SELECT * FROM "sake_records" WHERE id = ?').bind(recordId).first<any>();
  if (!record) {
    return Response.json({ error: { code: 'NOT_FOUND', message: 'record not found' } }, { status: 404 });
  }
  if (record.owner_id != sess.user_id) {
    return Response.json({ error: { code: 'FORBIDDEN', message: 'forbidden' } }, { status: 403 });
  }

  // 3. Fetch Child SakeImages
  const images = await env.DB.prepare('SELECT * FROM "sake_images" WHERE record_id = ?').bind(recordId).all<any>();
  const childImages = images.results || [];

  // 4. Sequential Child Images Deletion via HTTP API endpoints
  const urlOrigin = new URL(request.url).origin;
  for (const img of childImages) {
    // Step A: Delete R2 Blob via HTTP API DELETE /api/sake_images/{id}/blob/image_key with session cookie
    if (img.image_key) {
      const resBlobDel = await fetch(`${urlOrigin}/api/sake_images/${img.id}/blob/image_key`, {
        method: 'DELETE',
        headers: { Cookie: cookieHeader },
      });

      // Idempotent Retry: 200 OK or 404 Not Found (already deleted) is accepted as clean
      if (resBlobDel.status !== 200 && resBlobDel.status !== 404) {
        // Abort Contract: Partial failure on R2 blob deletion -> Stop and return 500!
        return Response.json({ error: { code: 'RECORD_DELETE_PARTIAL_FAILURE', message: `failed to delete blob for image ${img.id}` } }, { status: 500 });
      }
    }

    // Step B: Delete SakeImage Record via HTTP API DELETE /api/sake_images/{id} with session cookie
    const resImgDel = await fetch(`${urlOrigin}/api/sake_images/${img.id}`, {
      method: 'DELETE',
      headers: { Cookie: cookieHeader },
    });

    if (resImgDel.status !== 200 && resImgDel.status !== 404) {
      // Abort Contract: Partial failure on DB image deletion -> Stop before deleting SakeRecord!
      return Response.json({ error: { code: 'RECORD_DELETE_PARTIAL_FAILURE', message: `failed to delete image row ${img.id}` } }, { status: 500 });
    }
  }

  // 5. Delete Child RecordTag relations if any
  await env.DB.prepare('DELETE FROM "record_tags" WHERE sake_record_id = ?').bind(recordId).run();

  // 6. Delete SakeRecord Parent via HTTP API DELETE /api/sake_records/{id} with session cookie
  const resParentDel = await fetch(`${urlOrigin}/api/sake_records/${recordId}`, {
    method: 'DELETE',
    headers: { Cookie: cookieHeader },
  });

  if (resParentDel.status === 200 || resParentDel.status === 404) {
    return Response.json({ data: { deleted: true, id: Number(recordId) } }, { status: 200 });
  }

  return Response.json({ error: { code: 'PARENT_DELETE_FAILED', message: 'failed to delete parent sake record' } }, { status: resParentDel.status });
}
