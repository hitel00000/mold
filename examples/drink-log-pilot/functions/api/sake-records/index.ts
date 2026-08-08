import { createSakeRecord, listSakeRecords } from "../../_shared/sake";
import type { AppEnv } from "../../_shared/auth";

export const onRequestGet: PagesFunction<AppEnv> = async ({ env, request }) =>
  listSakeRecords(request, env);

export const onRequestPost: PagesFunction<AppEnv> = async ({ env, request }) =>
  createSakeRecord(request, env);
