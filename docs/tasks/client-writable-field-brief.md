# 작업 지시서: Client-Writable 필드 차단 (`client_writable: false`) — 설계 브리핑

> 이 문서는 `AGENTS.md` 원칙 9("IR 구조체나 스펙 문서를 변경해야 한다고 판단되면,
> 코드를 먼저 짜지 말고 먼저 질문한다")에 따라 작성된 **설계 전용** 브리핑이다.
> 이 문서 자체는 구현 착수 문서가 아니며, 옵션 A/B/C를 사람이 검토·확정한 뒤
> 별도의 구현 작업 지시서(`docs/tasks/task-8-2-implementation-brief.md` 등)로
> 이어진다.
>
> 작업 착수 전 `NOW.md`의 "읽는 순서"에 따라 프로젝트 문서를 먼저 읽을 것.
> 특히 `TASKS.md` Phase 8 절, `docs/resource-guide.md` 패턴 7,
> `docs/retrospectives/phase7-field-level-auth-and-papercuts.md`를 반드시 확인할 것.

> [!IMPORTANT]
> **리뷰어 확정 사항 (2026-08-05)**: 아래 3절/4.2절/5절의 검토를 거쳐 사람이
> 다음을 확정하였다.
> - **옵션 A (Field-level `client_writable: false` IR 확장)** 채택.
> - **400 Bad Request 거부** (조용히 무시하지 않음) 채택.
> - **네이밍은 `client_writable` 그대로** 확정 (create/update 분리 등 추가
>   확장은 하지 않음).
> - **Task 7.1과의 관계**: Task 7.1의 기각 판정을 뒤집는 것이 아니라, 애초에
>   그때도 field-level 개념 자체는 타당하다고 봤으나 정량적 채택 기준(3건)에
>   못 미쳐 보류되었던 것이며, 이번 Task 8.2는 그 판정과 별개로 "위험 필드가
>   폼/payload에 노출되는 구조적 한계"라는 다른 질문을 다룬다 (6절 참고).
>
> 이하 본문은 이 결정에 이르기까지의 검토 과정을 그대로 남긴 기록이다.
> 후속 구현 작업 지시서는
> [`docs/tasks/task-8-2-implementation-brief.md`](task-8-2-implementation-brief.md)를
> 참고할 것.

---

## 1. 배경 (왜 이 작업을 하는가)

Task 7.1에서 "Field-level 권한 부재로 인한 privilege escalation" 문제를 조사한 결과,
`User.role` 필드는 이미 코어 엔진(`auth/permission.go` L48-L55)의
`ErrPrivilegeEscalation` 가드로 write 시점에 차단되고 있음이 실측으로 확인되었다
(`docs/retrospectives/phase7-field-level-auth-and-papercuts.md` 레슨 1 참고).
그러나 이 가드는 **"role 필드 값을 바꾸려는 시도를 403으로 거부"**하는 것이지,
**"애초에 role 필드를 폼에 노출하지 않거나 입력을 조용히 무시"**하는 것은 아니다.
즉 현재는 다음 두 가지 나쁜 선택지만 존재한다:

1. `permissions.create`를 `role:admin` 등으로 좁혀 기본 View 폼 자체를 닫는다
   → 일반 사용자는 회원가입을 위해 반드시 별도 glue 핸들러(`/signup`)를
   개발자가 미리 준비해둬야 하며, 기본 View만으로는 가입이 불가능하다.
2. `permissions.create: public`으로 열어 기본 View 폼을 그대로 쓴다
   → `role` input이 그대로 폼에 렌더링되고, `403`으로 막히긴 하지만
   위험한 필드가 사용자 눈에 노출된 채로 서비스가 운영된다.

Task 8.1(Ownership Field CREATE-time 자동 주입)이 완료되면서 `author_id`,
`owner_id` 같은 **소유권 필드**의 위조는 이미 원천 차단되었다. 남은 것은
`User.role`처럼 **소유권이 아니면서도 클라이언트가 절대 임의 지정해서는 안 되는
필드** 범주다. 이 문서는 이 범주를 위한 IR 확장 여부와 설계안을 다룬다.

---

## 2. 문제의 정확한 스코프

`client_writable: false`가 다루려는 것은 다음 세 가지뿐이며, 이미 해결된 문제를
다시 풀지 않는다.

* **다루지 않는 것 (이미 해결됨)**:
  - 소유권 필드(`ownership_field`) 위조 → Task 8.1에서 CREATE-time 자동 주입으로 해결.
  - `role` 필드 자체의 write 시도 차단 → `auth/permission.go`의 privilege escalation
    가드가 이미 403으로 차단 (Milestone 5부터 작동).
  - 비밀번호 응답 노출 → `password` 타입의 자동 sanitization으로 이미 해결
    (`docs/ir-spec.md` 5절, `docs/resource-guide.md` 3절).
* **다루는 것 (미해결)**:
  - 클라이언트가 위험한 필드(`role` 등)에 값을 아예 **보낼 수 없게** 만드는 것
    (거부가 아니라 원천 배제).
  - 기본 View의 Create/Edit 폼에서 해당 input을 **렌더링 자체를 하지 않는 것**.
  - 이를 통해 `permissions.create: public`을 열어도 기본 View만으로 안전하게
    회원가입 폼을 서빙할 수 있게 만드는 것 (glue 핸들러 의존도 감소).

---

## 3. 설계 옵션 A / B / C

### 옵션 A — Field-level `client_writable: false` IR 확장 (원안)

`resource/ir.go`의 `Field` 구조체에 `ClientWritable *bool` (또는 `bool` +
`omitempty` 없는 명시적 존재 여부 판단, 아래 5절 참고) 속성을 추가한다.

```yaml
fields:
  - name: role
    type: enum
    nullable: false
    default: "user"
    client_writable: false   # 신규 속성
    constraints:
      values: ["admin", "user"]
```

**동작 규칙 (제안)**:
1. REST API(`POST`/`PUT`)의 payload 파싱 단계에서 이 필드가 존재하면 제거
   (아래 4절 "조용히 무시 vs 거부" 참고).
2. 기본 View의 Create/Edit 폼 렌더링(`view/widget.go`)에서 이 필드에 대한
   input 요소를 생성하지 않는다.
3. `default` 값이 지정되어 있으면 DB 쓰기 시 그 값이 그대로 적용된다
   (기존 `default` 처리 경로를 그대로 탄다 — 새 경로 불필요).
4. Cloudflare TS Codegen Target(`codegen/cloudflare/generator.go`)의 생성 코드도
   동일 규칙을 적용한다 (payload 필드 제거 + 폼 input 미생성).

**장점**:
- 문제를 정확히 겨냥한 가장 직접적인 해법. `role` 같은 필드를 가진 모든
  Resource(향후 `is_verified`, `credit_balance` 등)에 재사용 가능한 일반 패턴.
- `permissions.create: public`을 열어도 기본 View 폼만으로 안전한 회원가입이
  가능해져, Task 8.1과 결합 시 glue 핸들러 의존성이 크게 줄어든다.

**단점 / 리스크**:
- `resource/ir.go`, `docs/ir-spec.md` 변경 — 코어 계약 확장이므로 AGENTS.md
  원칙 9의 승인 절차를 밟아야 한다 (이 문서가 그 절차).
- `docs/ir-spec.md` 1.5절의 **9개 타깃 수렴 지점**(Layer 0/1/2) 각각에
  필드 하나가 새로 추가되는 만큼, `plan.FieldPlan`에도 반영해야 완전한 패리티가
  보장된다 — 파편화 재발 위험 (Milestone 2 회고 "조건부 로직의 경계 누락" 패턴).
- Field-level 권한이라는 새로운 개념 축이 IR에 하나 더 생긴다 (마세라티 원칙
  ⑦ "아직 발생하지 않은 문제를 미리 해결하지 않는다"와 정면으로 검토가 필요한 지점).

---

### 옵션 B — Resource-level 관례: "쓰기 금지 필드는 항상 별도 Signup Resource로 분리"

IR을 전혀 확장하지 않고, `User`와 같이 위험 필드를 가진 Resource는 애초에
일반 CRUD 대상에서 완전히 분리한다. 예를 들어 `User`는 `permissions.create:
role:admin`으로 계속 잠가두고, 공개 가입은 `UserSignup`이라는 **별도의 얇은
Resource**(`email`, `password`, `name`만 가진)를 만들어 `create: public`으로
연다. 이 Resource에 대한 CREATE 성공 시, 애플리케이션 레이어(glue handler 또는
DB 트리거격 후처리)가 실제 `User` 레코드를 `role: "user"` 고정값으로 생성한다.

**장점**:
- `resource/ir.go` 변경 0줄. `docs/ir-spec.md` 5절 정합성 문제 자체가 발생하지
  않는다.
- 기존 `Post.yaml`처럼 "제네릭 API는 좁게 잠그고 glue 핸들러로 우회"하는
  Task 7.1의 이미 검증된 패턴을 그대로 재사용한다.

**단점**:
- Resource가 사실상 이중으로 존재하게 되어(`User` + `UserSignup`) Resource가
  "유일한 Source of Truth"라는 `docs/philosophy.md` ①과 미묘하게 어긋난다.
  같은 개념(사용자)에 대한 필드 정의가 두 곳에 흩어진다.
- glue 핸들러가 여전히 필요하다 — Task 8.1이 없애고자 했던 문제를 그대로
  안고 간다. `docs/getting-started.md` 5.1절의 회원가입 안내가 이 방식과
  본질적으로 동일하며, 이미 "글루 핸들러로 처리하라"고 문서화되어 있다.
- 이 옵션은 사실상 **"Task 8.2를 안 하는 것"**에 가깝다. Task 8.2가 풀고자
  하는 "기본 View 폼만으로 안전한 가입"이라는 목표를 달성하지 못한다.

---

### 옵션 C — Payload 화이트리스트 방식: `client_writable`을 IR에 명시하지 않고
`auth.permissions.create`에 필드 화이트리스트를 붙인다

```yaml
auth:
  permissions:
    create: public
    create_allowed_fields: [email, password, name]   # 명시된 필드만 payload에서 수용
```

**장점**:
- `Field` 구조체 자체는 건드리지 않는다 (필드 단위 속성이 아니라 권한 단위 속성).
- "이 액션에서 무엇이 허용되는가"가 `auth` 노드 한 곳에 응집되어, 옵션 A처럼
  필드마다 흩어지는 것보다 리뷰하기 쉬울 수 있다.

**단점**:
- 필드 목록을 화이트리스트로 유지하는 방식은 필드가 늘어날 때마다
  `create_allowed_fields`를 계속 갱신해야 하는 운영 부담이 있다 (블랙리스트인
  옵션 A는 위험 필드만 표시하면 되므로 반대로 훨씬 적은 수정으로 끝난다).
- View 폼 렌더링 시점에 이 화이트리스트를 참조해 input을 생략하려면 결국
  `view/widget.go`가 `Field`가 아니라 `auth.Permissions`를 함께 읽어야 하는데,
  이는 `docs/ir-spec.md` 1.5절이 정의한 "View는 `FieldType`만 보고 판단한다"
  원칙(`docs/ir-spec.md` 7절 "View 렌더링 힌트의 IR 포함 여부" 결정)과 충돌
  소지가 있다.
- `update`/`delete` 액션까지 고려하면 `create_allowed_fields` 하나로는 부족하고
  `update_allowed_fields`도 필요해져 개념이 배로 늘어난다. 옵션 A는 필드 하나에
  플래그 하나로 CRUD 전 액션에 일관되게 적용 가능해 더 단순하다.

---

## 4. 세부 트레이드오프 비교 (요청된 두 가지 검토 항목)

### 4.1 `password` 타입의 응답 자동 은폐 메커니즘과의 관계

`password` 타입은 **응답에서 숨기는(read 방향)** 메커니즘이고, `client_writable:
false`는 **입력을 막는(write 방향)** 메커니즘이다. 방향이 반대이므로 서로 대체할
수 없고 충돌하지도 않는다.

- `password`는 "클라이언트가 써도 되지만(비밀번호 설정은 사용자 본인이 함),
  다시 읽어서는 안 되는" 필드다.
- `client_writable: false`가 겨냥하는 `role`은 "클라이언트가 읽는 건 상관없지만
  (본인 role을 보는 것은 정상), 써서는 안 되는" 필드다.
- 두 속성은 동시에 걸릴 수도 있다 — 예컨대 "관리자가 발급한 API 키" 같은
  필드는 `password`(응답 은폐)와 `client_writable: false`(클라이언트 write 금지,
  서버 내부 로직만 생성)가 동시에 필요할 수 있다. 이는 옵션 A가 두 속성을
  **직교(orthogonal)** 하게 설계해야 함을 시사한다 — `client_writable`이
  `password` 타입 전용이 아니라 모든 타입에 독립적으로 적용 가능해야 한다.

### 4.2 "조용히 무시" vs "400 Bad Request 거부"

| 항목 | 조용히 무시 (필드 제거 후 진행) | 400 Bad Request 거부 |
| :--- | :--- | :--- |
| **동작** | payload에서 해당 필드를 파싱 단계에서 삭제하고 나머지로 정상 처리 계속 | 해당 필드가 조금이라도 존재하면 요청 전체를 즉시 거부 |
| **UX** | 클라이언트가 실수로 필드를 보내도(예: 폼 라이브러리가 자동으로 빈 `role: ""`을 실어 보내는 경우) 요청이 성공한다 | 사소한 실수에도 요청이 실패해 프론트엔드 개발 중 디버깅 마찰이 늘어날 수 있다 |
| **보안 시그널** | 공격 시도(`role: admin` 주입)와 무해한 실수를 구별하지 못하고 조용히 넘어간다 — 로그에 남지 않으면 탐지가 어려움 | 공격 시도를 명시적 에러로 드러내어 로그·모니터링에서 포착하기 쉽다 |
| **기존 패턴과의 정합성** | Milestone 3 회고의 "Deprecated 필드 응답 Sanitization"(`SanitizeRecord`)이 이미 응답 쪽에서 "조용히 제거" 방식을 채택하고 있어 대칭적이다 | `auth/permission.go`의 기존 `ErrPrivilegeEscalation`이 이미 "거부(403)" 방식이므로, 이것과 섞이면 같은 필드(`role`)에 대해 "생성 시엔 무시, 수정 시엔 거부"처럼 액션별로 다른 동작이 되어 사용자가 혼란스러울 수 있다 |
| **위험** | 클라이언트가 "왜 role이 반영 안 됐지"를 조용히 놓칠 수 있음 (silent failure — Milestone 2 회고의 "타입 안전성 구멍" 패턴과 유사한 종류의 위험) | 없음 (명시적 실패가 항상 조용한 성공보다 디버깅하기 쉽다는 게 이 프로젝트의 일관된 원칙, 참고: Milestone 2 회고 "타입 검증을 constraints보다 먼저") |

**확정 (리뷰어 승인)**: **거부(400 Bad Request)**로 확정한다. 근거:
1. 이 프로젝트는 지금까지 "silent failure보다 명시적 에러"를 반복적으로
   선택해왔다 (Milestone 2 회고, `docs/ir-spec.md` "unknown field" 400 거부
   규칙과 동일선상).
2. `client_writable: false`가 존재한다는 것 자체가 "이 필드는 절대 클라이언트가
   건드려서는 안 된다"는 강한 의도 표명이므로, 그 의도를 위반하는 요청은
   눈에 띄게 실패해야 공격 시도 탐지에 유리하다.
3. 다만 이 경우 **View의 기본 Create 폼은 애초에 그 input을 렌더링하지
   않으므로**, 정상적인 사용 흐름(기본 View를 통한 가입)에서는 거부가 발동될
   일이 없다 — 거부는 오직 REST API를 직접 두드리는 비정상/공격 경로에서만
   나타난다.

---

## 5. 9개 Target 영향성 전수 점검

`docs/ir-spec.md` 1.5절이 정의한 3단계 파이프라인(Layer 0/1/2) 기준으로,
Task 4.1에서 수렴된 9개 타깃 각각에 대한 영향을 점검한다.

| # | Target (파일 위치) | 현재 역할 | `client_writable: false` 반영 필요 여부 및 내용 |
| :-- | :--- | :--- | :--- |
| 1 | Cloudflare DDL 생성 (`codegen/cloudflare/generator.go`) | D1 `CREATE TABLE` DDL 생성 | **불필요**. 컬럼 자체는 여전히 존재해야 하므로 DDL은 변경 없음. |
| 2 | SQLite DDL 생성 (`adapters/sqlite/schema.go`) | SQLite `CREATE TABLE` DDL 생성 | **불필요**. 위와 동일. |
| 3 | Record Validation (`resource/record_validate.go`, `ValidateRecord`) | payload 필드 검증 | **필요**. 타입/제약 검증 이전 단계에서 `client_writable: false` 필드가 payload에 존재하면 제거(무시) 또는 거부(4절 참고) 처리 추가. Milestone 2 회고 원칙("타입 검증은 boundary에서 최우선")과 같은 자리에 위치해야 함. |
| 4 | Transport Sanitize (`transport/handler.go`, `SanitizeRecord`) | 응답 직전 `deprecated` 필드 제거 | **불필요** (읽기 방향이라 무관). 다만 대칭성 확인 차 재점검 필요 — `client_writable: false` 필드는 읽기는 허용되므로 `SanitizeRecord`가 이 필드를 지우지 않아야 함(현재도 안 지움, 회귀 방지 테스트만 추가). |
| 5 | View Widget/Form (`view/widget.go`, `BuildFormFields`) | Create/Edit 폼 input 생성 | **필요**. `client_writable: false` 필드는 Form 필드 순회 대상에서 배제. |
| 6 | View Handler (`view/handler.go`, `parseFormPayload`) | HTML Form POST 파싱 | **필요**. Target 3(Record Validation)과 동일한 무시/거부 규칙을 여기서도 적용 (multipart/form-urlencoded 파싱 단계). |
| 7 | Transport Multipart Parsing (`transport/handler.go`) | 1-Step Blob 업로드 시 form 필드 파싱 | **필요**. Blob 업로드 경로도 결국 레코드 생성이므로 Target 3과 동일 규칙 적용 필요. |
| 8 | Cloudflare TS Validation (`codegen/cloudflare/generator.go`, TS `validateRecord`) | Codegen Target의 payload 검증 | **필요**. Go Target 3과 동일 규칙을 TS 생성 코드에도 반영 (Go/TS 패리티, `docs/retrospectives/cloudflare-codegen-review.md`의 "커버리지 격차 점검" 체크리스트 적용 대상). |
| 9 | Cloudflare TS D1 Parameter Bind | 생성 레코드를 D1 INSERT/UPDATE 바인딩 | **간접 필요**. Target 8에서 이미 필드가 제거/거부되었다면 바인딩 단계는 자동으로 안전해짐 — 별도 로직 불필요하나 회귀 테스트로 확인. |

**요약**: 실질적으로 손대야 하는 지점은 **Target 3, 5, 6, 7, 8** 5곳이며,
이는 `plan.FieldPlan`에 `ClientWritable bool` 하나만 추가하면 `plan.Build()`가
이미 이 5개 타깃 전부에 공급하는 단일 수렴 지점이므로 (`docs/ir-spec.md` 1.5절
Layer 1), 각 타깃에 개별 분기를 흩뿌리지 않고 `plan` 패키지 한 곳의 확장으로
해결 가능하다. 이것이 옵션 A를 채택할 경우의 실제 구현 범위를 좁혀준다.

---

## 6. `docs/philosophy.md` ⑦ (마세라티 원칙) 및 `docs/ir-spec.md` 5절과의 정합성 판정

### 마세라티 원칙 관점
> "아직 발생하지 않은 문제를 미리 해결하지 않는다."

이 원칙에 비추어 Task 8.2가 선제적 추상화인지 검토가 필요하다.

- **Task 7.1 기각을 뒤집는 것이 아님을 먼저 명확히 한다.** Task 7.1의 최종
  판정은 "field-level 권한이라는 개념 자체가 틀렸다"는 것이 아니라, "실측으로
  재현된 위조 건수가 2건으로 채택 기준(3건 이상)에 미달했다"는 **정량적 문턱**
  때문이었다. 즉 판정 당시에도 field-level 개념 자체를 부정한 것은 아니었고,
  단지 그 시점에 수집된 마찰이 코어 IR을 확장할 만큼 충분히 반복되지 않았다는
  판단이었다.
- 이번 Task 8.2는 그 문턱을 다시 재는 것이 아니라, Task 7.1이 애초에 스코프
  밖으로 뒀던 **다른 질문**을 다룬다. Task 7.1은 "위조된 값이 write될 때 막을
  것인가"를 물었고(→ 소유권 필드는 Task 8.1로, `role`은 기존
  `ErrPrivilegeEscalation` 가드로 이미 해결되어 있다), Task 8.2는 "위험한
  필드가 애초에 폼/payload에 노출되어야 하는가, 그리고 그것 때문에 glue
  핸들러 없이는 기본 View가 근본적으로 안전할 수 없는 구조적 한계를 감수해야
  하는가"를 묻는다.
- `User.role`이라는 구체적 필드가 `docs/getting-started.md` 5절 튜토리얼과
  `drink-log-pilot`의 `User.yaml` 양쪽에서 반복 등장하는 것은, 이 구조적
  한계가 가상의 미래 문제가 아니라 **매 프로젝트마다 반복해서 마주치는
  패턴**이라는 뜻이다. 마세라티 원칙이 경계하는 것은 "아직 아무도 겪지 않은
  문제를 상상해서 미리 푸는 것"이지, "매번 똑같이 마주치는 구조적 한계를
  정식으로 인정하고 한 번에 해결하는 것"이 아니다.

### `docs/ir-spec.md` 5절 정합성
현재 5절은 **Row-level**(`ownership_field`, `permissions`) 권한만 다루며
"Field 단위 권한은 1차 스코프에서 제외 (마세라티 원칙 — 실제 필요해지면 추가)"라고
명시하고 있다. `client_writable: false`는 엄밀히 말해 "권한(누가 접근 가능한가)"이
아니라 "쓰기 채널 자체의 존재 여부(누구도 이 채널로는 쓸 수 없다)"에 가까워
5절이 우려하는 "Field-level ACL 매트릭스"(역할별로 필드마다 다른 권한을 부여하는
것)와는 다른, 훨씬 좁은 개념이다. 즉 옵션 A는 5절이 명시적으로 유보한
"role별 field 권한"을 만드는 것이 아니라, "모든 역할에 대해 동일하게 적용되는
단일 boolean 플래그"를 추가하는 것이므로 5절의 우려 범위 밖에 있다. **리뷰어가
이 판정에 동의하여 옵션 A 채택을 확정했다.**

---

## 7. 애매했던 지점 (검토 후 확정됨)

1. **명칭**: `client_writable: false`로 **확정**. `server_only`,
   `readonly_after_create` 등 액션별 분리 대안도 검토했으나, 현재 실증된
   요구(`role`)는 "생성/수정 모두에서 항상 서버가 값을 고정한다"는 단일
   요구뿐이므로 단일 플래그로 충분하다고 판단, 액션별 분리는 마세라티 원칙에
   따라 도입하지 않는다. 실제로 create/update를 다르게 다뤄야 하는 필드가
   나타나면 그때 확장한다.
2. **거부 vs 무시**: **400 Bad Request 거부로 확정**. `password` 타입의
   "값이 있으면 해싱, 없으면 건너뜀"과는 성격이 다른 필드(서버가 항상 값을
   결정하는 필드)이므로 관대한 처리를 따를 이유가 없고, 이 프로젝트의
   일관된 명시적 에러 원칙(Milestone 2 회고)을 따르는 것이 맞다고 확정했다.
3. **옵션 A와 B의 결합 가능성**: 별도 확정 없이 **열어둔다**. 우선 옵션 A만
   구현하고, 실제로 `User`보다 훨씬 민감한 필드가 다수인 Resource가 나타나면
   그때 옵션 B(별도 Signup Resource) 도입 여부를 재검토한다 (마세라티 원칙).

---

## 8. 다음 단계

1. ~~사람이 이 브리핑을 검토하고 최종 옵션을 확정한다.~~ **완료** — 상단
   "리뷰어 확정 사항" 참고.
2. `docs/tasks/task-8-2-implementation-brief.md`를 `docs/tasks/runtime-package-brief.md`와
   동일한 형식(스코프 / 반드시 할 것 / 절대 하지 말 것 / 설계 스펙 / 검증 조건 /
   작업 방식)으로 작성하여 구현 착수를 지시한다. **→ 별도 파일로 작성됨.**
3. 구현 완료 후 `docs/ir-spec.md` 5절 및 3절(Field-level 공통 속성)에
   `client_writable` 스펙을 반영하고, `docs/resource-guide.md`에 패턴 8로
   Good/Bad 예제를 추가한다.
