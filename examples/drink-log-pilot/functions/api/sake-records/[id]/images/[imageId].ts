import { deleteSakeRecordImage } from "../../../../_shared/sake";
import type { AppEnv } from "../../../../_shared/auth";

export const onRequestDelete: PagesFunction<AppEnv> = async ({ env, params, request }) =>
  deleteSakeRecordImage(request, env, params.id as string, params.imageId as string);
