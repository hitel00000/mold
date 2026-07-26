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

## 현재 상태 (2026-07-26 기준)

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결), Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md`), `runtime` 패키지 신설 및 **잔여 표면 마찰 완화 완결 (`runtime.App.CreateRecord` 초기 데이터 시딩 캡슐화로 외부 코드의 `resource`/`storage` 패키지 직접 임포트 0줄 실측)**, Phase 2 `mold dev` DX 실험 완결, Phase 4 Task 4.1 Cloudflare Workers TS+Hono+D1 Codegen 완성, 및 **Plan 계층 (`plan` 패키지) 실구현 완결 (`resource.NormalizeFields()` Level 0 승격 + `plan.Build()` 9개 타깃 100% 단일화 수렴 및 회귀 0건 실측 완결)**  
👉 **Post-MVP: 다음 세션 진행할 작업 선택 준비 (Cloudflare Auth/View/Blob 확장 vs 문서 스펙 구조 반영)**

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음 (`mold dev`로 가설 2 채택 완료).
- **Explicit Layering**: `resource.NormalizeFields()` (Layer 0 IR 원천) ➔ `plan.Build()` (Layer 1 Execution Plan) ➔ Target Packages (Layer 2) 3단계 단방향 계층 형성.

---

## 다음 할 일 (Post-MVP - 다음 세션 시작 시 확정 필요)

*다음 후보 중 하나를 다음 세션 시작 시 사람이 최종 확정하여 진행합니다:*

1. [x] ~~**후보 (a) `runtime` 패키지 잔여 표면 마찰 완화**~~ (완료: `runtime.App.CreateRecord` 도입으로 e2e 테스트 내 `resource`/`storage` 직접 임포트 0줄 달성)
2. 👉 **후보 (b) Cloudflare Workers TS Codegen 기능 확장 (Auth / View / Blob)**:
   - **사유**: Task 4.1에서는 스코프 단축을 위해 단순 CRUD만 포함했으나, 사케 앱 배포 환경처럼 실서비스 적용을 위해서는 Auth/Permission, HTML View, 또는 Blob R2 필드 생성기로 확장이 필요함.
3. 👉 **후보 (c) 문서 스펙(`docs/ir-spec.md`, `docs/resource-guide.md`) 구조 반영 갱신**:
   - **사유**: Plan 계층(`plan/plan.go`) 도입 및 `resource.NormalizeFields()`의 Layer 0 유틸 승격으로 IR 필드 파생과 실행 플랜 생성 아키텍처가 발전했으나, 기존 문서 스펙에 해당 3단계 계층 구조 및 파생 필드 처리 방식 설명이 업데이트되어 있지 않음.
   - **내용**: `docs/ir-spec.md` 및 `docs/resource-guide.md`에 Plan 계층 파이프라인 및 `NormalizeFields()`의 구조적 역할 설명을 갱신하여 문서와 코드 간 드리프트 차단.

