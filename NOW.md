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

## 현재 상태 (2026-07-24 기준)

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결) 및 **Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md` 작성 및 가설 1 기각 / 가설 3 채택 및 구현 보류 판정 확정)**  
👉 **Post-MVP Phase 1 완결: 다음 세션 진행할 작업 선택 준비 (근본 원인 A/B 해결을 위한 `runtime` 패키지 신설 vs Phase 2 DX 실험)**

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음.

---

## 다음 할 일 (Post-MVP - 다음 세션 시작 시 확정 필요)

*다음 두 후보 중 하나를 다음 세션 시작 시 사람이 최종 확정하여 진행합니다:*

1. 👉 **후보 (a) `runtime` 패키지 신설 착수 (★ 잠정 권장안)**: 
   - **사유**: 가설 1 기각이 가장 명확히 실증된 문제이며, 근본 원인 B(App Container 부재)를 먼저 해결하여 `main.go`를 간결하게 다듬어두어야 Phase 2 (`mold dev` 파일 워처) 로직을 얹기 수월함.
   - **내용**: `github.com/hitel00000/mold/runtime` 패키지(`runtime.App`, `runtime.Config`)를 최소 스코프로 신설하고 `drink-log` `main.go`를 10줄 이내로 단축.
2. **후보 (b) `Phase 2` DX 실험 착수**: `resources/*.yaml` 파일 저장(`Ctrl + S`) 시 백그라운드 원자적 리로드가 동작하는 `mold dev` DX실험(Task 2.1) 진행.
