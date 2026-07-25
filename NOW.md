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
7. `docs/retrospectives/` 안의 가장 최근 회고 문서
8. 이 문서(NOW.md)의 "다음 할 일" 섹션

---

## 현재 상태 (2026-07-25 기준)

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결), Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md`), `runtime` 패키지 신설 완결 (`runtime/` 신설, 조립부 6줄 축소), Phase 2 `mold dev` DX 실험 완결, 및 **Phase 4 Task 4.1 Cloudflare Workers TS+Hono+D1 Codegen 완성 (`codegen/cloudflare` 신설, Miniflare V8 Isolate `wrangler dev` 라이브 실행 검증 및 Go API 응답 패리티 100% 확인 완료)**  
👉 **Post-MVP: 다음 세션 진행할 작업 선택 준비 (Plan 계층 `plan` 패키지 추출 착수 여부 vs runtime 잔여 마찰 완화 vs Cloudflare Auth/View/Blob 확장)**

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음 (`mold dev`로 가설 2 채택 완료).

---

## 다음 할 일 (Post-MVP - 다음 세션 시작 시 확정 필요)

*다음 후보 중 하나를 다음 세션 시작 시 사람이 최종 확정하여 진행합니다:*

1. 👉 **후보 (a) `Plan` 계층 (`plan` 패키지) 추출 및 가설 3 실구현 착수 (★ 잠정 권장안)**:
   - **사유**: 두 번째 타깃(Cloudflare Workers TS)이 실제 `wrangler dev` 라이브 검증까지 완결되어 가설 3의 전제 조건이 충족됨. 타입 분기 6➔9곳, 필드 루프 11➔14곳으로 중복이 확증되었으며, 문제는 착수 여부가 아니라 스코프 범위(방향 A: Go 코어 패키지 전체 리팩터링 vs 이번 생성기 수준의 국소적 Plan 추출)의 확정임.
   - **내용**: `plan` 중간 스냅샷/매퍼 패키지를 작성하여 DDL, API, Codegen의 타입 해석을 단일 지점으로 수렴시키는 리팩터링 수행.
2. **후보 (b) `runtime` 패키지 잔여 표면 마찰 완화**:
   - **사유**: `runtime` 패키지 도입으로 조립 코드는 6줄로 줄었으나, 초기 admin 계정 시딩 시 `resource.LoadAll` 및 `auth`/`storage` 서브패키지 직접 호출 마찰이 `runtime` 경계 외부에 남아있음.
   - **내용**: admin 시딩 헬퍼(예: `app.SeedAdmin(email, pass)`)나 Config 옵션을 `runtime` 패키지에 제공하여 외부 서브패키지 직접 참조를 캡슐화.
3. **후보 (c) Cloudflare Workers TS Codegen 기능 확장 (Auth / View / Blob)**:
   - **사유**: Task 4.1에서는 스코프 단축을 위해 단순 CRUD만 포함했으나, 사케 앱 배포 환경처럼 실서비스 적용을 위해서는 Auth/Permission, HTML View, 또는 Blob R2 필드 생성기로 확장이 필요함.

