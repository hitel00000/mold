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

## 현재 상태 (2026-07-28 기준)

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결), Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md`), `runtime` 패키지 신설 및 **잔여 표면 마찰 완화 완결 (`runtime.App.CreateRecord` 초기 데이터 시딩 캡슐화로 외부 코드의 `resource`/`storage` 패키지 직접 임포트 0줄 실측)**, Phase 2 `mold dev` DX 실험 완결, Phase 4 Task 4.1 Cloudflare Workers TS+Hono+D1 Codegen 완성, **Plan 계층 (`plan` 패키지) 실구현 완결 (`resource.NormalizeFields()` Level 0 승격 + `plan.Build()` 9개 타깃 100% 단일화 수렴 및 회귀 0건 실측 완결)**, **Cloudflare Workers TS Codegen 기능 확장 & 후속 리뷰 결함 수정 완결**, **Plan 계층 문서 스펙 갱신 완결**, **Cloudflare R2 Multi-Blob Orphan 동기 보상 삭제 완결**, **복합 Unique Constraint 스펙 반영 & N:M/OAuth 설계 결정 문서화 완결**, **Task 5.1 복합 Unique Constraint (`unique_together`) Go 코어 & Cloudflare D1 Target 실구현 완결**, **Task 5.2 N:M Join Resource 패턴 drink-log 파일럿 적용 완결**, **Task 5.3 OAuth 세션 발급 Escape Hatch 구현 완결**, **후보 (a) Nullable Ownership Go/TS Target 패리티 완결**, **Task 5.4 List 액션 Owner 권한 필터링 결함 해결 완결 (Go/TS 7대 시나리오 Miniflare dispatchFetch V8 Isolate real HTTP & SSR HTML View 실측 100% PASS 완료)**, 및 **Task 5.5 관계 조인 조회(`?include=`) API 지원 완결 (Go Transport/View & Cloudflare TS Codegen 삼중 적용, N+1 배치 조회 및 4-시나리오 권한 메트릭스 100% PASS 실측 완료)**  
👉 **Post-MVP: 다음 세션 백로그 확정 (후보 (c) 채택 논의)**

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음 (`mold dev`로 가설 2 채택 완료).
- **Explicit Layering**: `resource.NormalizeFields()` (Layer 0 IR 원천) ➔ `plan.Build()` (Layer 1 Execution Plan) ➔ Target Packages (Layer 2) 3단계 단방향 계층 형성.

---

## 다음 할 일 (Post-MVP - 다음 세션 시작 시 확정 필요)

*다음 후보 중 하나를 다음 세션 시작 시 사람이 최종 확정하여 진행합니다:*

1. [x] ~~**Task 5.1: 복합 Unique Constraint (`constraints.unique_together`) IR/DDL/Validation 실구현 (Go 코어 + Cloudflare D1 Target)**~~ (완료: `plan.Plan.UniqueTogether` 수렴, `resource.Validate` 메타스키마 검증, `adapters/sqlite` & `codegen/cloudflare` Partial Unique Index DDL 생성, Go/TS Create/Update 400 `INVALID_INPUT` 패리티 100%, Miniflare 5대 시나리오 실측 100% PASS 완료)
2. [x] ~~**Task 5.2: N:M (`record_tags`) Join Resource 패턴을 `drink-log` 파일럿 적용 및 마찰 수집**~~ (완료: `examples/drink-log-pilot`에 `SakeRecord`/`Tag`/`RecordTag` 작성, DDL Parity 비교, Miniflare 5대 시나리오 100% PASS, YAML loader `constraints` 파싱 결함 `fix(resource)` 수정, 4대 마찰 기록 완결)
3. [x] ~~**Task 5.3: 세션 발급 Escape Hatch (`IssueSessionForUser` 등) 구현 및 OAuth 연동 기반 마련**~~ (완료: Go in-process `IssueSessionForUser` API, HTTP 라우터 무등록 신뢰 경계, Cloudflare D1 외부 세션 직접 작성 스펙 문서화 및 Miniflare 실측 100% PASS 완료)
4. [x] ~~**후보 (a) Nullable Ownership IR 표현 및 Go/TS Target 평가 엔진 패리티 완결**~~ (완료: IR 구조 변경 없이 옵션 D 채택, Go 런타임 & Cloudflare TS generator 템플릿 정밀화, Node/Miniflare V8 Isolate real HTTP dispatch 15대 조합 시나리오 100% PASS 완료)
5. [x] ~~**Task 5.4: List 액션 Owner 권한 필터링 결함 해결 (Go 런타임 & Cloudflare TS Target)**~~ (완료: `storage.Query`에 `OwnerFilter` 추가, SQL 레벨 `(ownership_field = ? OR ownership_field IS NULL)` 자동 주입, Go/TS API 및 SSR HTML View 7개 시나리오 Miniflare 실측 100% PASS 완료)
6. [x] ~~**후보 (b) 관계 조인 조회(Eager Loading / Include query) API 지원**~~ (완료: Task 5.5 참조)
7. 👉 **후보 (c) PostgreSQL / MySQL Storage Adapter 또는 Remote REST Backend Adapter 추가**:
   - **사유**: `docs/philosophy.md` 마세라티 원칙에 따라 필요할 때 추가하도록 미뤄둔 다중 Storage 백엔드 확장. (Task 5.5 완료 시점까지 관측된 별도 신규 마찰 없음)


