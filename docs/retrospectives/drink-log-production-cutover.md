# Retrospective: `drink-log` 실제 프로덕션 D1/R2 컷오버 및 실배포 회고

> **작성일**: 2026-08-18  
> **관련 마일스톤**: Task 6.1 `drink-log` 실제 프로덕션 컷오버 (Production Cutover)  
> **배포 대상**: Cloudflare Pages (`https://drink-log.pages.dev`)  
> **원격 D1 데이터베이스**: `alcohol-log` (`cfccc3af-8f52-49dd-a77e-64b51a46e78f`)  
> **원격 R2 버킷**: `alcohol-log-images`  
> **이관 전 롤백 커밋**: `66c18679dbe436d9ae3564aaceb974aaad0edb78` (`main`)  
> **최종 머지 커밋**: `d91c045` (`main`, `--no-ff`)  
> **사전 백업 파일**: `backup_before_migration_20260818_212934.sql` (51,223 bytes)  

---

## 1. 개요

`examples/drink-log-pilot/`에서 Miniflare V8 Isolate 기반으로 사전 실측 검증된 D1 마이그레이션 SQL(`0001_drink_log_migration.sql`), 기본 태그 22개 Seed 멱등성, Pages Functions 엔드포인트 및 Mold Native REST API를 실제 Cloudflare 프로덕션 D1/R2 및 Pages에 무손실로 컷오버 완료하였다.

---

## 2. 사전 실측 계획 vs 실제 프로덕션 실행 대조

| 항목 | 사전 실측 계획 (`drink-log-real-migration.md`) | 실제 프로덕션 실행 결과 | 일치 여부 |
| :--- | :--- | :--- | :--- |
| **`users` 테이블** | `google:<sub>` 문자열 PK 유지 (2건) | `users` 2건 100% 무변경 보존 | ✅ 100% 일치 |
| **`sake_records` 테이블** | UUID ➔ INTEGER PK AUTOINCREMENT 변환 (19건) | 19건 전체 정수 ID(1~19) 발급, `legacy_id` 100% 보존 | ✅ 100% 일치 |
| **`sake_images` 테이블** | UUID ➔ INTEGER PK 변환, `record_id` 매핑 (18건) | 18건 전체 정수 ID 변환, `record_id` 정상 조인, R2 Key 보존 | ✅ 100% 일치 |
| **`tags` 테이블** | 기본 22개 슬러그 PK + 커스텀 2개 유지 (24건) | 24건 무변경 보존, Seed 2회 재실행 후에도 22개 멱등 유지 | ✅ 100% 일치 |
| **`record_tags` 테이블** | UUID ➔ INTEGER PK 변환, 정수 `sake_record_id` 매핑 (95건) | 95건 전체 정상 매핑 및 이관 완료 | ✅ 100% 일치 |
| **R2 객체 스토리지** | 키 변경/이동 없이 100% 원본 바이트 보존 | 실 배포 `/api/images?key=...`에서 원본 PNG/JPEG 이미지 정상 응답 | ✅ 100% 일치 |
| **Delete Orchestration** | 신규 레코드 생성 ➔ 단일 DELETE 호출 시 자식 및 연관관계 안전 삭제 | 실 프로덕션 스모크 테스트 생성 ➔ 204/200 삭제 ➔ 404 확인 완료 | ✅ 100% 일치 |

---

## 3. 프로덕션 실행 중 발견된 특이 사항 및 조치 (Operations Notes)

### 3.1 Cloudflare D1 SQL 배치 내 `BEGIN TRANSACTION` / `COMMIT` 제약
- **발견 사항**: `npx wrangler d1 execute --remote --file=...` 실행 시, Cloudflare D1 백엔드가 각 배치 파일을 자체 트랜잭션으로 자동 래핑하므로 명시적인 `BEGIN TRANSACTION;` 및 `COMMIT;` 문을 전달하면 에러를 반환함.
- **조치**: 마이그레이션 SQL에서 D1 내부 트랜잭션과 충돌하는 `BEGIN TRANSACTION;` / `COMMIT;` 및 `PRAGMA` 문을 제거하여 단일 배치 파일로 원자적 적용 완료.

### 3.2 Pages Functions 라우터 일원화 (`functions/api/[[path]].ts`)
- **발견 사항**: 초기 배포 시 파일 단위 라우팅(`functions/api/sake-records`, `functions/api/auth/...`)과 catch-all 라우팅 간 우선순위 충돌로 인해 로그인 리다이렉트 및 REST 엔드포인트 404/SPA Fallback 현상 발생.
- **조치**: 모든 인증 핸들러(`google/login`, `google-callback`, `logout`, `me`), 이미지 서빙 핸들러(`images`), Mold Native REST CRUD 엔드포인트를 단일 통합 라우터([`functions/api/[[path]].ts`](functions/api/[[path]].ts))로 일원화.

### 3.3 이미지 Base64 Data URL ➔ R2 자동 업로드
- **발견 사항**: 프론트엔드가 JSON 바디(`image_key`)에 Base64 Data URL을 담아 전송할 때 D1 `image_key NOT NULL` 제약 조건 위반 발생.
- **조치**: `POST /api/sake_images` 핸들러 내에 Base64 파서를 탑재하여 즉시 R2 버킷에 저장하고 D1에는 정규 R2 키(`images/...`)를 기록하도록 개선.

### 3.4 렌더링 시 이미지 URL 자동 합성
- **발견 사항**: DB 조회 시 `image_key`는 존재하나 프론트엔드 UI 컴포넌트가 기대하는 `data_url`이 `undefined`로 전달되어 `img alt`만 출력되는 현상 발견.
- **조치**: `buildSakeRecordEntry`에서 `image_key`를 감지하여 `/api/images?key=...` 엔드포인트 URL로 자동 합성하도록 보완.

---

## 4. 최종 결론

사전 분석 및 Miniflare V8 실측에서 수립된 마이그레이션 계획이 실제 프로덕션 D1/R2 데이터에 대해 **단 1건의 데이터 유실이나 스키마 불일치 없이 100% 정확하게 적용**되었음을 확인하였다.  
`drink-log` 레포지토리는 `main` 브랜치에 `--no-ff` 머지 및 원격 푸시(`d91c045`)가 완료되었다.
