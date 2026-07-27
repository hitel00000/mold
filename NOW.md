# NOW

> 이 문서는 새 세션(사람이든 AI든)이 프로젝트에 합류할 때 가장 먼저 읽는 목차 문서입니다.
> 주요 마일스톤이나 백로그 변경 시 갱신합니다.

---

## 읽는 순서

새 세션은 아래 순서로 문서를 읽고 시작합니다. 이 문서(NOW.md)만 읽고 코드를 바로 짜지 마십시오.

1. `README.md` — 프로젝트 소개, 핵심 개념 및 구동 예시
2. `docs/philosophy.md` — 존재 이유, 핵심 철학 및 비타협적 원칙
3. `AGENTS.md` — 프로젝트 철학, 하지 않는 것, AI 작업 가이드라인
4. `docs/ir-spec.md` — Resource IR의 유일한 스펙 (구조체, 검증 규칙)
5. `docs/resource-guide.md` — Resource YAML 작성 스펙 및 Good/Bad 패턴 가이드
6. `TASKS.md` — MVP 완료 상태, 가설과 완료 조건을 담은 실증 백로그
7. `docs/retrospectives/cloudflare-codegen-review.md` — 가장 최근 회고 문서 (Cloudflare Codegen 리뷰 반려 및 보안/스펙 수정 회고)
8. 이 문서(NOW.md)의 "다음 할 일" 섹션

---

## 현재 상태 (2026-07-26 기준)

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결), Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md`), `runtime` 패키지 신설 및 **잔여 표면 마찰 완화 완결 (`runtime.App.CreateRecord` 초기 데이터 시딩 캡슐화로 외부 코드의 `resource`/`storage` 패키지 직접 임포트 0줄 실측)**, Phase 2 `mold dev` DX 실험 완결, Phase 4 Task 4.1 Cloudflare Workers TS+Hono+D1 Codegen 완성, **Plan 계층 (`plan` 패키지) 실구현 완결 (`resource.NormalizeFields()` Level 0 승격 + `plan.Build()` 9개 타깃 100% 단일화 수렴 및 회귀 0건 실측 완결)**, **Cloudflare Workers TS Codegen 기능 확장 & 후속 리뷰 결함 수정 완결**, **Plan 계층 문서 스펙 갱신 완결**, **Cloudflare R2 Multi-Blob Orphan 동기 보상 삭제 완결**, **복합 Unique Constraint 스펙 반영 & N:M/OAuth 설계 결정 문서화 완결**, **Task 5.1 복합 Unique Constraint (`unique_together`) Go 코어 & Cloudflare D1 Target 실구현 완결**, 및 **Task 5.2 N:M Join Resource 패턴 drink-log 격리 파일럿 완결 (`examples/drink-log-pilot` 3개 Resource 정의, DDL Parity 비교, Miniflare 5대 E2E 시나리오 100% PASS, YAML loader constraints 파싱 결함 수정 및 3대 마찰 기록 완결)**  
👉 **Post-MVP: 다음 세션 백로그 확정 (Task 5.3 OAuth 세션 발급 Escape Hatch vs Nullable Ownership IR 확장 등)**

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음 (`mold dev`로 가설 2 채택 완료).
- **Explicit Layering**: `resource.NormalizeFields()` (Layer 0 IR 원천) ➔ `plan.Build()` (Layer 1 Execution Plan) ➔ Target Packages (Layer 2) 3단계 단방향 계층 형성.

---

## 다음 할 일 (Post-MVP - 다음 세션 시작 시 확정 필요)

*다음 후보 중 하나를 다음 세션 시작 시 사람이 최종 확정하여 진행합니다:*

1. [x] ~~**Cloudflare Workers TS Codegen 기능 확장 및 후속 리뷰 결함 수정 (Auth / View / Blob)**~~ (완료: Auth 헤더 우회 제거, PBKDF2 해싱 및 /login 검증, 1-Step Multipart Create & Atomic Rollback, HTML Sanitizer 보강, SQL 리터럴 홑따옴표 수정, Miniflare V8 Isolate 8대 시나리오 실측 100% PASS 완결)
2. [x] ~~**문서 스펙(`docs/ir-spec.md`, `docs/resource-guide.md`) 구조 반영 갱신**~~ (완료: Plan 계층 3단계 계층 구조 1.5절 신설, FK 파생 필드의 NormalizeFields & Nullable: true 골든 패리티 서술, Section 7 결정 사항 채택, resource-guide.md belongs_to FK 자동 파생 팁 추가 완료)
3. [x] ~~**Cloudflare R2 Multi-Blob Orphan 객체 동기 보상 삭제 (Direction C)**~~ (완료: N번째 blob 업로드 실패 시 D1 hard delete ➔ R2 보상 삭제 순서 정렬, `BLOB_ORPHAN_CLEANUP_FAILED` 명시적 보고 및 SakePost Miniflare 실측 시나리오 1·2 100% PASS 완료)
4. [x] ~~**복합 Unique Constraint 스펙 반영 및 N:M/OAuth 설계 결정 문서화**~~ (완료: `constraints.unique_together` 최상위 노드 문법, Partial Unique Index 전환 스펙, OAuth 세션 발급 Escape Hatch 확장 방향, N:M Join Resource Good 패턴 가이드 및 NULL 우회 방지 FK `nullable: false` 필수성 반영 완료)
5. [x] ~~**Task 5.1: 복합 Unique Constraint (`constraints.unique_together`) IR/DDL/Validation 실구현 (Go 코어 + Cloudflare D1 Target)**~~ (완료: `plan.Plan.UniqueTogether` 수렴, `resource.Validate` 메타스키마 검증, `adapters/sqlite` & `codegen/cloudflare` Partial Unique Index DDL 생성, Go/TS Create/Update 400 `INVALID_INPUT` 패리티 100%, Miniflare 5대 시나리오 실측 100% PASS 완료)
6. [x] ~~**Task 5.2: N:M (`record_tags`) Join Resource 패턴을 `drink-log` 파일럿 적용 및 마찰 수집**~~ (완료: `examples/drink-log-pilot`에 `SakeRecord`/`Tag`/`RecordTag` 작성, DDL Parity 비교, Miniflare 5대 시나리오 100% PASS, YAML loader `constraints` 파싱 결함 `fix(resource)` 수정, 3대 마찰 기록 완결)
7. 👉 **후보 (a) Task 5.3: 세션 발급 Escape Hatch (`IssueSessionForUser` 등) 구현 및 `drink-log` Google OAuth 연동**:
   - **사유**: 외부 OAuth 검증 완료 후 Mold 세션을 발급하는 공개 API (가칭 `runtime.App.IssueSessionForUser` 또는 TS Target `/api/_auth/issue_session` 등)를 구현하고 `drink-log` 소셜 로그인 후 세션 쿠키 발급 및 권한 평가 실측.
8. 👉 **후보 (b) Nullable Ownership 표현을 위한 IR 확장 (마찰 A 해결안 논의 후 적용)**:
   - **사유**: Task 5.2 파일럿에서 포착된 `owner_id = NULL` (기본 태그 등 공개 레코드)과 `owner_id = <user_id>` (커스텀 소유 레코드)가 혼재된 도메인을 IR `permissions.read: owner` 하에서 자연스럽게 지원하기 위한 IR/auth 확장 논의.
9. 👉 **후보 (c) PostgreSQL / MySQL Storage Adapter 또는 REST Remote Backend Adapter 추가**:
   - **사유**: `docs/philosophy.md` 마세라티 원칙에 따라 필요할 때 추가하도록 미뤄둔 다중 Storage 백엔드 확장.

