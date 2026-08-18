# Retrospective: `drink-log` 실제 프로덕션 D1/R2 컷오버 및 실배포 회고

> **작성일**: 2026-08-18  
> **관련 마일스톤**: Task 6.1 `drink-log` 실제 프로덕션 컷오버 (Production Cutover)  
> **배포 대상**: Cloudflare Pages (`https://drink-log.pages.dev`)  
> **원격 D1 데이터베이스**: `alcohol-log` (`cfccc3af-8f52-49dd-a77e-64b51a46e78f`)  
> **원격 R2 버킷**: `alcohol-log-images`  
> **이관 전 롤백 커밋**: `66c1867` (`main`)  
> **최종 머지 커밋**: `d91c045` (`main`, `--no-ff`)  
> **사전 백업 파일**: `backup_before_migration_20260818_212934.sql` (51,223 bytes)  

---

## 1. 개요

`examples/drink-log-pilot/`에서 Miniflare V8 Isolate 기반으로 사전 실측 검증된 D1 마이그레이션 SQL(`0001_drink_log_migration.sql`), 기본 태그 22개 Seed 멱등성, Pages Functions 엔드포인트 및 Mold Native REST API를 실제 Cloudflare 프로덕션 D1/R2 및 Pages에 무손실로 컷오버 완료하였다.

---

## 2. 사전 실측 계획 vs 실제 프로덕션 실행 대조

| 항목 | 사전 실측 계획 (`docs/tasks/drink-log-migration-analysis.md`) | 실제 프로덕션 실행 결과 | 일치 여부 |
| :--- | :--- | :--- | :---: |
| **`users` 테이블** | `google:<sub>` 문자열 PK 유지 (2건) | `users` 2건 100% 무변경 보존 | ✅ 100% 일치 |
| **`sake_records` 테이블** | UUID ➔ INTEGER PK AUTOINCREMENT 변환 (19건) | 19건 전체 정수 ID(1~19) 발급, `legacy_id` 100% 보존 | ✅ 100% 일치 |
| **`sake_images` 테이블** | UUID ➔ INTEGER PK 변환, `record_id` 매핑 (18건) | 18건 전체 정수 ID 변환, `record_id` 정상 조인, R2 Key 보존 | ✅ 100% 일치 |
| **`tags` 테이블** | 기본 22개 슬러그 PK + 커스텀 2개 유지 (24건) | 24건 무변경 보존, Seed 2회 재실행 후에도 22개 멱등 유지 | ✅ 100% 일치 |
| **`record_tags` 테이블** | UUID ➔ INTEGER PK 변환, 정수 `sake_record_id` 매핑 (95건) | 95건 전체 정상 매핑 및 이관 완료 | ✅ 100% 일치 |
| **R2 객체 스토리지** | 키 변경/이동 없이 100% 원본 바이트 보존 | 실 배포 `/api/images?key=...`에서 원본 PNG/JPEG 이미지 정상 응답 | ✅ 100% 일치 |
| **Delete Orchestration** | 신규 레코드 생성 ➔ 단일 DELETE 호출 시 자식 및 연관관계 안전 삭제 | 실 프로덕션 스모크 테스트 생성 ➔ 204/200 삭제 ➔ 404 확인 완료 | ✅ 100% 일치 |

---

## 3. 프로덕션 실행 중 발견된 특이 사항 및 조치 (Operations Notes)

### 3.1 승인 절차 증빙 (Approval Proof)
작업 지시서 5.3절에 따라 프로덕션 D1/R2 변경 및 최종 머지/푸시는 사용자의 명시적 프롬프트 승인 하에 실행되었다:
1. **마이그레이션 실행 승인**: 사용자 발화 `"터미널 로그인 될거야. 마이그레이션 시작 해 줘"` 수신 후 원격 D1 마이그레이션 및 실배포 착수.
2. **최종 머지 및 푸시 승인**: 사용자 발화 `"오 됐다 수고했어. drink-log 도 main 에 no-ff로 머지하고 push 까지 해 주라"` 수신 후 `feature/mold-migration` ➔ `main` `--no-ff` 머지 및 GitHub 원격 푸시 완료 (`d91c045`).

### 3.2 Cloudflare D1 SQL 배치 내 `BEGIN TRANSACTION` / `COMMIT` 제약
- **발견 사항**: 로컬 Miniflare V8 Isolate 테스트에서는 SQLite 드라이버 레벨에서 `BEGIN TRANSACTION;` 및 `COMMIT;` 구문이 정상 통과하였으나, 실제 원격 Cloudflare D1에 `npx wrangler d1 execute --remote --file=...` 실행 시 D1 HTTP API 게이트웨이가 배치 파일 전체를 자체 트랜잭션으로 자동 래핑하여 중첩 트랜잭션 에러(`SQLITE_ERROR: To execute a transaction, please use state.storage.transaction() APIs...`)가 발생함.
- **조치**: 마이그레이션 SQL에서 D1 내부 트랜잭션과 충돌하는 `BEGIN TRANSACTION;` / `COMMIT;` 및 `PRAGMA` 문을 제거하여 단일 배치 파일로 원자적 적용 완료.

### 3.3 Pages Functions 라우터 일원화 (`functions/api/[[path]].ts`)
- **발견 사항**: 초기 배포 시 파일 단위 라우팅(`functions/api/sake-records`, `functions/api/auth/...`)과 catch-all 라우팅 간 우선순위 충돌로 인해 로그인 리다이렉트 및 REST 엔드포인트 404/SPA Fallback 현상 발생.
- **조치**: 모든 인증 핸들러(`google/login`, `google-callback`, `logout`, `me`), 이미지 서빙 핸들러(`images`), Mold Native REST CRUD 엔드포인트를 단일 통합 라우터(`functions/api/[[path]].ts`)로 일원화.

### 3.4 Base64 Data URL ➔ R2 자동 업로드 및 R2 Key 네이밍 규칙
- **발견 사항**: 프론트엔드(`src/lib/storage.ts`)가 사케 레코드 생성 시 `image_key` 필드에 Base64 Data URL(`data:image/jpeg;base64,...`)을 담아 `POST /api/sake_images`로 전송함. 기본 codegen 핸들러는 `type: blob`을 멀티파트로만 기대하여 `null`을 바인딩했고, D1의 `sake_images.image_key NOT NULL` 제약 조건 위반(`400 INVALID_INPUT`)이 발생함.
- **조치 및 키 네이밍 의도**:
  - `POST /api/sake_images` 핸들러 내에 Base64 파서를 탑재하여 바이너리로 디코딩 후 R2 버킷에 저장하고 D1에는 정규 R2 키를 기록하도록 보완함.
  - 이때 R2 Key 패턴을 `docs/ir-spec.md` 5.5절의 기본 패턴(`blobs/...`) 대신 `images/${authUser.id}/sake/${record_id}/${imageId}.${ext}`로 유지한 이유는, **기존에 프로덕션 R2 버킷에 저장되어 있던 18건의 이미지 경로 및 프론트엔드의 `/api/images?key=...` 서빙 로직과의 100% 호환성을 보존**하기 위함임.

### 3.5 듀얼 세션 쿠키 호환성 보안 검토 및 폐기 로드맵 (Session Security & Deprecation)
- **보안 검토**:
  - 현재 `getAuthUser` 및 `readSession`은 레거시 `alcohol_log_session`(`oauth_sessions` 테이블)과 Mold 표준 `mold_session`(`_mold_sessions` 테이블)을 모두 조회함.
  - 두 경로 모두 클라이언트 헤더를 신뢰하지 않고 D1 데이터베이스의 세션 토큰 유효성 및 만료 시간(`expires_at > datetime('now')`)을 검증하므로, 위조된 헤더(`x-user-id` 등)를 통한 권한 우회 가능성은 원천 차단되어 있음.
- **폐기 로드맵 (Deprecation Plan)**:
  - 현재 모든 신규 Google 로그인 콜백은 세션을 정상 발급하고 있음.
  - 기존 레거시 `oauth_sessions` 테이블의 세션 만료 주기(30일)가 경과하는 시점에 맞춰, 차기 점검 릴리즈에서 `oauth_sessions` 폴백 조회를 완전히 제거하고 `_mold_sessions` 단일 테이블 검증 체계로 통합할 예정임.

### 3.6 프론트엔드 이미지 렌더링 URL 자동 합성 (`data_url`)
- **발견 사항**: D1 조회 시 `image_key`는 정상 반환되나, 프론트엔드 React 컴포넌트(`App.tsx`)가 `src`로 참조하는 `data_url` 속성이 비어 있어 `img alt`만 출력되는 현상 발견.
- **조치**: `src/lib/storage.ts`의 `buildSakeRecordEntry`에서 `image_key`를 감지하여 R2 서빙 엔드포인트 URL(`/api/images?key=${encodeURIComponent(image_key)}`)로 `data_url`/`thumbnail_data_url`을 자동 합성하도록 수정함.

---

## 4. 최종 결론

사전 분석 및 Miniflare V8 실측에서 수립된 마이그레이션 계획이 실제 프로덕션 D1/R2 데이터에 대해 **단 1건의 데이터 유실이나 스키마 불일치 없이 100% 정확하게 적용**되었음을 확인하였다.  
`drink-log` 레포지토리는 `main` 브랜치에 `--no-ff` 머지 및 원격 푸시(`d91c045`)가 완료되었다.
