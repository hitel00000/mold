import { searchSakeRecords } from "../../_shared/sake";
import type { AppEnv } from "../../_shared/auth";

export const onRequestGet: PagesFunction<AppEnv> = async ({ env, request }) =>
  searchSakeRecords(request, env);
