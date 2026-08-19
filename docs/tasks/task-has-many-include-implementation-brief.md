# 작업 지시서: `has_many` 관계 Eager Loading (`?include=`) 구현

> 이 문서는 소스 코드에 접근 가능한 구현 에이전트에게 전달하기 위한 작업 브리핑이다.
> 작업 착수 전 `NOW.md`의 "읽는 순서"에 따라 프로젝트 문서를 먼저 읽을 것.
> 특히 `docs/tasks/has-many-include-brief.md`(이 작업의 설계 승인 문서 —
> 옵션 A / 부모당 자식 50건 상한 / `400 INCLUDE_TOO_LARGE` 거부 / 점 체이닝 배제가
> 이미 사람 승인으로 확정됨)와 `docs/ir-spec.md` 5.7절을 반드시 확인할 것.

---

## 1. 배경 (왜 이 작업을 하는가)

`docs/tasks/has-many-include-brief.md`에서 옵션 A(개별 인메모리 `auth.Evaluate` 평가 기반 `has_many` Eager Loading 확장)가 채택 확정되었다.

Task 5.5로 도입된 `?include=`는 `belongs_to` 관계만 지원하여, 사케 기록 1건과 사진/태그를 조회하기 위해 모바일 클라이언트가 4번 이상의 개별 HTTP 요청을 보내고 수백 줄의 수동 조립 코드를 작성해야 했던 실증된 병목(`drink-log-feedback` RFC)을 해결하기 위함이다.

**이미 해결되어 이 작업 범위가 아닌 것** (중복 작업 방지):
- `belongs_to` Eager Loading → Task 5.5에서 이미 완결 및 검증됨.
- 쓰기(Nested Writes) 경로 → `docs/tasks/nested-writes-brief.md`에서 별도 진행.
- Blob 필드 스트리밍 → 프로덕션에서 이미 분리 해결됨.

---

## 2. 스코프

### 반드시 할 것
1. **`has_many` 관계 `?include=` 조회 지원**:
   - `GET /api/{table}?include=relation1,relation2` 및 `GET /api/{table}/{id}?include=relation1,relation2`에서 `has_many` 관계로 선언된 자식 리소스 목록을 부모 레코드의 `"relation_name": [...]` 배열로 embed.
   - 자식이 0건인 경우 `null`이 아니라 반드시 **빈 배열(`[]`)**로 반환.
2. **부모 1건당 자식 최대 50건 상한 검증**:
   - 부모 레코드 1건에 매핑되는 자식 레코드 수가 50건을 초과하면 즉시 **HTTP 400 Bad Request** (`INCLUDE_TOO_LARGE`)로 거부.
   - 응답 포맷: `{"error":{"code":"INCLUDE_TOO_LARGE","message":"nested records for relation 'images' exceed limit of 50"}}`
3. **N+1 방지 1회 배치 쿼리 실행**:
   - 부모 레코드들의 PK 목록(`parentIDs: [id1, id2, ...]`)을 모아 자식 리소스의 FK 컬럼에 대해 단 1회의 SQL 배치 쿼리(`WHERE {foreign_key} IN (?, ?, ...) AND deleted_at IS NULL`)로 조회.
   - Go 런타임: `storage.Query.Filter`에 슬라이스(`[]any`)가 전달될 때 `AND "{column}" IN (?, ?, ...)` SQL을 생성하도록 `adapters/sqlite/crud.go`의 `List` 메서드 지원.
   - Cloudflare TS Codegen: `processIncludes`에서 `SELECT * FROM "${targetTable}" WHERE "${fkCol}" IN (${placeholders})${softCond}` 1회 실행 후 인메모리 Map 그룹핑.
4. **개별 인메모리 `auth.Evaluate` 및 Sanitization (옵션 A)**:
   - 배치 조회된 자식 레코드 각각에 대해 `auth.Evaluate(sess, targetRes, auth.ActionRead, childRec, nil)`를 실행하여 권한이 허용된 레코드만 자식 배열에 포함.
   - 자식 레코드에 `SanitizeRecord`를 적용하여 `password` 및 `deprecated` 필드를 안전하게 은폐.
5. **SSR View 지원**:
   - `view/handler.go`의 `renderList`/`renderDetail`에서도 `has_many` include가 동일하게 동작하도록 지원.
6. **점(`.`) 체이닝 문법 명시적 거부**:
   - `?include=record_tags.tag`와 같이 점(`.`)이 포함된 다단계 중첩 시도 시 즉시 **HTTP 400 Bad Request** (`INVALID_INCLUDE`)로 거부.
   - 클라이언트 가이드: `Tag`와 같은 준정적 메타데이터는 앱 시작 시 `GET /api/tags`로 1회 조회 후 클라이언트 캐시에서 매핑하도록 문서화.
7. **Go 런타임 및 Cloudflare TS Codegen 100% 패리티 달성 및 실측 테스트**.

### 절대 하지 말 것
- `storage.Store` 인터페이스의 시그니처를 파괴하거나 변경하지 말 것.
- 2단계 이상 깊이의 점 체이닝(`.`)이나 순환 참조를 지원하려 하지 말 것 (마세라티 원칙).
- `truncated: true` 등 응답 배열 구조를 객체로 래핑하여 클라이언트 직관성을 해치는 암묵적 처리를 만들지 말 것.
- `has_many` embed 시 자식이 없을 때 `null`로 반환하지 말 것 (belongs_to의 null과 구분하여 항상 `[]` 반환).

---

## 3. 설계 스펙

### 3.1 Go 런타임 ([transport/include.go](file:///D:/dev/mold/transport/include.go))

```go
// belongs_to 와 has_many 관계 모두 수용하도록 분기 확장
for _, rel := range requestedRels {
    switch rel.Kind {
    case resource.KindBelongsTo:
        // 기존 belongs_to 로직 (WHERE id IN (fkVals) -> 단일 객체 매핑 또는 null)
    case resource.KindHasMany:
        // 1. 부모 레코드들의 PK (id) 수집
        // 2. targetEntry.Store.List(ctx, targetRes, storage.Query{ Filter: map[string]any{ rel.ForeignKey: parentIDs } })
        // 3. 자식 레코드별 soft_delete 확인, auth.Evaluate(ActionRead), SanitizeRecord 수행
        // 4. parentID별로 그룹핑 (자식 수 > 50건이면 ErrIncludeTooLarge 반환)
        // 5. 각 부모 레코드에 rec[rel.Name] = childList (0건이면 []storage.Record{})
    default:
        return ErrInvalidInclude{Relation: rel.Name}
    }
}
```

### 3.2 SQLite 어댑터 ([adapters/sqlite/crud.go](file:///D:/dev/mold/adapters/sqlite/crud.go))

`query.Filter` 맵을 순회할 때 값이 슬라이스(`[]any`, `[]int64`, `[]string` 등)인 경우 `AND "{key}" IN (?, ?, ?)` 쿼리를 생성하여 단일 ID와 복수 ID 조회를 투명하게 지원.

### 3.3 Cloudflare TS Codegen ([codegen/cloudflare/generator.go](file:///D:/dev/mold/codegen/cloudflare/generator.go))

`processIncludes` 헬퍼 함수 템플릿에 `has_many` 분기를 추가:
1. `rel.info.kind === 'has_many'`인 경우 부모 `r.id`들을 수집.
2. `c.env.DB.prepare('SELECT * FROM ... WHERE fkCol IN (...)').all()` 1회 실행.
3. 인메모리 Map `parentID -> child[]` 그룹핑 및 부모당 50건 초과 시 `writeError(c, 400, 'INCLUDE_TOO_LARGE', ...)` 반환.
4. `permRead` 권한 검사 및 `sanitizeRecord` 적용 후 `r[rel.name] = childList || []`.

---

## 4. 검증 조건 (완료 기준)

- [ ] `transport/include_test.go`:
  - `has_many` 단일/다중 include 정상 동작 (`?include=images,record_tags`).
  - 자식 레코드 0건일 때 `[]` 빈 배열 반환 검증.
  - 부모당 50건 초과 시 `400 INCLUDE_TOO_LARGE` 거부 검증.
  - 점 체이닝(`record_tags.tag`) 시도 시 `400 INVALID_INCLUDE` 거부 검증.
  - 자식 레코드 권한 평가(`auth.permissions.read: role:admin` 등) 시 미권한 사용자에게 해당 자식 레코드가 필터링되는지 검증.
  - 자식 레코드의 `password` / `deprecated` 필드가 `SanitizeRecord`로 은폐되는지 검증.
- [ ] `view/handler_test.go`: SSR View 목록/상세 화면에서 `has_many` include 렌더링 검증.
- [ ] `adapters/sqlite/crud_test.go`: `query.Filter`에 슬라이스 전달 시 `IN (...)` 쿼리 생성 및 조회 검증.
- [ ] `codegen/cloudflare/generator_test.go` 및 Miniflare E2E: Cloudflare 환경에서 `has_many` `?include=` 정상 응답, 50건 초과 400 거부, 빈 배열 반환 패리티 검증.
- [ ] `examples/drink-log-pilot` 통합 E2E: `GET /api/sake_records?include=images,record_tags`가 단 1회의 요청으로 부모+자식 전체를 올바르게 반환하는지 실측.
- [ ] `go test ./...` 전체 통과.
- [ ] **성능 벤치마크 raw 로그 첨부**: 1,000건 자식 레코드 포함 `ProcessIncludes` 벤치마크 실행 결과 첨부.

---

## 5. 작업 방식 (AGENTS.md 워크플로우 준수)

- **단위 커밋 원칙**:
  1. `feat(storage): support slice IN filtering in SQLite adapter query`
  2. `feat(transport): extend ProcessIncludes for has_many relations with limit and in-memory auth`
  3. `feat(codegen/cloudflare): implement has_many processIncludes in TypeScript runtime`
  4. `test(transport,view): add comprehensive unit and E2E tests for has_many include`
  5. `test(codegen/cloudflare): add Miniflare parity tests for has_many include`
- **보고 원칙**:
  - 커밋별 요약 및 **실제 git diff** 전문 첨부.
  - 새로 추가/수정된 테스트 목록 명시.
  - 벤치마크 raw 출력 결과 첨부.
