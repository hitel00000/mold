# Mold Resource 작성 가이드 (Resource Guide)

> 이 문서는 Mold의 유일한 Source of Truth인 **Resource YAML 작성법과 작성 규칙**에 관한 실전 종합 가이드이다.  
> AI 에이전트 및 개발자는 이 가이드를 준수하여 Resource YAML을 작성하고 서비스 기능을 확장해야 한다.

---

## 1. 개요 및 설계 철학

Mold에서는 **Resource가 유일한 Source of Truth**이다.  
YAML로 Resource를 정의하고 명시적 API(`POST /_mold/reload`)를 호출하면, **Go 코드를 전혀 건드리지 않고(Zero Go Code)** 다음이 런타임에 원자적으로 생성 및 자동 반영된다:

- SQLite Database Schema (`CREATE TABLE`, Partial Unique Index 등)
- Record Validation & Type Check
- CRUD REST API
- Session Authentication & Permission Authorization
- HTML Default CRUD View

---

## 2. Resource YAML 구조 스펙

Resource YAML 파일은 아래 4개 최상위 노드로 구성된다.

```yaml
resource:
  name: Post              # PascalCase, 고유 식별자 (필수)
  table: posts             # snake_case, DB 테이블명 (선택, 기본값: name의 snake_case + s)
  schema_version: 1        # append-only 버전 번호 (기본값: 1)
  timestamps: true         # created_at, updated_at 자동 관리 여부 (기본값: true)
  soft_delete: true        # deleted_at 마킹 삭제 정책 (기본값: true)

fields:
  - name: title
    type: string
    nullable: false
    constraints:
      min_length: 1
      max_length: 200

relations:
  - name: comments
    kind: has_many
    target: Comment
    foreign_key: post_id
    on_delete: soft_cascade

auth:
  ownership_field: author_id
  permissions:
    create: authenticated
    read: public
    update: owner
    delete: owner
```

---

## 3. Supported Field Types & Constraints

Mold는 11가지 Primitive / Semantic Type을 지원한다.

| Primitive Type | SQLite 매핑 | 허용되는 Constraints | 비고 / 주의사항 |
| :--- | :--- | :--- | :--- |
| `string` | `TEXT` | `min_length`, `max_length`, `pattern`, `unique` | 짧은 문자열 |
| `text` | `TEXT` | `min_length`, `max_length`, `pattern`, `unique` | 긴 본문 문자열 |
| `markdown` | `TEXT` | `min_length`, `max_length`, `pattern`, `unique` | View 렌더링 시 XSS Sanitization 적용 |
| `int` | `INTEGER` | `min`, `max`, `unique` | 정수 수치 |
| `float` | `REAL` | `min`, `max`, `unique` | 실수 수치 |
| `bool` | `INTEGER` | `unique` | `0` 또는 `1` |
| `datetime` | `TEXT` | `unique` | ISO8601 포맷 문자열 |
| `enum` | `TEXT` | `values` **(필수)**, `unique` | `values: ["draft", "published"]` 필수 지정 |
| `email` | `TEXT` | `min_length`, `max_length`, `pattern`, `unique` | 이메일 포맷 검증 |
| `url` | `TEXT` | `min_length`, `max_length`, `pattern`, `unique` | URL 포맷 검증 |
| `password` | `TEXT` | `min_length`, `max_length`, `pattern` | **bcrypt 자동 해싱**, 응답 sanitization, **`unique` 및 `values` 사용 불가** |

> [!CAUTION]
> **Constraint 규칙 준수**: Type에 맞지 않는 Constraint(예: `string`에 `min/max`, `int`에 `min_length`, `password`에 `unique`)가 들어올 경우 로드 시점 검증(`resource.Validate`)에서 즉시 400 에러로 거부된다.

---

## 4. Relations (연관 관계)

관계는 `has_many`와 `belongs_to` 조합을 기본으로 작성한다.

- `name`: 관계 식별 이름 (필수)
- `kind`: `has_many` | `belongs_to` (필수)
- `target`: 대상 Resource의 PascalCase 이름 (필수, registry에 존재하는 Resource여야 함)
- `foreign_key`: 타겟 또는 대상 테이블의 FK 컬럼명 (필수, 예: `post_id`)
- `on_delete`: `restrict` | `soft_cascade` (선택, 부모 삭제 시 자식 처리)

---

## 5. Auth & Permissions (인증 및 권한)

Row-level 접근 제어는 `auth` 노드에서 정의한다.

- `ownership_field`: 레코드 소유권 User ID를 담는 필드명 (예: `author_id`, `user_id`). **반드시 `fields` 목록 또는 시스템 필드로 실제로 존재하는 이름이어야 함.**
- `permissions`: `create`, `read`, `update`, `delete` 각 CRUD 액션별 권한 지정.
  - `public`: 누구나 접근 가능
  - `authenticated`: 로그인 세션 사용자 전체
  - `owner`: 세션 사용자 ID == 레코드 `ownership_field` 값 또는 `role: admin` 사용자
  - `role:<name>`: 세션 사용자의 `role`이 지정한 `<name>`과 일치하거나 `admin`인 경우

---

## 6. Reload API (`POST /_mold/reload`)

Resource YAML을 추가/수정한 후 프로세스 재시작 없이 반영하려면 아래 API를 호출한다:

```http
POST /_mold/reload
Authorization: Cookie (admin 세션 쿠키 필요)
```

**원자적 보장 (Atomic Guaranty)**:
- YAML 파싱 및 검증, 관계 검증 중 **단 하나의 오류라도 발생 시 기존 런타임 IR이 100% 그대로 유지**되며, 에러 응답(400 Bad Request)을 반환한다.
- 검증 성공 시에만 `atomic.Pointer[Registry]` 스왑 및 DB DDL 마이그레이션이 수행된다.

---

## 7. Good vs Bad YAML 패턴 대조 가이드 (AI 명심 사항)

AI 에이전트는 Resource YAML을 작성/수정할 때 아래의 **잘못된 패턴(Bad)**을 절대 생성해서는 안 되며, 반드시 **올바른 패턴(Good)**을 따라야 한다.

### 패턴 1: 민감/소유권 리소스의 Write 권한 오설정 (`permissions.update: public`)

> **위험성**: `update`나 `delete` 권한을 `public`으로 둘 경우 비인증 사용자나 다른 사용자가 다른 사람의 데이터를 무단 수정/삭제할 수 있는 심각한 보안 구멍이 발생함.

| ❌ Bad (잘못된 설정) | ✅ Good (올바른 설정) |
| :--- | :--- |
| ```yaml<br>resource:<br>  name: Article<br><br>fields:<br>  - name: title<br>    type: string<br>  - name: author_id<br>    type: int<br><br>auth:<br>  ownership_field: author_id<br>  permissions:<br>    create: authenticated<br>    read: public<br>    update: public   # ❌ 위험! 타인이 글 수정 가능<br>    delete: public   # ❌ 위험! 타인이 글 삭제 가능<br>``` | ```yaml<br>resource:<br>  name: Article<br><br>fields:<br>  - name: title<br>    type: string<br>  - name: author_id<br>    type: int<br><br>auth:<br>  ownership_field: author_id<br>  permissions:<br>    create: authenticated<br>    read: public<br>    update: owner    # ✅ 작성자만 수정 가능<br>    delete: owner    # ✅ 작성자만 삭제 가능<br>``` |

---

### 패턴 2: `password` 타입 필드에 안 맞는 Constraint 적용 (`unique: true`)

> **위험성**: `password` 타입은 Mold 런타임에서 자동으로 bcrypt 해시(60자) 문자열로 변환하여 DB에 저장함. 해시된 비밀번호 컬럼에 `unique: true` 제약을 거는 것은 보안 및 로직상 부적절하며 로드 시점 거부 대상임.

| ❌ Bad (잘못된 설정) | ✅ Good (올바른 설정) |
| :--- | :--- |
| ```yaml<br>fields:<br>  - name: password<br>    type: password<br>    constraints:<br>      min_length: 8<br>      unique: true    # ❌ 비밀번호에 unique 제약 사용 불가!<br>``` | ```yaml<br>fields:<br>  - name: password<br>    type: password<br>    constraints:<br>      min_length: 8   # ✅ 평문 최소 길이만 지정<br>``` |

---

### 패턴 3: `ownership_field` 오타 및 미존재 필드 지정

> **위험성**: `auth.ownership_field`에 지정된 필드가 `fields` 목록에 존재하지 않을 경우, 소유권(`owner`) 권한 평가 시 레코드 값을 찾지 못해 권한 검증이 실패하거나 의도치 않게 거부됨.

| ❌ Bad (잘못된 설정) | ✅ Good (올바른 설정) |
| :--- | :--- |
| ```yaml<br>resource:<br>  name: Document<br><br>fields:<br>  - name: user_id     # 실제 필드는 user_id<br>    type: int<br><br>auth:<br>  ownership_field: author_id  # ❌ 오타! author_id라는 필드는 존재하지 않음<br>  permissions:<br>    update: owner<br>``` | ```yaml<br>resource:<br>  name: Document<br><br>fields:<br>  - name: user_id     # 실제 필드명<br>    type: int<br><br>auth:<br>  ownership_field: user_id    # ✅ fields의 user_id와 정확히 일치<br>  permissions:<br>    update: owner<br>``` |

---

### 패턴 4: `relations.target`에 존재하지 않는 Resource 지정

> **위험성**: `target`에 존재하지 않는 Resource명을 쓰면 관계 무결성이 깨지며 로드 시점(`ValidateTargetResources`)에 400 에러로 즉시 교체가 거부됨.

| ❌ Bad (잘못된 설정) | ✅ Good (올바른 설정) |
| :--- | :--- |
| ```yaml<br>relations:<br>  - name: author<br>    kind: belongs_to<br>    target: UserProfile  # ❌ UserProfile이라는 Resource가 registry에 없음<br>    foreign_key: user_id<br>``` | ```yaml<br>relations:<br>  - name: author<br>    kind: belongs_to<br>    target: User         # ✅ 등록된 User Resource와 매핑<br>    foreign_key: user_id<br>``` |

---

### 패턴 5: `enum` 타입에 `values` Constraint 누락

> **위험성**: `enum` 타입은 허용할 값 목록(`values`)이 반드시 필요함. 누락 시 로드 검증 단계에서 명확히 차단됨.

| ❌ Bad (잘못된 설정) | ✅ Good (올바른 설정) |
| :--- | :--- |
| ```yaml<br>fields:<br>  - name: status<br>    type: enum       # ❌ values 미지정으로 로드 실패<br>``` | ```yaml<br>fields:<br>  - name: status<br>    type: enum<br>    constraints:<br>      values: ["draft", "published", "archived"]  # ✅ 필수 값 목록 지정<br>``` |

---
