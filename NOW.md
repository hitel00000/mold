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
8. `docs/retrospectives/drink-log-production-cutover.md` — drink-log 실제 프로덕션 컷오버 및 실배포 회고
9. `docs/retrospectives/mold-native-migration.md` — drink-log Mold 네이티브 이관 회고 (Phase 9)
10. 이 문서(NOW.md)의 "다음 할 일" 섹션

---

## 현재 상태 (2026-08-19 기준)

**완료된 마일스톤**: Milestone 0~6 (MVP 100% 완결), Phase 1 종합 회고 완결 (`docs/retrospectives/phase1-retrospective.md`), Phase 2 `mold dev` DX 실험 완결, Phase 4 Cloudflare Workers TS+Hono+D1 Codegen & Plan 계층 실구현 완결, Phase 5 Task 5.1~5.5 완결, Phase 6 Task 6.1/6.2 완결, Task 8.1/8.2/8.4 완결, Phase 9 Mold 네이티브 이관 완결 및 **`drink-log` 실제 프로덕션 D1/R2 컷오버 및 실배포 100% 완결 (`https://drink-log.pages.dev`)**, **Task 11.1 `has_many` Eager Loading (`?include=`) 100% 완결**, **Task 11.2 Nested Writes (`관계형 중첩 쓰기` Option B) 100% 완결**, **Task 12.1 Cloudflare Workers TS 타깃 1-Step `multipart/form-data` (R2 Blob) 생성기 100% 완결**, **Task 12.2 `POST /_mold/batch` 폐기 (불필요한 미니 ORM화 방지 합의 완결)**, **Task 12.3 Mold 통합 CLI (`cmd/mold`) 및 `mold codegen` 서브커맨드 100% 완결**, **Task 12.4 선언적 `on_delete: cascade` 및 연관 Blob (R2/FS) 자동 청소 엔진 100% 완결**.  
👉 **현재 상태: Task 12.4 완결 및 drink-log 마이그레이션 가이드 (`pipe/mold-drinklog/2026-08-19-mold-to-drinklog-cascade-delete-guide.md`) 배포 완료**

---

## 핵심 원칙 및 확정 결정

- **실험 ➔ 관찰 ➔ 마찰 제거**: 미지의 문제를 사전에 상상해 미리 코드를 짜지 않고, 외부 적용 실험을 통해 발견된 마찰을 기록하고 해결하는 마세라티 원칙 적용.
- **Dumb Target**: IR은 Target에 독립적이며, Target은 비즈니스 해석 없이 주어진 명세를 이행함.
- **Invisible Infrastructure**: 개발자는 `generate`를 의식하지 않으며 소스 저장만으로 결과를 확인하는 DX를 다듬음 (`mold dev`로 가설 2 채택 완료).
- **Explicit Layering**: `resource.NormalizeFields()` (Layer 0 IR 원천) ➔ `plan.Build()` (Layer 1 Execution Plan) ➔ Target Packages (Layer 2) 3단계 단방향 계층 형성.
- **보고서 Diff 원칙 준수**: 보고서에 포함되는 모든 diff는 반드시 `git diff`, `git log -p`, `git show`의 raw stdout을 그대로 첨부하며, 수동 타이핑 및 편집을 영구 금지함 (`docs/retrospectives/has-many-include-diff-fabrication-incident.md`).

---

## 다음 할 일 (Phase 12 진행 목록)

1. **Task 12.5: 다중 Storage Adapter (PostgreSQL / MySQL) 실증**
   - `docs/philosophy.md` Adapter 우선 원칙에 입각하여, 동일한 Resource YAML이 PostgreSQL DDL 및 쿼리로 무결하게 컴파일되는지 실증.


