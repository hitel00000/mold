# NOW

> 이 문서는 새 세션(사람이든 AI든)이 프로젝트에 합류할 때 가장 먼저 읽는 문서다.
> 마일스톤이 끝날 때마다 갱신한다. 오래된 정보가 남아있으면 문서 전체의 신뢰도가
> 떨어지므로, 갱신을 미루지 말 것.

---

## 읽는 순서

새 세션은 아래 순서로 문서를 읽고 시작한다. 이 문서(NOW.md)만 읽고 코드를
바로 짜지 말 것 — 아래 문서들이 실제 계약이고, 이 문서는 그것들의 목차 역할만 한다.

1. `AGENTS.md` — 프로젝트 철학, 하지 않는 것, 설계 원칙
2. `docs/ir-spec.md` — Resource IR의 유일한 스펙 (구조체, 검증 규칙, 결정된 설계 사항)
3. `TASKS.md` — 마일스톤별 체크리스트
4. `docs/retrospectives/` 안의 가장 최근 회고 문서
5. 이 문서(NOW.md)의 "다음 할 일" 섹션

---

## 현재 상태 (2026-07-22 기준)

**완료된 마일스톤**: Milestone 0(철학 고정), Milestone 1(Resource IR), Milestone 2(Storage/SQLite Adapter), Milestone 3(Transport/REST API)

**진행 중인 마일스톤**: 없음 (Milestone 3 종료 후 휴지 상태)

**다음 시작점**: Milestone 4 — Default View (자동 관리 화면)

---

## 지금까지 확정된 핵심 결정

새 세션이 다시 논의하지 않아도 되는, 이미 결정된 사항들이다. 재논의가
필요하면 왜 필요한지 먼저 설명할 것.

- **언어**: Go. AI 에이전트가 작업하기 가장 편한 언어로 선택 (컴파일러 피드백 루프가 빠르고 명확함)
- **Transport**: HTTPS 고정, Storage: SQLite 고정 (다른 Adapter는 나중에 필요해지면 추가)
- **아키텍처**: Backend는 "런타임 컴파일러" 방식 — 부팅/reload 시점에 YAML → 검증 → IR(강타입 struct)로 한 번 컴파일하고, 이후 모든 레이어는 이 IR만 참조. 요청마다 재파싱하지 않음
- **REST API 라우팅**: 단일 Wildcard 동적 라우터(`/api/{table}`, `/api/{table}/{id}`)와 `atomic.Pointer[Registry]` 기반 스냅샷 스왑 구조 적용
- **Pagination**: limit/offset 방식 (기본 limit=20, max=100)
- **Migration**: destructive만 구현 (diff 기반 마이그레이션은 아직 안 만듦 — 실제 필요해지면 추가)
- **삭제 정책**: append-only + soft_delete 기본값. 실제 DELETE 대신 `deleted_at` 마킹
- **YAML 문법**: 정식 배열 형태 하나만 지원 (`fields: - name: ... type: ...`). 축약 형태는 명시적으로 거부됨
- **Auth 방향**: 세션 쿠키 기반으로 갈 예정 (아직 미구현, Milestone 5). 현재 API는 무인증 상태.
- **reload 트리거**: 파일 워처 대신 명시적 API (`POST /_mold/reload`, admin 세션 필요) — 결정성 확보 목적
- **프로젝트 포지셔닝**: 복잡한 프로덕션 서비스가 아니라, 빠른 프로토타이핑/작은 프로덕트용 도구

---

## 다음 할 일: Milestone 4 (Default View)

착수 전에 `docs/retrospectives/milestone-3.md`의 "알려진 제약 및 다음 마일스톤 적용 체크리스트" 항목들을 먼저 훑어볼 것.

Milestone 4 범위 (TASKS.md 기준): List View, Detail View, Create Form, Edit Form, Navigation. 완료 기준은 "브라우저에서 CRUD가 가능하다".

---

## 열려있는 질문 / 아직 미해결

- 없음 (Milestone 3까지 나온 질문은 전부 정리됨)

---

## 갱신 이력

- 2026-07-21: 최초 작성, Milestone 2 완료 시점 기준으로 작성
- 2026-07-22: Milestone 3 (Transport) 완료 반영 및 Milestone 4 (Default View) 전달사항 업데이트

