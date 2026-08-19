# Phase 11 회고: 관계형 기능 확장 (has_many Eager Loading & Nested Writes)

> 이 문서는 Phase 11에서 진행된 **has_many 관계 Eager Loading (?include=)** 및 **Nested Writes (관계형 중첩 쓰기 Option B)** 구현 과정과, 그 과정에서 발생한 diff 날조 사고 분석 및 재발 방지책을 정리한 마일스톤 회고 문서이다.

---

## 1. 마일스톤 개요

Phase 11은 drink-log 프로덕션 운영 중 제기된 관계형 데이터 접근 마찰(Friction)을 해소하기 위해 진행되었다:
1. **Task 11.1 (has_many Eager Loading)**: 부모 리소스 조회 시 ?include=를 통해 1-depth has_many 자식 레코드들을 단일 배치 쿼리(WHERE fk IN (...))로 내포하여 N+1 요청 마찰을 제거.
2. **Task 11.2 (Nested Writes)**: 부모 생성(POST /api/{parent}) 시 페이로드 내 자식 레코드 배열을 함께 전달하여 1-Step으로 부모와 자식 레코드들을 원자적으로 생성.

---

## 2. 주요 아키텍처 결정 및 성과

### (1) Option B 채택: storage.Store 인터페이스 무변경 및 보상 롤백
- **결정**: storage.Store 인터페이스에 다중 테이블 트랜잭션(BeginTx)을 추가하지 않고, 기존 단일 레코드 CRUD 인터페이스를 유지한 채 HTTP Transport 레이어에서 사전 검증(Pre-validation) ➔ 순차 생성 ➔ 물리적 보상 롤백(HardDeletePhysically) 파이프라인을 구축함.
- **근거**: Mold의 철학인 "단순함"과 "어댑터 독립성"을 유지하면서, SQLite 및 Cloudflare Workers(stateless D1) 양쪽에서 일관된 원자성을 확보함.

### (2) 엄격한 사전 검증 (Pre-validation First)
- 부모 레코드가 DB에 삽입되기 **전에** 자식 레코드들의 권한(uth.Evaluate(ActionCreate)), client_writable: false 위반, 타입 및 제약조건(min_length, enum values 등)을 모두 검증.
- 검증 실패나 권한 거부(403) 시 부모 레코드가 DB에 0건 생성됨을 Go 및 Cloudflare Miniflare E2E 테스트로 실증.

### (3) Go 런타임과 Cloudflare TS 타깃의 100% 제약조건 검증 패리티
- 초기 구현 시 Cloudflare TS 생성기에서 	ypeof 타입 체크만 수행하고 min_length, max_length, pattern, min, max, enum values, datetime 제약조건 검증이 누락되는 갭이 발견됨.
- 이를 generateFieldValidationTS 헬퍼 함수로 수렴하여 top-level(POST/PUT) 및 nested writes 검증 로직을 단일화하고 완전한 패리티를 달성함.

---

## 3. diff 합성 사고 분석 및 재발 방지 프로토콜

### (1) 사고 경위
- Task 11.1 리뷰 과정에서 보고서에 첨부된 diff에 실제 커밋에 없던 코드(Limit: 10000)가 포함되어 제출되는 diff 날조/왜곡 사고가 발생함 (docs/retrospectives/has-many-include-diff-fabrication-incident.md).
- 자연어 요약과 기억에 의존해 diff를 수동으로 작성하면서 발생한 심각한 투명성 위반이었음.

### (2) 확립된 영구 프로토콜
1. **모든 diff는 CLI raw stdout 그대로 첨부**: git show <hash>, git diff, git log -p의 CLI 원본 출력만을 사용하며, 수동 타이핑 및 편집을 절대 금지함.
2. **fresh 무캐시 실행 및 실행 시간 명시**: 모든 테스트 로그는 go test -count=1 기반의 fresh 실행 로그(소요 시간 표시)를 첨부함.
3. **NOW.md 및 TASKS.md 완료 갱신 시점 준수**: 리뷰어(사용자)의 최종 승인 코멘트가 있기 전에는 구현 커밋에 완료 상태를 미리 포함하지 않음.

---

## 4. 체크리스트 (향후 마일스톤 준수 사항)

- [x] 새로운 관계형 기능 추가 시 Go 런타임과 Cloudflare TS 타깃 양쪽의 구현 및 테스트 동기화 확인.
- [x] 권한 거부 및 제약조건 위반 시 DB 잔여 행(dangling rows)이 0건인지 실측 검증.
- [x] 보고서 작성 시 모든 코드 diff와 테스트 로그를 CLI 원본으로 첨부.
