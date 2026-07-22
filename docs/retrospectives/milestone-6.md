# Milestone 6 (AI Workflow) 및 MVP 전체 종합 회고

> 이 문서는 Mold Milestone 6 (AI Workflow)의 성공적 완결과 MVP (Milestone 1~6) 전체 개발 과정을 돌아보는 종합 회고 기록이다.

---

## 1. 개요

Milestone 6은 Mold MVP의 마지막 마일스톤으로, 백엔드 Go 코드를 단 한 줄도 수정하지 않고 **AI 에이전트가 Resource YAML 파일만을 작성/수정하여 전체 서비스(Database Schema, REST API, Auth/Permissions, HTML View)를 안전하고 원자적으로 확장할 수 있는지**를 검증하고 문서화하는 마일스톤이다.

---

## 2. Milestone 6 주요 성과 및 검증 결과

1. **`docs/resource-guide.md` 작성 및 Good/Bad 예제 대조 체계화**:
   - AI 에이전트가 Resource 작성 시 실수하기 쉬운 5대 엣지 케이스(권한 상승/노출, `password` 타입 불일치 제약, `ownership_field` 오타, 타겟 리소스 미존재, `enum` constraint 누락)를 Good/Bad 예제 쌍으로 정리하여 가이드화함.
2. **3대 검증 프로젝트 (`blog`, `todo`, `crm`) YAML 작성 및 원자적 로드 검증**:
   - `examples/blog/` (`User`, `Post`, `Comment`)
   - `examples/todo/` (`User`, `Project`, `Task`)
   - `examples/crm/` (`Company`, `Contact`, `Customer`)  
   3개 프로젝트 세트를 구축하고 `resource.LoadAll` 통합 테스트(`resource/examples_test.go`)로 원자적 검증을 성공함.
3. **잘못된 YAML 로드 차단 및 기존 IR preservation 스트레스 테스트**:
   - 잘못된 `permissions`, `ownership_field`, `password` constraint, 미존재 target relation 등에 대해 로드 시점(`resource.Validate`)에서 400 에러로 즉시 차단함.
   - `POST /_mold/reload` 실패 시 **기존 런타임 IR이 100% 원자적으로 보존되어 `GET /api/posts` 등 기존 API가 200 OK로 계속 정상 동작함**을 E2E 스트레스 테스트 (`transport/reload_atomic_stress_test.go`)로 검증 완료함.
4. **MVP 성공 기준 완주 및 view 템플릿 격리 버그 수정**:
   - `Post` YAML 하나로부터 처음부터 끝까지 (Storage -> CRUD -> REST API -> Default HTML View -> Auth/Session -> AI Reload) 전 과정을 커버하는 통합 테스트 (`cmd/mvp_e2e_test.go`) 완주.
   - E2E 검증 중 발견된 `view` 레이어의 템플릿 상속 오버라이딩 버그(단일 템플릿 트리가 `formTemplate`으로 덮어씌워지는 현상)를 `template.Clone()` 방식으로 완벽하게 수정·격리함.

---

## 3. MVP 전체 종합 회고 (Milestone 1 ~ Milestone 6)

### (1) 통틀어 가장 많이 반복되었던 5대 문제 패턴

| 구분 | 문제 패턴 내용 | 원인 분석 | 런타임 해결 방안 |
| :--- | :--- | :--- | :--- |
| **1) 검증 책임 혼동** | 정의(Schema) 검증 vs 런타임 레코드 데이터 검증 혼동 | `Validate()` 함수명 하나의 모호성 | `Validate()` (Schema)와 `ValidateRecord()` (Payload)로 함수 및 시점 명확 분리 |
| **2) 타입 검증 순서** | 제약조건(Constraints) 체크를 타입(Type Check) 체크보다 먼저 수행 | `map[string]any` 동적 타입 맵의 Silent Failure 특성 | 경계 지점에서 `validateFieldType`을 최우선 순위로 강제 |
| **3) 조건부 로직 누락** | Create/Update, SoftDelete, Authority 가드가 특정 엔드포인트에서 누락 | 단일 함수 단위 구현 사고의 한계 | CRUD 5개 엔드포인트 전체에 영향받는 가드 목록 전수 체크리스트화 |
| **4) IR 속성 조합 케이스** | `soft_delete` + `unique`, `permissions` + `ownership_field` + `password` | 단일 속성 단위 사고로 인한 조합 엣지 케이스 간섭 | Partial Unique Index 및 `auth.Can`/`Evaluate` 단일 통합 엔진 구축 |
| **5) 다중 Resource View 템플릿 오염** | 단일 Resource 테스트에서는 숨어있다가 다중 Resource 통합 E2E에서 드러난 템플릿 오버라이딩 | 단일 `tmpl` 트리에 `define "content"`가 중복 파싱되어 마지막 템플릿이 전체를 덮어씀 | `baseLayout.Clone()`으로 뷰 타입별(`list`, `detail`, `login`, `form`) 독립 템플릿 트리 격리 |

---

### (2) 프로젝트의 최고 자산: 회고 → 체크리스트 → 실측 검증 워크플로우

Mold 프로젝트 개발을 이끈 것은 단순한 기능 코드가 아니라 **"회고 -> 다음 마일스톤 체크리스트 -> 코드 구현 -> 실측 검증"으로 이어지는 엄격한 리뷰 워크플로우**였다.

1. **"구현했다"와 "실제로 확인했다"의 완벽한 구분**:
   - "이 코드가 작성되어 있으니 동작할 것이다"라는 주관적 추측을 철저히 배제하고, 반드시 HTTP/DB 수준의 실제 테스트 코드를 동반하여 통과 여부를 검증함.
2. **리뷰 사이클의 극적인 축소 효과**:
   - Milestone 2 초기에는 피드백 및 후속 수정 사이클이 3차례 이상 반복되었으나, Milestone 3부터 5에 이르기까지 이전 회고의 체크리스트를 착수 프롬프트와 개발 과정에 직접 매핑함으로써 후속 수정 발생 빈도를 1차례 이내로 대폭 줄임.
3. **append-only 히스토리 문화**:
   - 기존 커밋을 함부로 amend/rebase로 숨기지 않고, 문제 발견 및 수정 과정을 append-only 커밋으로 남김으로써 AI와 사람 모두가 과거의 선택 근거를 명확히 추적할 수 있게 함.

---

## 4. Post-MVP 향후 Mold 확장 방향

마세라티 원칙("아직 발생하지 않은 문제를 미리 해결하지 않는다")에 따라 MVP 단계에서 의도적으로 미뤄두었던 과제들은 향후 실제 필요성이 제기될 때 다음 순서로 확장한다:

1. **Storage Adapter 확장**: PostgreSQL, MySQL Adapter 추가 (Resource IR은 변경 없이 유지)
2. **Migration 전략 진화**: Destructive 마이그레이션 외에 Schema Diff 기반의 Non-destructive Migration 지원
3. **View UI/UX 확장**: `belongs_to` FK 필드의 직접 ID 숫자 입력 방식에서 대상 Resource Title/Name `<select>` Dropdown 방식으로 확장
4. **N:M Relation 지원**: `has_and_belongs_to_many` 관계 및 중간 조인 테이블 자동 마이그레이션 지원

---

**결론**: Mold는 "Resource가 유일한 Source of Truth"라는 원칙을 지키며, AI와 사람이 함께 온라인 서비스를 최소 노력으로 안정적으로 구축할 수 있음을 성공적으로 증명하였다.
