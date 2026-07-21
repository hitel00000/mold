# Mold IR (Intermediate Representation) 초안

> 이 문서는 Resource YAML이 로드 시점에 검증된 뒤 변환되는 단일 IR 구조를 정의한다.
> Storage / Transport / View는 오직 이 IR만 참조하며, 원본 YAML을 직접 다시 파싱하지 않는다.

---

## 1. 설계 전제

* IR은 **런타임 컴파일러의 산출물**이다. Resource YAML을 로드 → 검증 → IR로 변환하는 파이프라인은 부팅 시점(bootstrap) 또는 명시적 reload API 호출 시점에만 실행된다.
* IR은 append-only 철학을 따른다. 필드는 삭제되지 않고 `deprecated` 마킹만 된다.
* IR은 Go 구조체(strong type)로 메모리에 존재하며, 문자열 map이 아니다.

---

## 2. Top-level: Resource

```yaml
resource:
  name: Post              # PascalCase, 고유 식별자
  table: posts             # snake_case, 실제 SQLite 테이블명 (기본값: name의 snake_case + s)
  schema_version: 1        # 이 Resource IR이 몇 번째 append-only 버전인지
  timestamps: true         # created_at / updated_at 자동 추가 여부 (기본값: true)
  soft_delete: true        # true면 delete가 실제 DELETE가 아니라 deleted_at 마킹 (append-only 원칙과 일치)
```

append-only 정책상 `soft_delete: true`가 기본값이자 사실상 유일한 권장값이다. `false`는 프로토타입 단계에서 확실히 필요 없는 경우에만 escape hatch로 허용.

---

## 3. Field

```yaml
fields:
  - name: title
    type: string
    nullable: false
    default: null
    constraints:
      min_length: 1
      max_length: 200
    deprecated: false

  - name: body
    type: markdown
    nullable: false

  - name: view_count
    type: int
    nullable: false
    default: 0
    constraints:
      min: 0

  - name: legacy_slug        # append-only 예시: 더 이상 안 쓰지만 남겨둔 필드
    type: string
    nullable: true
    deprecated: true
    deprecated_since: 2
```

### 지원 Primitive Type (1차 후보)

| type       | SQLite 매핑     | 비고                          |
|------------|-----------------|-------------------------------|
| string     | TEXT             | 짧은 문자열, max_length 권장   |
| text       | TEXT             | 긴 문자열, 제약 없음            |
| markdown   | TEXT             | 저장은 text와 동일, View 렌더링만 다름 |
| int        | INTEGER          |                                |
| float      | REAL             |                                |
| bool       | INTEGER (0/1)    |                                |
| datetime   | TEXT (ISO8601)   |                                |
| enum       | TEXT + CHECK     | `constraints.values` 필요       |
| email      | TEXT + CHECK     | 포맷 검증용 semantic type       |
| url        | TEXT + CHECK     | 포맷 검증용 semantic type       |

> markdown/email/url처럼 "저장 타입은 같지만 검증·렌더링 방식이 다른" semantic type을 별도로 두는 이유: Resource 정의만 보고 View/Validation이 자동으로 달라져야 한다는 핵심 철학과 직결됨.

### Type별 허용 Constraints

| Primitive Type 그룹 | 허용되는 Constraints 키 | 비고 / 필수 여부 |
|----------------------|--------------------------|------------------|
| `string`, `text`, `markdown`, `email`, `url` | `min_length`, `max_length`, `pattern`, `unique` | 문자열 길이나 정규식 검증 |
| `int`, `float` | `min`, `max`, `unique` | 수치 범위 검증 |
| `enum` | `values` (필수), `unique` | `values` 미지정 시 검증 에러 |
| `bool`, `datetime` | `unique` | |

### Field-level 공통 속성

* `name`, `type`: 필수
* `nullable`: 기본값 `false`
* `default`: 생략 가능
* `constraints`: type별로 허용되는 키가 다름 (min/max, min_length/max_length, pattern, unique, values)
* `deprecated`, `deprecated_since`: append-only 필드 폐기용. `deprecated: true`인 필드는 CRUD API 응답/Form에서 제외되지만 컬럼은 유지됨.

---

## 4. Relation

Post-Comment를 최소 스트레스 테스트 케이스로 삼는다.

```yaml
relations:
  - name: comments
    kind: has_many          # has_many | belongs_to | has_and_belongs_to_many
    target: Comment
    foreign_key: post_id     # target 쪽에 생성되는 FK 컬럼
    on_delete: restrict       # restrict | soft_cascade  (append-only라 hard cascade는 없음)
```

```yaml
# Comment.yaml
resource:
  name: Comment
  timestamps: true
  soft_delete: true

fields:
  - name: body
    type: text
    nullable: false

relations:
  - name: post
    kind: belongs_to
    target: Post
    foreign_key: post_id
```

* `on_delete: soft_cascade` — 부모가 soft-delete되면 자식도 함께 soft-delete 마킹 (append-only 정책과 일관)
* N:M은 1차 스트레스 테스트에서는 제외하고, has_many/belongs_to만으로 Milestone 2~4를 완주한 뒤 추가 여부 결정 (마세라티 원칙)

---

## 5. Meta / Auth 연동 필드 (초안)

```yaml
auth:
  ownership_field: author_id   # 이 Resource의 row-level owner를 나타내는 필드 (nullable이면 공개 리소스)
  permissions:
    create: authenticated
    read: public
    update: owner
    delete: owner
```

* 최소 모델: `public | authenticated | owner | role:<name>` 4종만 1차 지원
* Field 단위 권한은 1차 스코프에서 제외 (마세라티 원칙 — 실제 필요해지면 추가)

---

## 6. Reload 트리거 (지난 논의 반영)

```
POST /_mold/reload
Authorization: (세션 쿠키, role: admin 필요)
```

* 파일 워처 대신 명시적 API로만 트리거 (결정성 확보)
* 요청 시 전체 Resource 디렉터리를 다시 로드 → 검증 → 새 IR 생성 → 검증 실패 시 **기존 IR 유지 + 에러 반환** (원자적 교체, 절대 부분 반영 없음)

---

## 7. 결정된 설계 사항

* [x] **Type별 Constraints 스키마 강제 규칙**  
  **결정**: Primitive Type 그룹별로 허용되는 constraint 키를 [validate.go](file:///C:/Users/jeongwoong/dev/mold/resource/validate.go)에 엄격하게 구현 및 명시함.  
  **근거**: 부적절한 제약조건(예: `string`에 `min/max`, `int`에 `min_length`)을 부팅/로드 검증 단계에서 명확한 에러로 차단하여 오염된 설정이 하위 레이어(Storage/View)로 전파되는 것을 예방함.

* [x] **View 렌더링 힌트의 IR 포함 여부**  
  **결정**: IR에는 View 렌더링 힌트를 포함하지 않으며, View 레이어가 `FieldType`만 보고 자체적으로 판단함.  
  **근거**: IR의 역할을 Resource 정의의 단일 소스 오브 트루스로 한정하고, IR 및 런타임 추상화의 단순함을 유지하기 위함.

* [x] **`schema_version` 관리 단위**  
  **결정**: `schema_version`은 Resource 단위로 관리함.  
  **근거**: Resource 파싱, 검증, 로드가 단일 파일(Resource) 단위로 원자적(Atomic) 처리되므로, 필드 단위 관리는 불필요한 추상화 복잡도를 가중시킴 (마세라티 원칙 적용).
