import { addSakeRecordImage } from "../../../_shared/sake";
import type { AppEnv } from "../../../_shared/auth";

export const onRequestPost: PagesFunction<AppEnv> = async ({ env, params, request }) =>
  addSakeRecordImage(request, env, params.id as string);
