# Milestone 2 (Storage) 회고

> 이 문서는 Mold Milestone 2 (Storage 및 SQLite Adapter) 구현 과정에서 발생한 설계 문제와 피드백 패턴을 기록하고, 향후 마일스톤(Transport, Auth 등)에 적용할 지침을 정리한다.

---

## 1. 개요

Milestone 2에서는 특정 DB에 종속되지 않는 순수 `storage.Store` 인터페이스와 SQLite 어댑터(`adapters/sqlite`)를 구현하였다. Resource IR 기반의 `CREATE TABLE` DDL 자동 생성, `schema_version` 추적을 통한 Destructive 마이그레이션, 레코드 CRUD, 그리고 `Post`-`Comment` 연관 관계 통합 테스트를 성공적으로 완성하였다.

---

## 2. 발견된 문제 패턴 4가지

### 1) 검증 레이어의 책임 범위 혼동
- **무엇이 문제였는가**: `resource.Validate()`가 Resource 스키마 정의 자체만 검증함에도 불구하고, 런타임에 입력되는 데이터 레코드 값까지 검증해줄 것으로 착각하여 초기 CRUD 구현에서 레코드 데이터 값 검증이 누락되었음.
- **왜 놓치기 쉬웠는가**: 함수 이름이 단순 `Validate`여서 스키마 정의 검증과 데이터 레코드 검증이라는 서로 다른 책임 범위가 명확하게 분리되어 인식되지 않았음.
- **어떻게 고쳤는가**: `resource.ValidateRecord(r *Resource, record map[string]any, isUpdate bool)` 함수를 명시적으로 분리 신설하여 `Create` 및 `Update` 쓰기 수행 직전에 연결함.
- **다음 마일스톤에 적용할 원칙**: "이 검증 함수가 정의(Metadata/Schema)를 검증하는가, 실제 전달된 데이터 값(Payload)을 검증하는가"를 함수 이름 및 주석에 항상 명확히 명시할 것.

---

### 2) 타입 안전성 구멍
- **무엇이 문제였는가**: 레코드 검증 시 `min_length`, `pattern`, `min`/`max` 등의 제약조건(Constraints)은 체크했으나, 정작 전달된 값의 데이터 타입 자체(Go `string`, `int`, `float64`, `bool`, `datetime`)에 대한 검증이 빠져 있었음.
- **왜 놓치기 쉬웠는가**: `map[string]any`와 같은 동적 타입 맵 및 SQLite의 동적 타핑(Dynamic Typing) 특성상 타입 오류가 나더라도 에러 없이 조용히 통과(Silent Failure)되거나 나중에 엉뚱한 곳에서 버그가 터질 수 있었음.
- **어떻게 고쳤는가**: `ValidateRecord` 내부의 `validateFieldType` 단계를 Constraints 검증보다 최우선 순위로 배치하여, `int` 필드에 소수점이 있는 실수(`10.5`)나 문자열이 전달될 경우 사전에 명확하게 거부함.
- **다음 마일스톤에 적용할 원칙**: 동적 타입 데이터를 다루는 경계 지점(HTTP 파싱, REST API 바디 디코딩, DB 쓰기)에서는 **타입 검증(Type Check)을 제약조건 검증(Constraints Check)보다 반드시 먼저 수행**할 것.

---

### 3) 조건부 로직의 경계 누락
- **무엇이 문제였는가**: PK `id` 거부 로직이 `Create`에만 적용되고 `Update`에서 빠졌던 문제, 그리고 `soft_delete` 여부에 따른 쿼리 가딩(`deleted_at IS NULL`)이 일관되게 적용되었는지 점검이 필요한 문제 발생.
- **왜 놓치기 쉬웠는가**: "Create와 Update는 비슷하지만 다르다" 혹은 "SoftDelete 여부에 따른 branch가 여러 CRUD 함수에 흩어져 있다"는 이유로, 단일 함수 단위로만 생각하다가 전체 API 엔드포인트 세트에서의 일관성을 놓침.
- **어떻게 고쳤는가**: `id` 필드를 `Create` 및 `Update` 페이로드 전체에서 엄격히 사전 거부하도록 공통화하고, `Get`/`List`/`Update`/`SoftDelete` 전반에 걸쳐 `res.SoftDelete` 가드가 정상 적용됨을 E2E 테스트로 입증함.
- **다음 마일스톤에 적용할 원칙**: 하나의 조건(`soft_delete` 여부, `create/update` 구분, `auth/role` 권한 등)이 여러 함수나 API 엔드포인트에 영향을 준다면, **영향을 받는 대상 함수/엔드포인트 목록을 체크리스트로 나열하여 전수 점검**할 것.

---

### 4) 동시성/도메인 지식이 결합된 제약
- **무엇이 문제였는가**: `unique: true` 필드와 `soft_delete: true`가 결합될 때, 일반적인 DB `UNIQUE` 컬럼 제약은 SoftDelete 처리된 행이 UNIQUE 값을 계속 점유하여 동일한 값으로 재생성(`Create`)이 막히는 문제 발생.
- **왜 놓치기 쉬웠는가**: `unique` 속성과 `soft_delete` 속성을 각각 단독 기능으로만 생각하고, 두 속성이 조합되었을 때 발생할 런타임 비즈니스 엣지 케이스를 미리 머릿속으로 시뮬레이션하지 못함.
- **어떻게 고쳤는가**: `soft_delete: true`인 리소스는 DDL의 컬럼 레벨 `UNIQUE` 키워드를 빼고, `CREATE UNIQUE INDEX ... WHERE deleted_at IS NULL` 형태의 Partial Unique Index로 전환하여 반영함.
- **다음 마일스톤에 적용할 원칙**: 두 개 이상의 IR 속성(예: `permissions` + `ownership_field` + `soft_delete`)이 동시에 관여하는 지점은 각각 따로 구현하고 끝내지 말고, **조합되었을 때의 실제 런타임 동작을 별도 검토하고 전용 테스트로 검증**할 것.

---

## 3. 다음 마일스톤(Transport, Auth)에 적용할 체크리스트

Milestone 3(Transport / REST API) 및 Milestone 5(Auth / Authorization) 개발 시작 전 아래 질문을 스스로 검증한다:

1. **[타입 검증 우선]** HTTP 요청 바디(JSON)를 파싱할 때, 각 필드의 타입 검증(`FieldType` 일치 여부)이 범위/길이/패턴 등 제약조건 검증보다 먼저 실행되는가?
2. **[입력 필드 화이트리스트]** DTO/Payload에 Resource IR에 선언되지 않은 미정의 필드(`unknown field`)나 폐기된 필드(`deprecated: true`), 혹은 PK (`id`)가 포함되어 있을 때 명확한 400 Bad Request 에러로 거부하는가?
3. **[조건부 로직 전수 반영]** `soft_delete`, `permissions`, `ownership` 등 조건부 가딩 로직이 `List`, `Detail(Get)`, `Create`, `Update`, `Delete` 5개 REST API 엔드포인트 전체에 누락 없이 일관되게 반영되었는가?
4. **[속성 조합 엣지 케이스]** 인증/권한(`Auth`)과 연관관계(`Relation`)가 결합된 요청(예: `belongs_to` 타겟 Resource에 대한 소유권 및 접근 권한 검증)이 올바르게 중첩 처리되는가?
5. **[에러 메시지 명확성]** 검증 실패 시 DB 레벨의 불친절한 에러 대신, 클라이언트가 어떤 필드가 어떤 이유로 실패했는지 쉽게 알 수 있는 명확한 구조화 에러를 반환하는가?

---

## 4. 커밋 및 리뷰 사이클 요약

- **전체 커밋 수**: 총 13개 커밋
- **진행 과정**:
  1. **초기 5단계 구현** (5 커밋): Storage 인터페이스 정의, DDL 파서, destructive 마이그레이션, CRUD, Post-Comment 관계 테스트
  2. **1차 후속 수정** (2 커밋): 레코드 데이터 검증 `ValidateRecord` 도입 & `soft_delete: false` 조건부 쿼리 가딩
  3. **2차 후속 수정** (3 커밋): `ValidateRecord` 타입 검증 추가 & unknown/deprecated/PK 거부 & Partial Unique Index 전환
  4. **3차 후속 수정** (2 커밋): Update 페이로드 내 PK `id` 거부 조건 공통화 & `sqlite_master` 조회를 통한 Partial Index 검증 보강
  5. **문서화** (1 커밋): Milestone 2 회고 문서 추가
- **참고 사항**: 초기 구현 이후 피드백/리뷰를 통한 3차례의 명확한 후속 수정 사이클을 거쳤으며, 향후 마일스톤에서는 본 회고 체크리스트를 사전에 검토하여 후속 리뷰 수정 사이클을 1~2회 이내로 단축하는 것을 목표로 한다.
