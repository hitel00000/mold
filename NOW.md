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

## 현재 상태 (2026-07-22 기준)

**완료된 마일스톤**: Milestone 0(철학 고정) ~ Milestone 6(AI Workflow & Resource Guide & Zero-Code Service Expansion)  
👉 **Mold MVP 100% 개발 완결 및 실증적 실험 백로그 수립 완료 (`TASKS.md`)**

---

## 내일의 첫 번째 실험 (First Experiment)

* **Task 1.1**: 외부 프로젝트 `drink-log`에서 Mold 패키지를 직접 불러와 부팅하는 첫 실험 진행
  * **목적**: Mold를 외부 프로젝트에서 불러올 때 발생하는 초기 마찰(Friction)을 관찰하고 기록하여 제거함.

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음.

---

## 다음 할 일 (Post-MVP)

1. `Phase 1 / Task 1.1`부터 순서대로 시작: `drink-log` 외부 프로젝트에서 Mold 패키지 임포트 및 마찰 제거
2. `TASKS.md`의 실험 ➔ 관찰 ➔ 마찰 제거 백로그 완료 조건에 따라 진행
