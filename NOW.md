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
7. `docs/tasks/drink-log-migration-analysis.md` — drink-log 프로덕션 이관 분석 명세서 (Revision 1~10)
8. `docs/retrospectives/drink-log-migration.md` — drink-log 이관 회고 문서 (10차례 분석/구현 교훈)
9. 이 문서(NOW.md)의 "다음 할 일" 섹션

---

## 현재 상태 (2026-07-31 기준)

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결), Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md`), Phase 2 `mold dev` DX 실험 완결, Phase 4 Cloudflare Workers TS+Hono+D1 Codegen & Plan 계층 실구현 완결, Phase 5 Task 5.1~5.5 (복합 Unique Constraint, N:M Join Resource, OAuth 세션발급 Escape Hatch, Nullable Ownership 패리티, List Owner 필터링, 관계 조인 조회 `?include=`) 완결, 및 **Phase 6 Task 6.1 `drink-log` 실제 프로덕션 이관 완결 (Option C INTEGER AUTOINCREMENT PK, R2 키 보존, `tags` `slug` + `unique_together` Idempotency, 5개 테이블 `soft_delete: true`, Delete Orchestration 세션 HTTP API Abort 계약, Miniflare E2E 6대 시나리오 100% PASS)**, **Task 6.2 [독립 bugfix] Cloudflare TS Target D1 DDL `FOREIGN KEY ... ON DELETE RESTRICT` 명시적 강제 픽스 완결**.  
👉 **Post-MVP: 다음 백로그 확정 (후보 (c) PostgreSQL/MySQL Storage Adapter 등)**

**추가 진행**: `docs/getting-started.md` 튜토리얼 및 `examples/quickstart/` 작성 완료.
튜토리얼을 실제로 손으로 따라가며 검증(dogfooding)하는 과정에서 field-level 권한
부재로 인한 privilege escalation 마찰(User.role, Post.author_id) 및 부수 마찰
3건을 발견하여 `TASKS.md` Phase 7로 등재함 (아직 미착수).

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음 (`mold dev`로 가설 2 채택 완료).
- **Explicit Layering**: `resource.NormalizeFields()` (Layer 0 IR 원천) ➔ `plan.Build()` (Layer 1 Execution Plan) ➔ Target Packages (Layer 2) 3단계 단방향 계층 형성.

---

## 다음 할 일 (Post-MVP - 다음 세션 시작 시 확정 필요)

*다음 후보 중 하나를 다음 세션 시작 시 사람이 최종 확정하여 진행합니다:*

1. [x] ~~**Task 6.1: `drink-log` 실제 프로덕션 이관 구현 및 E2E 실측 검증**~~ (완료)
2. [x] ~~**Task 6.2: Cloudflare Target D1 DDL `FOREIGN KEY ... ON DELETE RESTRICT` 강제 픽스**~~ (완료: 커밋 `9d74c02`)
3. 👉 **Phase 7 (Task 7.1~7.4): getting-started.md 실측 중 발견된 field-level 권한 부재 마찰 해소** — 신규 등재, 미착수. 특히 Task 7.1(IR 확장 채택/기각 판정)이 나머지 3개 Task의 방향을 결정하므로 우선순위 최상단으로 올림.
4. **후보 (c) PostgreSQL / MySQL Storage Adapter 또는 Remote REST Backend Adapter 추가**:
   - **사유**: `docs/philosophy.md` 마세라티 원칙에 따라 필요할 때 추가하도록 미뤄둔 다중 Storage 백엔드 확장. Phase 7 대비 우선순위 낮음 (Phase 7이 실사용 중 발견된 보안 마찰이라 우선).
