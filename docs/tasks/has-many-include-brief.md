# 작업 지시서: `has_many` 관계 Eager Loading (`?include=`) 확장 구현

> 이 문서는 소스 코드에 접근 가능한 구현 에이전트에게 전달하기 위한 작업 브리핑이다.
> 작업 착수 전 `NOW.md`의 "읽는 순서"에 따라 프로젝트 문서를 먼저 읽을 것.
> 특히 `docs/tasks/has-many-eager-loading-brief.md`(이 작업의 설계 승인 문서 —
> 옵션 A / `400 INCLUDE_TOO_LARGE` / 점 체이닝 배제가 이미 실측 근거와 함께
> 확정됨)와 `docs/ir-spec.md` 5.7절(현행 `belongs_to` `?include=` 스펙)을
> 반드시 확인할 것.

---

## 1. 배경 (왜 이 작업을 하는가)

`docs/tasks/has-many-eager-loading-brief.md`에서 옵션 A(개별 레코드별
`auth.Evaluate` 평가, 기존 `belongs_to` embed 패턴 그대로 확장)가 실측
벤치마크(`BenchmarkEvaluate_1000Records`, 1,000건 평가 0.182ms)와 함께
채택 확정되었다. `SakeRecord`처럼 `has_many` 자식(`SakeImage`,
`RecordTag`)을 갖는 리소스가 목록 화면 하나를 위해 클라이언트에서 4회
이상의 순차 요청과 수백 줄의 그룹핑 코드를 필요로 하는 구조적 한계를
해소하기 위함이다.

**이미 해결되어 이 작업 범위가 아닌 것** (중복 작업 방지):
- `belongs_to` `?include=` — Task 5.5에서 완료. 이번 작업은 그 코드 경로를
  최대한 재사용하되, `belongs_to` 자체의 동작은 변경하지 않는다.
- Nested Writes (쓰기 경로) — `docs/tasks/nested-writes-brief.md`에서
  별도로 다룸. 이번 작업은 순수 조회(GET) 경로만 다룬다.
- Blob 필드 스트리밍 — 이미 해결됨, 이번 스코프 아님.

---

## 2. 스코프

### 반드시 할 것

- `?include=` 파라미터가 `has_many` 관계명을 포함할 때, 배치 쿼리로 자식
  레코드를 가져와 부모 응답에 배열 필드(`"images": [...]`)로 embed한다.
- 관계당 자식 개수 상한 **50건**을 적용한다. 초과 시 해당 요청 전체를
  `400 Bad Request` (`code: INCLUDE_TOO_LARGE`, message:
  `"relation '...' has more than 50 records; use the dedicated list endpoint"`)
  로 거부한다 (silent truncation 금지 — 브리핑 5-1절 확정 사항).
- 자식 레코드 각각에 대해 `auth.Evaluate(sess, targetRes, auth.ActionRead,
  rec, nil)`를 개별 호출하여 권한 거부된 레코드는 결과 배열에서 제외한다
  (포함하지 않을 뿐 `null`로 채우지 않는다 — `belongs_to`의 "식별 불가능성
  원칙"과 달리 `has_many`는 배열이므로 자연스럽게 없는 것으로 처리).
- soft-deleted 자식 레코드는 배치 쿼리 조건(`WHERE ... AND deleted_at IS
  NULL`)에서부터 제외한다.
- 자식이 0건인 경우 빈 배열(`[]`)을 반환한다 (`null` 아님).
- N+1 방지를 위해 부모 레코드들의 PK를 모아 자식 리소스에
  `WHERE {foreign_key} IN (?, ?, ...)` 배치 쿼리 1회만 실행한다. 기존
  `storage.Query.IDs` 패턴(Task 5.5, belongs_to 배치 조회에 사용된 것과
  동일한 메커니즘)을 재사용하되, 대상 필드를 PK(`id`)가 아니라 FK
  컬럼(`record_id` 등)으로 바꿔 쿼리하는 경로를 추가한다.
- `?include=`에 `has_many`와 `belongs_to`를 쉼표로 섞어 지정할 수 있어야
  한다 (예: `?include=owner,images,record_tags` — `owner`는 belongs_to,
  나머지는 has_many).
- `?include=`에 점(`.`)이 포함된 체이닝 문법(예: `record_tags.tag`)이
  들어오면 `400 Bad Request` (`code: INVALID_INCLUDE`, message:
  `"nested include chaining is not supported"`)로 명시적으로 거부한다
  (조용히 무시하거나 부분 처리하지 않는다).
- 아래 5개 지점에 동일 규칙을 반영한다 (Go/Cloudflare 패리티,
  REST/SSR View 패리티):
  1. `transport/include.go` (또는 현재 실제 위치) — REST API List/Detail.
  2. `view/handler.go`의 `renderList`/`renderDetail` — SSR HTML View.
  3. `codegen/cloudflare/generator.go`의 `processIncludes` — Cloudflare TS
     Target List/Detail.
  4. Cloudflare TS Target의 SSR 상당 경로(있다면).
  5. (확인용) 기존 `belongs_to` 처리 경로와 응답 스키마가 충돌하지 않는지
     — 같은 응답 안에 `belongs_to` embed(`"owner": {...}` 단일 객체)와
     `has_many` embed(`"images": [...]` 배열)이 동시에 섞여도 정상 직렬화
     되는지 확인.
- Cloudflare TS Target에서도 `WHERE {fkCol} IN (${placeholders})` 배치
  쿼리를 1회 실행하고, 그룹핑은 인메모리 Map(`parent_id → [child, ...]`)으로
  처리한다.

### 절대 하지 말 것

- 점(`.`) 체이닝(2-depth 이상 중첩) 문법을 구현하지 말 것 — 브리핑 5-2절에서
  N:M 문제이지 조회 depth 문제가 아니라고 판단되어 명시적으로 배제됨.
- 상한 초과 시 부분 응답(`truncated: true`)을 만들지 말 것 — 브리핑 5-1절에서
  명시적 400 거부로 확정됨.
- 옵션 B(부모 `OwnerFilter` SQL 조건 재사용)의 최적화 경로를 만들지 말 것 —
  자식이 부모와 다른 권한 정책을 가질 때 권한 우회로 이어지는 리스크가
  실측으로 배제되지 않았으므로, 이번 작업에서는 옵션 A(개별 평가)만 구현.
- `resource/ir.go`, `docs/ir-spec.md`의 기존 `Relation`/`Field` 구조체를
  변경하지 말 것 — 이번 확장은 쿼리 레이어 로직 확장이며 IR 변경이 아님
  (브리핑 3절에서 스키마/마이그레이션 영향 없음이 확정됨).
- 자식 리소스의 `auth.permissions.create`/`update`/`delete`에는 어떤
  영향도 주지 말 것 — 이번 작업은 `ActionRead` 조회 경로만 다룬다.

---

## 3. 설계 스펙

### 3.1 쿼리 파라미터 파싱

```
GET /api/sake_records?include=owner,images,record_tags
```

- 쉼표로 분리된 각 토큰을 부모 리소스의 `relations` 목록과 대조한다.
- 토큰에 `.`이 포함되어 있으면 즉시 `400 INVALID_INCLUDE` (nested chaining
  거부).
- 존재하지 않는 관계명이면 기존 규칙 그대로 `400 INVALID_INCLUDE`.
- `kind`가 `belongs_to`면 기존 Task 5.5 경로, `has_many`면 이번에 추가하는
  신규 경로로 분기.

### 3.2 배치 조회 및 권한 평가 (Go 런타임 의사코드)

```go
// 1. 부모 목록에서 PK 수집
parentIDs := collectIDs(parentRecords)

// 2. has_many 자식 리소스 배치 조회 (deleted_at IS NULL 포함)
childRecords, err := store.List(ctx, childRes, storage.Query{
    ForeignKey: rel.ForeignKey, // "record_id" 등
    IDs:        parentIDs,
})

// 3. 상한 체크 (관계당, 즉 전체 childRecords 기준이 아니라
//    parent별 그룹 기준으로 50건 초과 여부를 판정)
groups := groupByForeignKey(childRecords, rel.ForeignKey)
for parentID, children := range groups {
    if len(children) > 50 {
        return errINCLUDE_TOO_LARGE(rel.Name)
    }
}

// 4. 레코드별 개별 권한 평가 + Sanitize
for parentID, children := range groups {
    var embedded []map[string]any
    for _, rec := range children {
        _, allowed, _ := auth.Evaluate(sess, childRes, auth.ActionRead, rec, nil)
        if !allowed {
            continue
        }
        embedded = append(embedded, SanitizeRecord(childRes, rec))
    }
    if embedded == nil {
        embedded = []map[string]any{} // nil이 아니라 빈 배열 직렬화 보장
    }
    parentMap[parentID][rel.Name] = embedded
}
```

- `storage.Query`에 `ForeignKey`/`IN` 조건을 표현하는 필드가 이미 있는지
  (Task 5.5의 `IDs []any` 필드를 재사용 가능한지) 확인 후, 없다면 최소
  확장으로 추가한다. PK 기준 `IN` 조회와 FK 기준 `IN` 조회는 컬럼명만
  다를 뿐 쿼리 형태가 동일하므로 하나의 헬퍼로 통합 가능한지 검토할 것.
- 상한 체크는 "관계당 총 레코드 수"가 아니라 **"부모 1건당 자식 수"** 기준
  임에 유의 (브리핑 4.1절 "관계당 최대 N건"의 의도가 부모별 자식 개수임을
  명확히 할 것 — 애매하면 구현 전 확인 요망).

### 3.3 에러 응답 형식

```json
{"error":{"code":"INCLUDE_TOO_LARGE","message":"relation 'images' has more than 50 records for one or more parent records; use GET /api/sake_images?record_id=... instead"}}
```

```json
{"error":{"code":"INVALID_INCLUDE","message":"nested include chaining ('record_tags.tag') is not supported"}}
```

---

## 4. 검증 조건 (완료 기준)

- [ ] `transport` 패키지 유닛 테스트: has_many 배치 조회 + 상한 체크 +
      권한 개별 평가 로직.
- [ ] REST API E2E (`httptest`): `GET /api/sake_records?include=images`가
      각 레코드에 `images: [...]` 배열을 정확히 embed하는지, 0건인 레코드는
      `images: []`인지 확인.
- [ ] 권한 우회 회귀 테스트: 자식 리소스가 `role:admin` 전용 read 정책을
      가질 때, 일반 사용자로 `?include=`를 걸어도 해당 자식이 배열에서
      빠지는지 확인 (soft-deleted, FK 불일치 케이스와 함께 3가지 시나리오
      raw HTTP 로그로 검증).
- [ ] 상한 초과 E2E: 51건 이상의 자식을 가진 부모에 `?include=`를 걸었을 때
      `400 INCLUDE_TOO_LARGE` raw 응답 확인.
- [ ] 점 체이닝 거부 E2E: `?include=record_tags.tag` 요청 시
      `400 INVALID_INCLUDE` raw 응답 확인.
- [ ] `belongs_to` + `has_many` 혼합 E2E: `?include=owner,images`가 한
      응답 안에서 단일 객체(`owner`)와 배열(`images`)을 동시에 정상
      직렬화하는지 확인.
- [ ] SSR HTML View E2E: `GET /view/sake_records?include=images`가 200 OK로
      정상 렌더링되는지 확인.
- [ ] Cloudflare TS Codegen 생성 코드 유닛 테스트 + Miniflare 실측: Go
      런타임과 동일한 상한/거부/권한 필터링 패리티 확인
      (`docs/retrospectives/cloudflare-codegen-review.md`의 "커버리지 격차
      점검" 체크리스트에 따라 raw HTTP 로그 첨부).
- [ ] 기존 `examples/drink-log-pilot` 전체 테스트(`pilot_test.go` 포함)
      회귀 없이 통과.
- [ ] `go test ./...` 및 `go build ./...` 전체 통과.
- [ ] **DB 스키마 변경이 실제로 0건임을 확인**: 이번 작업 전후로
      `schema.sql`/DDL 생성 결과에 diff가 없음을 실행하여 증명 (브리핑
      3절의 "마이그레이션 불필요" 판단이 실측과 일치하는지 최종 확인).

---

## 5. 작업 방식 (AGENTS.md 워크플로우 준수)

- 단위 커밋으로 쪼갤 것 (예: `storage.Query` FK 배치 조회 확장 → Transport
  has_many include 로직 → View 적용 → Cloudflare Codegen 적용 → 테스트,
  각각 별도 커밋).
- 커밋 메시지: `type(scope): 내용` 형식.
- 문제 발견 시 기존 커밋 amend 금지, 새 커밋으로 추가 (append-only).
- 완료 후 보고에 반드시 포함:
  - 커밋별 요약 + **실제 diff**.
  - 새로 추가/수정된 테스트 목록.
  - 애매하거나 임의로 판단한 지점과 근거 (특히 3.2절의 "관계당 상한" 기준이
    부모별인지 전체 합산인지 — 구현 중 애매하면 반드시 명시).
  - "구현되어 있다"와 "실제로 실행해서 확인했다"를 구분해서 보고. 벤치마크나
    성능 관련 주장을 포함할 경우 이전 리뷰에서 요구된 대로 **raw
    `go test -bench` 출력을 그대로 첨부**할 것 (요약/추정치 금지).

---

## 6. 이 작업 완료 후 갱신할 문서

- `docs/ir-spec.md`: 5.7절을 `has_many` 확장 내용으로 갱신 (상한, 응답 형태,
  점 체이닝 배제, 권한 평가 방식).
- `docs/resource-guide.md`: N:M 관계 조인 조회 가이드 섹션에 `has_many`
  eager loading 사용법 및 상한 안내 추가.
- `NOW.md`, `TASKS.md`: 완료 표시 및 다음 백로그(Nested Writes) 갱신.
- 회고 문서 작성 여부는 마일스톤 규모 변경인지 판단해서 결정.