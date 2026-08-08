import { createSakeTag, listSakeTags } from "../../_shared/sake";
import type { AppEnv } from "../../_shared/auth";

export const onRequestGet: PagesFunction<AppEnv> = async ({ env, request }) =>
  listSakeTags(request, env);

export const onRequestPost: PagesFunction<AppEnv> = async ({ env, request }) =>
  createSakeTag(request, env);
