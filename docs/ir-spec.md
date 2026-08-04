# Mold IR (Intermediate Representation) 초안

> 이 문서는 Resource YAML이 로드 시점에 검증된 뒤 변환되는 단일 IR 구조를 정의한다.
> Storage / Transport / View는 오직 이 IR만 참조하며, 원본 YAML을 직접 다시 파싱하지 않는다.

---

## 1. 설계 전제

* IR은 **런타임 컴파일러의 산출물**이다. Resource YAML을 로드 → 검증 → IR로 변환하는 파이프라인은 부팅 시점(bootstrap) 또는 명시적 reload API 호출 시점에만 실행된다.
* IR은 append-only 철학을 따른다. 필드는 삭제되지 않고 `deprecated` 마킹만 된다.
* IR은 Go 구조체(strong type)로 메모리에 존재하며, 문자열 map이 아니다.

---

## 1.5 실행 파이프라인 계층 (Resource → IR → Plan → Target)

Mold 런타임 및 정적 코드 생성기(Codegen)는 아래 3단계 단방향 파이프라인으로 동작하며, 9개 타깃(DDL/Validation/Sanitize/View Form/View Handler/Cloudflare TS 등)의 필드 해석이 이 계층을 통해 100% 수렴되어 있다.

```
resource.NormalizeFields()  (Layer 0: IR 원천 유틸 — FK 필드 파생 등 구조적 사실)
        ↓
plan.Build()                (Layer 1: 타깃 독립 정규화 Execution Plan)
        ↓
각 타깃 패키지               (Layer 2: adapters/sqlite, transport, view, codegen/cloudflare)
```

1. **Layer 0 (IR 원천 및 파생 유틸 - `resource.Resource` / `resource.NormalizeFields()`)**:
   - Resource IR의 순수 구조적 명세 및 기본 필드 파생(예: `belongs_to` 연관 관계에 따른 implicit FK 필드 팽창)을 담당한다.
   - 타깃 독립적이고 순수한 domain entity 계층으로, 상위 `plan` 패키지를 참조하지 않아 Go 언어 패키지 순환 참조(`import cycle`)를 차단한다.
2. **Layer 1 (Execution Plan - `plan.Build()`)**:
   - 단일 `*resource.Resource` IR을 입력받아 타깃 독립적인 실행 플랜(`*plan.Plan`)을 생성한다.
   - `res.NormalizeFields()`를 순회하며 각 필드의 `IsDerivedFK` 등의 속성을 계산하고 정규화된 `FieldPlan` / `RelationPlan` 목록을 구축한다.
   - 단일 리소스 스코프 1:1 보존으로 관계 대상 리소스 간 순환 참조 발생 가능성을 원천 차단한다.
3. **Layer 2 (Target Implementations - `adapters/sqlite`, `transport`, `view`, `codegen/cloudflare`)**:
   - 비즈니스 해석이나 추가 파생 로직 없이 `plan.Plan` (또는 필요시 `resource.NormalizeFields()`)만을 전달받아 주어진 명세를 직렬화 및 실행(DDL 생성, JSON Parsing, Form Rendering, TS Codegen 등)한다.

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

### 파생 FK 필드 정규화 (`resource.NormalizeFields()`) 및 Golden DDL Parity

* **암묵적 FK 필드 확장**: `relations` 노드에 `belongs_to` 관계(`rel.Kind == KindBelongsTo`)가 선언되고 외래키 이름(`rel.ForeignKey`)이 지정된 경우, `fields` 목록에 해당 FK 필드를 중복 기재하지 않더라도 `res.NormalizeFields()`를 통해 단일 `[]Field` 슬라이스로 자동 확장된다.
* **Golden DDL Parity (파생 FK의 `Nullable: true`)**: `NormalizeFields()`에 의해 자동 파생되는 FK 필드는 `Type: TypeInt`, `Nullable: true`로 기본 생성된다 (`Field{Name: rel.ForeignKey, Type: TypeInt, Nullable: true}`). 이는 DDL 생성 및 Record Validation 시 명시적 필드 미작성 FK 컬럼이 `"post_id" INTEGER` (NULL 허용)로 일관되게 처리되도록 보장하기 위한 골든 패리티 보정 규칙이다.
* **중복 방지 (Deduplication)**: YAML 작성자가 `fields`에 FK 컬럼(예: `post_id`)을 명시적으로 선언한 경우, `NormalizeFields()`는 `fieldMap`을 통해 이를 감지하고 중복 필드를 생성하지 않으며 명시적 필드 정의를 그대로 유지한다.

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

### Nullable Ownership 레코드 평가 규칙

`ownership_field`에 지정된 필드 값이 `NULL`인 레코드는 특정 소유자가 없는 공용/시스템 레코드로 취급된다. `permissions.read: owner`가 적용된 리소스에서 이런 레코드는 미인증 요청을 포함해 누구나 조회할 수 있다 (public과 동일하게 동작). 반면 `update`/`delete`에서 `owner`가 적용된 경우, 소유자가 없는 레코드는 `role: admin` 세션만 수정/삭제할 수 있으며 일반 인증 사용자는 403 Forbidden으로 거부된다. 이 규칙은 Go 런타임과 Cloudflare TS Codegen Target에 동일하게 적용된다.

### CREATE-time Ownership Field 자동 주입 규칙

`ownership_field`가 지정된 리소스(예: `ownership_field: author_id` 또는 `owner_id`)의 레코드 생성(CREATE) 요청 처리 시, 클라이언트가 payload에 실어 보낸 값은 무시되고 **항상 세션 사용자 ID(`sess.UserID` / `authUser.id`)로 서버가 자동 덮어쓰기**한다.
- **인증 사용자 요청 (`sess != nil`)**: 클라이언트가 payload에 타인의 ID를 포함하여 전송하더라도(예: `author_id: 999`), 서버가 항상 로그인한 세션 사용자 ID로 덮어써서 소유권 위조를 API/View/Codegen 전체 타깃에서 원천 차단한다.
- **미인증 요청 (`sess == nil`)**: 클라이언트가 payload에 보낸 소유권 필드 값은 제거(`NULL` / 삭제)된다. 필드가 `nullable: true`이면 DB에 `NULL` 레코드로 저장되며(공용/시스템 레코드 규칙과 일관), `nullable: false`이면 유효성 검사 실패(`400 VALIDATION_FAILED: field ... is required`)로 처리되어 미인증 사용자의 타인 소유권 명의 도용을 차단한다.
- **특수 예외 (`ownership_field: id`)**: `User.yaml`처럼 자기 자신의 레코드 소유권을 나타내어 `ownership_field`가 `id` (INTEGER Primary Key)로 지정된 특수 케이스는 PK 자동 발급 무결성을 유지하기 위해 세션 ID 덮어쓰기 대상에서 제외된다.

### List 액션의 Ownership 레코드 필터링 규칙

`permissions.read: owner`가 지정된 리소스의 List 액션 (`GET /api/{table}` 및 `/view/{table}`)은 DB 쿼리 레벨에서 레코드를 자동으로 필터링한다:
- **일반 인증 사용자 (non-admin)**: `(ownership_field = ? OR ownership_field IS NULL) AND deleted_at IS NULL` 조건이 주입되어 본인 소유 레코드와 공용/시스템 레코드(`NULL`)가 함께 반환되며, 페이지네이션 메타데이터(`meta.total`)도 해당 조건으로 정확히 집계된다.
- **어드민 사용자 (role: admin)**: 필터링 조건이 우회되어 전체 레코드가 반환된다 (Detail/Update/Delete의 admin bypass 원칙과 일관성 유지).
- **미인증 사용자**: `ownership_field`가 존재하는 경우 `ownership_field IS NULL AND deleted_at IS NULL` 조건으로 공용/시스템 레코드만 반환되며, `ownership_field`가 미지정된 리소스는 401 Unauthorized로 차단된다.
- **ownership_field 미지정 리소스**: 필터링 조건이 주입되지 않으며 기존 인증 여부 확인 후 전체 목록이 반환된다.

### 외부 OAuth 연동 및 세션 발급 전략 (예정된 확장 방향)

Mold는 특정 OAuth Provider(Google, GitHub 등)와 직접 통신하거나 프로토콜 구현체를 코어 런타임에 내장하지 않는다 (특정 벤더 종속성 회피 원칙).

* **외부 인증 처리 및 세션 발급 Escape Hatch (추가 예정)**: OAuth 토큰 교환 및 ID claim 검증은 애플리케이션 외부(예: `drink-log` 서버 코드)에서 처리하며, 검증이 끝난 후 Mold `auth`/`runtime`은 "이미 인증된 사용자 ID에 대해 세션 쿠키를 발급하는" 공개 API (가칭 `runtime.App.IssueSessionForUser` 또는 `auth.SessionManager` 레벨 동등 메서드)를 향후 별도 작업(Task 5.3)에서 제공할 예정이다.
* **Provider 식별 정보의 IR 표현**: 소셜 계정 연동 식별 필드(`provider`, `provider_user_id`)는 새로운 semantic type을 만들지 않고 일반 `string` 필드와 [5.6 복합 Unique Constraint](#56-복합-unique-constraint-초안) (`unique_together: [[provider, provider_user_id]]`) 조합으로 지원할 예정이다.
* **주의**: 이 절은 **향후 설계 및 확장 방향의 문서화**이며, 실제 세션 발급 API 메서드는 Task 5.3 구현 시점에 확정될 예정이다.

---

## 5.5 Blob Field (초안)

이미지/파일처럼 바이트 크기가 커서 SQLite 컬럼에 직접 넣기 부적절한 데이터를 위한
semantic type이다. 사케 앱(`docs/schema.sql`의 `sake_images.image_key`)에서 이미
암묵적으로 쓰이던 패턴 — "실제 바이트는 별도 Blob Storage에, DB에는 key만" — 을
IR 레벨로 끌어올린 것이다.

```yaml
fields:
  - name: image_key
    type: blob
    nullable: false
```

### SQLite 매핑

`blob` 타입은 `int`/`string`처럼 새로운 컬럼 종류를 만들지 않는다. DB 컬럼은
지금까지와 동일하게 `TEXT`이며, 저장되는 값은 실제 바이트가 아니라 Blob Storage
어댑터가 발급한 key(또는 URL)다.

| type   | SQLite 매핑 | 비고 |
|--------|-------------|------|
| `blob` | TEXT        | 값은 바이트가 아니라 BlobStore key. `constraints`는 미지원 (1차 스코프 제외) |

### Storage 경계

`blob` 필드가 있는 Resource라도 `storage.Store` 인터페이스(CRUD)는 지금과 동일하게
동작한다. 실제 바이트 업로드/다운로드/삭제는 이 인터페이스를 거치지 않고 별도
`storage.BlobStore` 인터페이스(가칭 `Put`/`Get`/`Delete`)를 통해서만 이뤄진다.

* `Store`(관계형 record CRUD)와 `BlobStore`(바이트 저장)는 서로 다른 책임이며,
  하나의 인터페이스로 합치지 않는다 (Milestone 2 회고 "검증 레이어의 책임 범위
  혼동" 패턴을 Storage 레이어에서 반복하지 않기 위함).
* 레코드 생성과 최초 Blob 업로드는 `POST /api/{table}`에 `multipart/form-data`를 전송하여 단일 `ActionCreate` 권한 평가로 원자 처리할 수 있다. ( create 권한만 있는 사용자도 레코드 생성과 함께 이미지를 즉시 업로드 가능).
* 기존 레코드의 Blob 교체(overwrite) 및 삭제는 별도 서브 엔드포인트(`POST /api/{table}/{id}/upload/{field}`, `DELETE /api/{table}/{id}/blob/{field}`)를 통해 이뤄지며, 각각 `ActionUpdate`, `ActionDelete` 권한 평가가 적용된다. (남의 레코드에 대한 권한 우회 구멍 방지).
* `POST /_mold/reload`는 스키마(컬럼, relation)만 원자적으로 교체하며, Blob
  Storage 쪽 상태를 건드리지 않는다. reload 실패 시 기존 IR이 보존되는 것과
  별개로, Blob Storage에는 애초에 reload가 손댈 대상이 없다.

### 다중 이미지 표현

레코드당 이미지 여러 장은 새로운 relation kind를 만들지 않고, 기존
`has_many`/`belongs_to`와 `blob` 타입 필드를 가진 별도 Resource 조합으로
표현한다 (예: `Post` `has_many` `PostImage`, `PostImage`가 `blob` 필드 보유).
N:M과 마찬가지로, 전용 storage kind는 실제 필요성이 확인되기 전까지 도입하지
않는다 (마세라티 원칙).

### 결정된 사항 (Task 1.2.5 확정)

* [x] **Blob Storage 어댑터 인터페이스 메서드 시그니처**  
  `storage.BlobStore` 인터페이스로 정의함:  
  - `Put(ctx context.Context, key string, data io.Reader, size int64, contentType string) error`
  - `Get(ctx context.Context, key string) (io.ReadCloser, string, error)` (바이트 스트림 및 Content-Type 반환)
  - `Delete(ctx context.Context, key string) error`
* [x] **Key 발급 규칙**  
  고유성이 보장되는(collision-free) Resource-scoped 계층형 경로인 `blobs/{table}/{record_id}/{field_name}_{timestamp_or_uuid}{ext}` 패턴을 채택함 (예: `blobs/drink_images/1/image_key_17847849.jpg`). 1-Step 멀티파트 생성(`POST /api/{table}`)과 2-Step 덮어쓰기 모두 리터럴 경로(예: `"new"`) 없이 실제 생성된 `{record_id}` 스코프 규칙을 동일하게 엄격히 준수함.
* [x] **`auth.permissions` 및 엔드포인트 권한 적용**  
  별도 가드 코드 신설 없이 기존 Mold의 `auth.Evaluate` 엔진을 100% 동일하게 활용함.  
  - 1-Step 최초 생성 (`POST /api/{table}` multipart): 대상 레코드 생성에 대한 `ActionCreate` 권한 평가.
  - 2-Step 덮어쓰기 (`POST /api/{table}/{id}/upload/{field}`): 대상 레코드에 대한 `ActionUpdate` 권한 평가 (overwrite 전용).
  - 조회 (`GET /api/{table}/{id}/blob/{field}`): 대상 레코드에 대한 `ActionRead` 권한 평가.
  - 삭제 (`DELETE /api/{table}/{id}/blob/{field}`): 대상 레코드에 대한 `ActionDelete` 권한 평가.
* [x] **1-Step 생성 실패 시 원자적 롤백 메커니즘 및 알려진 제약**  
  - 1-Step 생성 중 Blob 파일 저장(`BlobStore.Put`) 또는 DB 업데이트 실패 시, 미완결 트랜잭션을 되돌리기 위해 DB 레코드를 물리적 hard delete(`DELETE FROM table WHERE id = ?`)로 롤백함.
  - 이 hard delete는 공개 삭제 정책(append-only + soft_delete)을 바꾸는 것이 아니며, 단일 요청 내 미완결 생성 트랜잭션 취소 전용 헬퍼임. (마세라티 원칙에 따라 롤백 중 동시 조회 클라이언트에 수 ms 간 레코드가 관찰될 수 있음).
  - 1-step create의 원자적 롤백은 Store 어댑터가 `HardDeletePhysically`(내부 hard delete 능력)를 제공할 때만 보장되며, 미지원 어댑터에서의 `SoftDelete` 조용한 폴백은 무결성 회귀 방지를 위해 엄격히 금지함. 미지원 어댑터이거나 FK 제약 등으로 롤백 DELETE가 실패하면 `500 BLOB_STORE_FAILED_RECORD_PRESERVED` 에러 코드로 레코드 보존 사실을 명시적으로 안내함.
  - **알려진 제약 (다중 Blob 필드 실패 시 R2 Orphan 객체 남음)**: 현 5.5절의 1-Step 원자적 롤백 명세는 단일 blob 필드를 전제로 작성됨. 하나의 리소스에 2개 이상의 blob 필드가 존재할 때, 첫 번째 blob 업로드는 성공했으나 두 번째 blob 업로드가 실패하는 경우 DB 레코드는 물리적 hard delete로 정상 롤백되지만, 이미 Storage/R2에 저장된 첫 번째 blob 객체는 삭제되지 않고 orphan 상태로 남게 됨. 마세라티 원칙에 따라 현재 단계에서 별도의 트랜잭션 보상(compensating deletion) 복잡성을 도입하지 않고 알려진 제약으로 남기며, 향후 다중 blob 리소스가 프로덕션에 실제 등장했을 때 해결 대상으로 추적함.
  - **[해결됨 - Cloudflare R2 Target 동기 보상 삭제 적용]**: `codegen/cloudflare` (Cloudflare Workers TS+D1+R2 Target)에서 1-Step Multipart Create 도중 N번째 blob 업로드가 실패할 경우, 요청 스코프 내에서 이미 R2에 성공적으로 저장된 이전 blob 키 목록(`uploadedBlobKeys`)을 추적하여 동기 보상 삭제(Compensating Deletion: `c.env.BUCKET.delete(key)`)를 수행함. 이 때, 클라이언트 GET 요청 시 깨진 이미지 링크(dangling reference)가 노출되는 것을 원천 차단하기 위해 **D1 레코드 hard delete를 먼저 실행하여 404 NOT_FOUND 상태를 확보한 후 R2 보상 삭제를 수행**함. 보상 삭제 중 하나 이상의 객체 삭제가 실패하는 경우, 조용히 삼키지 않고 `BLOB_ORPHAN_CLEANUP_FAILED` (HTTP 500) 에러 코드와 함께 정리되지 못한 `orphan_keys` 목록 및 `d1_rollback_failed` 여부를 명시적 응답 바디로 반환함.
  - **새로운 알려진 제약 (보상 삭제 1회 시도 및 비동기 스위퍼 미도입)**: R2 보상 삭제 시도는 네트워크 transient failure에 대비한 백오프 재시도(retry with backoff)나 비동기 스위퍼(sweep queue)를 도입하지 않고 현재 요청 스코프 내 1회 시도 후 실패 시 즉시 `BLOB_ORPHAN_CLEANUP_FAILED`로 명시적 보고함. 마세라티 원칙에 따라 실제 프로덕션에서 R2 보상 삭제 재시도가 자주 요구되는 상황이 검증되었을 때 비동기 스위퍼 도입을 검토함.

---

## 5.6 복합 Unique Constraint (초안)

N:M 관계를 표현하는 Join Resource(예: `RecordTag`의 `sake_record_id` + `tag_id`) 또는 소셜 로그인 계정 식별(예: `UserIdentity`의 `provider` + `provider_user_id`)처럼 **두 개 이상의 필드 조합에 대한 유일성(Uniqueness)**을 강제하기 위한 스펙이다.

### Resource YAML 스펙

`constraints.unique_together`는 단일 필드 하위가 아니라 Resource 최상위 노드 레벨에 정의하며, 복합 유일성을 보장할 필드명 배열의 리스트를 가진다.

```yaml
resource:
  name: RecordTag
  table: record_tags
  soft_delete: true

fields:
  - name: sake_record_id
    type: int
  - name: tag_id
    type: int

constraints:
  unique_together:
    - [sake_record_id, tag_id]   # 두 컬럼 조합의 중복 생성을 차단
```

### `soft_delete: true` 결합 및 Partial Unique Index 전환 스펙

* **Partial Unique Index 전환 규칙**: 단일 필드 `unique: true`와 동일하게, `soft_delete: true` 리소스에서 `unique_together`가 정의된 경우 DDL 생성 시 컬럼 레벨 `UNIQUE` 제약 대신 `deleted_at IS NULL` 조건을 가진 Partial Unique Index로 자동 전환된다.
* **SQLite DDL 산출 스펙 (예시)**:
  ```sql
  CREATE UNIQUE INDEX "idx_record_tags_unique_sake_record_id_tag_id"
  ON "record_tags" ("sake_record_id", "tag_id")
  WHERE "deleted_at" IS NULL;
  ```
* **효과**: soft-delete 마킹된 레코드(`deleted_at IS NOT NULL`)와의 무결성 충돌 없이 동일한 연결/조합을 다시 생성(re-create)할 수 있도록 보장한다.

---

## 5.7 관계 조인 조회 (`?include=`) 스펙 (Task 5.2 마찰 B)

N:M Join Resource(예: `RecordTag`) 조회 시 클라이언트가 각 연결 레코드의 대상 리소스(`Tag` 등) 상세 정보를 확인하기 위해 N+1번의 API 요청을 보내는 마찰(Friction B)을 해소하기 위한 스펙이다. REST API(`GET /api/{table}`, `GET /api/{table}/{id}`)와 SSR HTML View(`GET /view/{table}`, `GET /view/{table}/{id}`)에서 단일 요청으로 연관 대상 리소스를 동시 조인 조회하여 내포(embed) 응답할 수 있다.

### 1. 스코프 제약 및 파라미터 검증
* **`belongs_to` 관계 전용**: `?include=` query 파라미터에는 대상 리소스의 `belongs_to` 연관 관계명만 쉼표(`,`) 구분 리스트로 지정할 수 있다 (예: `?include=tag` 또는 `?include=tag,sake_record`).
* **strict 파라미터 거부 (`INVALID_INCLUDE`)**: 존재하지 않는 관계명이거나 `has_many` 관계명이 `?include=`에 지정될 경우, 조용히 무시하지 않고 즉시 **HTTP 400 Bad Request** (`code: INVALID_INCLUDE`, message: `"invalid relation '...' for include"`) 에러를 반환한다.

### 2. N+1 배치 쿼리 최적화 (`WHERE id IN (...)`)
* 메인 레코드 목록 조회 후, 지정된 연관 리소스별로 외래 키(FK) 값들을 고유 집합으로 수집(deduplicate)한다.
* 연관 리소스 Storage 어댑터의 `List(ctx, res, storage.Query{IDs: fkVals})` 배치 쿼리를 실행하여 단 1회의 `WHERE "id" IN (?, ?, ...)` 쿼리로 대상 레코드들을 일괄 가져온다. N개의 메인 레코드에 대해 1회의 배치 쿼리만 발생하므로 N+1 쿼리를 완전히 방지한다.

### 3. 권한 평가 및 식별 불가 `null` 안전 보장 (보안 메트릭스)
연관 대상 레코드가 개별적으로 조회되어 메인 응답에 포함(embed)될 때, 각 연관 레코드 객체는 아래 4가지 시나리오에 따라 안전하게 처리된다:

| 시나리오 | 연관 대상 상태 | 반환 형태 | 비고 |
|---------|----------------|-----------|------|
| (a) 읽기 허용 | `ActionRead` 권한 통과 (`auth.Evaluate` OK) | `tag: { id: 1, name: "..." }` | Sanitize 적용된 객체 내포 |
| (b) 읽기 거부 | `ActionRead` 권한 거부 (Unauthorized/Forbidden) | `tag: null` | 정보 유출 차단 |
| (c) FK 미설정 | 메인 레코드의 FK 값이 `NULL` | `tag: null` | 정상적인 null 관계 |
| (d) Soft-Deleted | 연관 대상 레코드가 삭제됨 (`deleted_at != null`) | `tag: null` | Soft-deleted 숨김 |

* **식별 불가능성(Indistinguishability) 원칙**: 권한 거부(b), FK NULL(c), Soft-deleted(d) 3가지 경우 모두 메인 응답의 JSON/Map 상에서 동일하게 `"relation_name": null`로 직렬화되며 구조적/에러상 어떠한 차이점도 노출하지 않는다. 이를 통해 열거 공격(enumeration attack)이나 권한 미달 레코드의 존재 유무 피싱을 원천 차단한다.
* **응답 정제 (Sanitization)**: 내포되는 연관 객체는 메인 레코드와 동일하게 `SanitizeRecord` (비밀번호 필드 제거 등)가 강제 적용된 후 포함된다.

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
  **결정**: Primitive Type 그룹별로 허용되는 constraint 키를 [validate.go](../resource/validate.go)에 엄격하게 구현 및 명시함.  
  **근거**: 부적절한 제약조건(예: `string`에 `min/max`, `int`에 `min_length`)을 부팅/로드 검증 단계에서 명확한 에러로 차단하여 오염된 설정이 하위 레이어(Storage/View)로 전파되는 것을 예방함.

* [x] **View 렌더링 힌트의 IR 포함 여부**  
  **결정**: IR에는 View 렌더링 힌트를 포함하지 않으며, View 레이어가 `FieldType`만 보고 자체적으로 판단함.  
  **근거**: IR의 역할을 Resource 정의의 단일 소스 오브 트루스로 한정하고, IR 및 런타임 추상화의 단순함을 유지하기 위함.

* [x] **`schema_version` 관리 단위**  
  **결정**: `schema_version`은 Resource 단위로 관리함.  
  **근거**: Resource 파싱, 검증, 로드가 단일 파일(Resource) 단위로 원자적(Atomic) 처리되므로, 필드 단위 관리는 불필요한 추상화 복잡도를 가중시킴 (마세라티 원칙 적용).

* [x] **Plan 계층 (`plan` 패키지) 도입 및 3단계 파이프라인 수렴**  
  **결정**: `resource.NormalizeFields()` (Layer 0) ➔ `plan.Build()` (Layer 1) ➔ Target Packages (Layer 2) 3단계 단방향 계층 구조를 채택하고, 9개 타깃(SQLite DDL, Transport Sanitize/Multipart, View Form/Widget, Record Validation, Cloudflare Codegen DDL/Validation/Bind)의 필드 루프 및 FK 파생 로직을 단일 수렴 지점으로 이관 완료함.  
  **근거**: Target 패키지마다 반복되던 `switch f.Type` 및 `KindBelongsTo` 루프 파편화를 원천 차단하고, `resource` ➔ `plan` ➔ `resource` 순환 참조(`import cycle`) 오류 포착 시 `NormalizeFields()`를 Layer 0 IR 원천 유틸로 승격시켜 단방향 컴파일 계층을 완성함.

* [x] **복합 Unique Constraint (`constraints.unique_together`) 스펙 채택**  
  **결정**: N:M 관계를 전용 relation kind 대신 명시적 Join Resource로 표현하고 외부 OAuth 사용자 식별을 지원하기 위해, Resource 최상위 노드에 `constraints.unique_together` 문법 스펙을 채택함.  
  **근거**: `has_and_belongs_to_many` 같은 전용 relation kind를 런타임에 추가하는 것은 런타임 개념 복잡도를 가중시킴. 명시적 Join Resource 패턴과 복합 unique 제약 조합으로 N:M과 OAuth 식별 요구사항을 기존 `has_many`/`belongs_to` 구조 위에서 단순하게 충족할 수 있음 (마세라티 원칙 및 단일 소스 오브 트루스 유지).
