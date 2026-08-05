# 작업 지시서: `client_writable: false` 필드 속성 구현

> 이 문서는 소스 코드에 접근 가능한 구현 에이전트에게 전달하기 위한 작업 브리핑이다.
> 작업 착수 전 `NOW.md`의 "읽는 순서"에 따라 프로젝트 문서를 먼저 읽을 것.
> 특히 `docs/tasks/client-writable-field-brief.md`(이 작업의 설계 승인 문서 —
> 옵션 A / 400 거부 / `client_writable` 네이밍이 이미 사람 승인으로 확정됨)와
> `docs/ir-spec.md` 1.5절(Layer 0/1/2 파이프라인)을 반드시 확인할 것.

---

## 1. 배경 (왜 이 작업을 하는가)

`docs/tasks/client-writable-field-brief.md`에서 옵션 A(Field-level
`client_writable: false` IR 확장)가 채택 확정되었다. `User.role`처럼 서버만
값을 결정해야 하는 필드가 REST payload와 기본 View 폼에 그대로 노출되는
구조적 한계를 해소하기 위함이다. 이 작업은 그 설계를 실제로 구현한다.

**이미 해결되어 이 작업 범위가 아닌 것** (중복 작업 방지):
- 소유권 필드(`ownership_field`) CREATE-time 자동 주입 → Task 8.1에서 완료.
- `role` 필드 write 시도 자체의 403 차단 → `auth/permission.go`의
  `ErrPrivilegeEscalation` 가드가 이미 처리 중.
- 비밀번호 응답 은폐 → `password` 타입의 기존 sanitization이 처리 중.

---

## 2. 스코프

### 반드시 할 것
- `resource/ir.go`의 `Field` 구조체에 `ClientWritable bool` (기본값 `true`,
  YAML에 `client_writable: false`가 명시된 경우에만 `false`) 속성 추가.
- `plan/plan.go`의 `FieldPlan`에 동일 속성을 편입시켜 단일 수렴 지점으로 공급
  (브리핑 문서 5절에서 식별된 5개 타깃이 전부 이 플랜을 통해 값을 받도록).
- 아래 5개 지점에 `client_writable: false` 필드가 payload/form에 존재할 경우
  **HTTP 400 Bad Request**로 거부하는 로직 추가 (브리핑 4.2절 확정 사항):
  1. `resource/record_validate.go` (`ValidateRecord`) — REST JSON payload.
  2. `view/handler.go` (`parseFormPayload`) — HTML Form POST.
  3. `transport/handler.go` — 1-Step Multipart Blob 업로드 시 form 필드.
  4. `codegen/cloudflare/generator.go`의 TS `validateRecord` 생성 코드.
  5. (확인용) `view/widget.go`(`BuildFormFields`)에서 `client_writable: false`
     필드의 Create/Edit input을 렌더링하지 않도록 배제.
- 에러 코드는 기존 관례(`INVALID_INPUT` 등)와 일관되게: 예)
  `{"error":{"code":"CLIENT_WRITE_FORBIDDEN","message":"field 'role' is not
  client-writable"}}` — 정확한 에러 코드 명칭은 `transport/handler.go`의
  기존 에러 코드 목록과 충돌하지 않는지 확인 후 결정.
- `default` 값이 지정된 `client_writable: false` 필드는 CREATE 시 해당
  default가 정상적으로 적용되는지 확인 (기존 default 처리 경로를 그대로
  타는지 검증만 하면 됨 — 새 경로 불필요).
- `transport/handler.go`의 `SanitizeRecord`가 `client_writable: false` 필드를
  **응답에서는 지우지 않는지** 회귀 테스트로 확인 (읽기는 허용되어야 함,
  브리핑 4.1절 참고).

### 절대 하지 말 것
- `resource/ir.go`, `docs/ir-spec.md`의 기존 구조체를 이 작업 범위 밖으로
  변경하지 말 것 — 이번 확장은 `Field`에 필드 하나 추가로 한정.
- create/update 액션별로 분리된 속성(`client_writable_create`,
  `client_writable_update` 등)을 만들지 말 것 — 브리핑 7절에서 마세라티
  원칙에 따라 명시적으로 보류됨.
- "조용히 무시(silent ignore)" 경로를 만들지 말 것 — 브리핑 4.2절에서
  400 거부로 확정됨.
- 옵션 B(별도 Signup Resource) 관련 코드를 만들지 말 것 — 이번 스코프 아님.
- `password` 타입의 기존 해싱/sanitization 로직을 수정하지 말 것 (직교하는
  별개 메커니즘, 브리핑 4.1절 참고).

---

## 3. 설계 스펙

### 3.1 Resource YAML 문법

```yaml
fields:
  - name: role
    type: enum
    nullable: false
    default: "user"
    client_writable: false     # 신규 — 생략 시 기본값 true
    constraints:
      values: ["admin", "user"]
```

### 3.2 `resource/ir.go`

```go
type Field struct {
    // ...기존 필드...
    ClientWritable bool // 기본값 true. false면 CREATE/UPDATE payload에서 거부되고
                         // 기본 View 폼에서 input이 렌더링되지 않는다.
}
```

- YAML 로더(`resource/loader.go`)에서 `client_writable` 키가 명시되지
  않은 경우 반드시 `true`로 채워야 한다 (Go의 `bool` zero value가 `false`이므로
  로더에서 명시적으로 처리하지 않으면 모든 필드가 기본적으로 쓰기 금지되는
  치명적 회귀가 발생한다 — **이 지점을 최우선으로 테스트할 것**).

### 3.3 `plan/plan.go`

```go
type FieldPlan struct {
    // ...기존 필드...
    ClientWritable bool
}
```

`plan.Build()`가 `res.NormalizeFields()`를 순회하며 `FieldPlan.ClientWritable`을
그대로 복사. 파생 FK 필드(`belongs_to` 자동 확장분)는 `ClientWritable: true`가
기본이며, 이번 작업에서 FK 필드에 대한 특수 처리는 하지 않는다 (필요성이
확인되지 않았으므로 마세라티 원칙에 따라 범위 제외).

### 3.4 검증 로직 (5개 지점 공통 규칙)

각 지점에서 payload/form에 `client_writable: false`인 필드명이 키로
존재하면 (값이 `null`이든 실제 값이든 관계없이 **키 자체의 존재**를 기준으로):

1. HTTP 400 Bad Request 반환.
2. 에러 메시지에 필드명을 명시 (`"field 'role' is not client-writable"`).
3. 요청 전체를 중단 — 다른 필드도 함께 무시하고 부분 처리하지 않는다.

> [!NOTE]
> "키 자체의 존재"를 기준으로 삼는 이유: `{"role": null}`처럼 명시적으로
> null을 보내는 것도 "이 필드를 건드리려는 시도"이므로 조용히 넘어가면 안
> 된다. 브리핑 4.2절의 "공격 시도와 무해한 실수를 구별하지 못하는 위험"을
> 피하기 위함.

### 3.5 View Form 배제 (`view/widget.go`)

`BuildFormFields`가 `plan.Plan.Fields`를 순회할 때 `ClientWritable == false`인
필드는 Create/Edit 폼 input 목록에서 제외한다. List/Detail 화면에는 계속
표시되어야 한다 (읽기는 허용 — 3.4절과 대칭).

---

## 4. 검증 조건 (완료 기준)

- [ ] `resource/ir.go`, `resource/loader.go`, `resource/validate.go` 유닛
      테스트: `client_writable` 생략 시 기본값 `true`, 명시적 `false` 파싱,
      YAML round-trip.
- [ ] `resource/record_validate.go`의 `ValidateRecord` 유닛 테스트:
      `client_writable: false` 필드가 payload에 있으면 400 거부, 없으면
      정상 처리 + `default` 값 적용 확인.
- [ ] REST API E2E (`httptest`): `POST /api/users`에 `role: admin` 포함 시
      400 (`CLIENT_WRITE_FORBIDDEN` 등), `role` 필드를 아예 보내지 않은
      정상 가입 시 201 + `role: "user"`(default) 확인.
- [ ] `GET /api/users/:id` 응답에 `role`이 정상적으로 포함되는지 확인
      (읽기는 막히지 않음 — sanitization 회귀 테스트).
- [ ] `view/widget.go` 유닛 테스트: `client_writable: false` 필드가
      Create/Edit 폼 HTML에 input으로 렌더링되지 않는지 확인.
- [ ] `view/handler.go` E2E: HTML Form POST로 `role=admin`을 강제로 실어
      보내도(개발자 도구로 hidden input 조작 시뮬레이션) 400 거부되는지 확인.
- [ ] Cloudflare TS Codegen 생성 코드 유닛 테스트 + Miniflare 실측:
      Go 런타임과 동일한 400 거부 패리티 확인 (`docs/retrospectives/cloudflare-codegen-review.md`의
      "커버리지 격차 점검" 체크리스트에 따라 raw HTTP 로그 첨부).
- [ ] 기존 `runtime/privilege_escalation_test.go`, `examples/quickstart/with-auth`
      E2E, `examples/drink-log-pilot` 전체 테스트가 회귀 없이 통과.
- [ ] `go test ./...` 및 `go build ./...` 전체 통과.
- [ ] **로더 기본값 회귀 테스트 최우선**: `client_writable` 키가 없는 기존
      모든 Resource YAML(`examples/quickstart`, `examples/drink-log-pilot`,
      `examples/blog`, `examples/todo`, `examples/crm` 등)을 로드했을 때
      모든 필드의 `ClientWritable`이 `true`로 채워지는지 전수 확인 — 이
      항목이 실패하면 기존 서비스 전체가 깨진다.

---

## 5. 작업 방식 (AGENTS.md 워크플로우 준수)

- 단위 커밋으로 쪼갤 것 (예: IR/Loader 골격 → Plan 편입 → Record Validation
  → View Form/Handler → Transport Multipart → Cloudflare Codegen → 테스트,
  각각 별도 커밋).
- 커밋 메시지: `type(scope): 내용` 형식.
- 문제 발견 시 기존 커밋 amend 금지, 새 커밋으로 추가 (append-only).
- 완료 후 보고에 반드시 포함:
  - 커밋별 요약 + **실제 diff**.
  - 새로 추가/수정된 테스트 목록.
  - 애매하거나 임의로 판단한 지점과 근거 (특히 에러 코드 명칭 최종 선택).
  - "구현되어 있다"와 "실제로 실행해서 확인했다"를 구분해서 보고.
  - **로더 기본값(`true`) 처리가 실제로 기존 예제 전체에서 회귀 없음을
    실행하여 확인한 결과**를 명시적으로 보고 (4절 마지막 항목).

---

## 6. 이 작업 완료 후 갱신할 문서

- `docs/ir-spec.md`: 3절(Field-level 공통 속성)에 `client_writable` 스펙 추가,
  5절에 이 규칙과 `ownership_field` 자동 주입 규칙의 관계 명시.
- `docs/resource-guide.md`: 3절 표에 해당 없음(타입이 아니라 공통 속성이므로),
  7절에 패턴 8("client_writable 없이 민감 필드를 공개 폼에 노출")으로
  Good/Bad 예제 추가.
- `docs/getting-started.md`: 5.1절 회원가입 섹션을 갱신 — `client_writable`
  적용 후에도 여전히 glue 핸들러(`/signup`)가 필요한지, 아니면 기본 View
  폼만으로 안전한 가입이 가능해졌는지 실측 후 반영.
- `NOW.md`: Task 8.2 완료 표시 및 다음 백로그 후보 갱신.
- `TASKS.md`: Task 8.2 항목을 완료 메모 포함하여 갱신.
- 새 회고 문서 작성 여부는 마일스톤 규모 변경인지 판단해서 결정
  (AGENTS.md "회고와 핸드오프" 섹션 참고).
