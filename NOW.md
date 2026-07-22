# NOW

> 이 문서는 새 세션(사람이든 AI든)이 프로젝트에 합류할 때 가장 먼저 읽는 목차 문서입니다.
> 주요 마일스톤이나 문서 체계 변경 시 갱신합니다.

---

## 읽는 순서

새 세션은 아래 순서로 문서를 읽고 시작합니다. 이 문서(NOW.md)만 읽고 코드를 바로 짜지 마십시오.

1. `README.md` — 프로젝트 소개, 핵심 개념 및 구동 예시
2. `docs/philosophy.md` — 존재 이유, 핵심 철학 및 비타협적 원칙
3. `AGENTS.md` — 프로젝트 철학, 하지 않는 것, AI 작업 가이드라인
4. `docs/ir-spec.md` — Resource IR의 유일한 스펙 (구조체, 검증 규칙)
5. `docs/resource-guide.md` — Resource YAML 작성 스펙 및 Good/Bad 패턴 가이드
6. `TASKS.md` — MVP 완료 상태, 검증해야 할 가설(Hypotheses) 및 Post-MVP 로드맵
7. `docs/retrospectives/` 안의 가장 최근 회고 문서
8. 이 문서(NOW.md)의 "다음 할 일" 섹션

---

## 현재 상태 (2026-07-22 기준)

**완료된 마일스톤**: Milestone 0(철학 고정) ~ Milestone 6(AI Workflow & Resource Guide & Zero-Code Service Expansion)  
👉 **Mold MVP 100% 개발 완결 및 최소화된 지속 가능 문서 체계 정립 완료 (`README.md`, `docs/philosophy.md`, `TASKS.md`)**

**다음 시작점 (Post-MVP)**:
1. 별도 예제 프로젝트(`Drink Log`)에서 Mold 실증 및 수직 패턴 발굴
2. `mold dev` 중심의 투명한 개발자 경험(DX, `Ctrl + S` 핫컴파일) 정비
3. CLI 구조 정비 및 Target/Plan 가설 검증
4. Cloudflare Workers Static Target Generator 가설 검증

---

## 핵심 원칙 및 확정 결정

- **문서 최소화 원칙**: 중복 문서 방지 및 문서-코드 드리프트 최소화를 위해 `README.md`, `docs/philosophy.md`, `TASKS.md` 최소 구조를 유지함.
- **가설 분리**: Feature, Plan, Pipeline, Multi-Target 등 미검증 아이디어는 `ARCHITECTURE.md` 대신 `TASKS.md` 내 `검증해야 할 가설(Hypotheses)` 섹션에서 관리함.
- **Smart Spec, Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행하는 가장 멍청한 형태(Dumb Target)를 지향함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 `mold dev` 하나로 `Ctrl + S` 저장을 거쳐 결과를 즉시 확인함.

---

## 다음 할 일 (Post-MVP)

1. `Drink Log` 예제 프로젝트를 구축하며 Mold의 유용성 및 반복 패턴 발굴
2. `TASKS.md`의 Post-MVP 목록 순서에 따라 작업 진행
