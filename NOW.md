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

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결), Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md`), `runtime` 패키지 신설 완결 (`runtime/` 신설, 조립부 6줄 축소), 및 **Phase 2 Task 2.1 `mold dev` DX 실험 완료 (`cmd/mold-dev` 신설, 파일 저장 ➔ HTTP POST /_mold/reload 자동 감지 핫컴파일 및 가설 2 [채택] 판정 완결)**  
👉 **Post-MVP: 다음 세션 진행할 작업 선택 준비 (runtime 잔여 표면 마찰 완화 vs Phase 4 멀티 타깃 생성기 준비)**

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음 (`mold dev`로 가설 2 채택 완료).

---

## 다음 할 일 (Post-MVP - 다음 세션 시작 시 확정 필요)

*다음 후보 중 하나를 다음 세션 시작 시 사람이 최종 확정하여 진행합니다:*

1. 👉 **후보 (a) `runtime` 패키지 잔여 표면 마찰 완화 (★ 잠정 권장안)**:
   - **사유**: `runtime` 패키지 도입으로 조립 코드는 6줄로 줄었으나, `cmd/runtime_e2e_test.go` 및 `cmd/mold-dev/dev_test.go`에서 확인되듯 초기 admin 계정 시딩 시 `resource.LoadAll` 및 `auth`/`storage` 서브패키지 직접 호출 마찰이 `runtime` 경계 외부에 남아있음.
   - **내용**: admin 시딩 헬퍼(예: `app.SeedAdmin(email, pass)`)나 Config 옵션을 `runtime` 패키지에 제공하여 외부 서브패키지 직접 참조를 캡슐화할지 검토.
2. **후보 (b) `Phase 4` Cloudflare Workers Static Generator 실험 착수**:
   - **사유**: Go 런타임 및 `mold dev` DX 가설 검증이 완료되었으므로, 두 번째 타깃인 TS+Hono+D1 정적 코드 생성기 실험 (Task 4.1)을 착수할 준비가 됨. (※ 채택된 가설 3 Plan 계층 구현을 함께 검토).

