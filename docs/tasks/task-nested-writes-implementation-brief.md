# 작업 지시서: Nested Writes (`관계형 중첩 쓰기` Option B) — 구현 명세서

> **승인 상태**: `docs/tasks/nested-writes-brief.md`에 기반하여 리뷰어(사용자)가 **옵션 B (순차 생성 + 사전 검증 + 보상 롤백)** 로 공식 확정한 구현 지시서이다.
>
> **보고 원칙 준수**: 모든 단위 커밋 완료 보고 시 diff는 반드시 `git diff`, `git log -p`, `git show`의 raw stdout을 그대로 복사하여 첨부하며, 수동 타이핑 및 편집을 영구 금지한다 (`docs/retrospectives/has-many-include-diff-fabrication-incident.md`).

---

## 1. 배경 및 목표

* **문제**: `drink-log`에서 사케 기록 1건 저장 시 `부모 POST → 이미지 N건 POST → 태그 M건 POST`로 6~10회 이상의 순차 HTTP 요청이 발생하고 중간 실패 시 고아 레코드가 남음.
* **목표**: `POST /api/{parent}` 1회의 HTTP 요청 본문에 `has_many` 자식 레코드 배열을 중첩하여 전달하면, 부모 생성과 자식 생성들을 원자적으로(실패 시 보상 롤백) 처리하여 `201 Created`로 응답.

---

## 2. 반드시 할 것 (Must-Do)

1. **`storage.Store` 인터페이스 무변경 유지**:
   - `storage.Store` 인터페이스에 트랜잭션 메서드를 추가하지 않고, 기존 `Create`, `HardDeletePhysically`를 조합하여 Transport 계층에서 오케스트레이션 수행.
2. **철저한 사전 검증 (Pre-validation)**:
   - 부모 레코드를 DB에 INSERT하기 전에, 페이로드에 포함된 모든 `has_many` 자식 레코드들에 대해:
     1. 자식 개수 상한 검증 (부모당 최대 50건, 초과 시 `400 NESTED_WRITE_TOO_LARGE`).
     2. 자식 스키마 유효성 검증 (`resource.ValidateRecord(childRes, childPayload, false)`).
     3. 자식 `client_writable` 검증.
     4. 자식 생성 권한 검증 (`auth.Evaluate(sess, childRes, auth.ActionCreate, nil, childPayload)`).
   - 단 하나라도 사전 검증에 실패하면 부모 레코드를 생성하지 않고 즉시 `400 Bad Request` 또는 `403 Forbidden`을 반환.
3. **순차 생성 및 부모 FK / 소유권 자동 주입**:
   - 부모 레코드 생성 (`parentStore.Create`).
   - 각 자식 레코드에 `childPayload[rel.ForeignKey] = parentRecord["id"]` 자동 주입.
   - 자식 리소스에 `ownership_field`가 정의되어 있고 인증 세션이 존재하면 `childPayload[ownership_field] = sess.UserID` 자동 주입.
   - 자식 레코드 순차 생성 및 생성된 ID 목록 추적 (`createdTracker`).
4. **실패 시 보상 롤백 (Compensating Rollback)**:
   - 자식 레코드 생성 도중 DB 제약조건 위반(예: `unique_together`)이나 에러 발생 시:
     1. 이미 생성된 자식 레코드들을 역순으로 `HardDeletePhysically(ctx, childRes, childID)` 호출.
     2. 부모 레코드를 `HardDeletePhysically(ctx, parentRes, parentID)` 호출하여 물리적 롤백.
     3. `400 Bad Request` (또는 `500 INTERNAL_ERROR`)로 실패 원인 응답.
5. **성공 시 일체형 응답 (201 Created)**:
   - 부모 레코드와 생성된 자식 레코드 배열(`images: [...]`, `record_tags: [...]`)이 임베드된 일체형 JSON 응답 반환.
6. **Cloudflare Workers TS 타깃 생성기 동기화**:
   - `codegen/cloudflare/generator.go`의 `POST /api/{table}` 핸들러 템플릿에 동일한 사전 검증 ➔ 부모 INSERT ➔ 자식 INSERT ➔ catch 시 역순 DELETE 보상 롤백 로직 구현 및 Miniflare E2E 검증.

---

## 3. 절대 하지 말 것 (Must-Not-Do)

1. **`storage.Store`에 트랜잭션 추상화 추가 금지**: Go 런타임과 Cloudflare D1 간의 비대칭 추상화를 방지하기 위해 스토리지 레이어는 순수한 단일 레코드 CRUD로 유지한다.
2. **2-depth 이상 중첩 쓰기 금지**: 자식의 자식(2-depth) 중첩 쓰기는 허용하지 않으며, 1-depth `has_many` 관계로 제한한다.
3. **`belongs_to` 중첩 쓰기 금지**: 부모 생성 시 조부모 생성 등은 FK 의존 방향상 모호하므로 배제한다.
4. **보고서 Diff 수동 재구성 금지**: 모든 보고서의 diff는 반드시 `git diff` raw stdout만 첨부한다.

---

## 4. 단위 커밋 계획

1. **커밋 1**: `feat(transport): implement nested writes for has_many relations with pre-validation and compensating rollback`
   - [transport/handler.go](file:///D:/dev/mold/transport/handler.go)의 `handleCreate`에 nested write 파싱, 사전 검증, 순차 생성, `HardDeletePhysically` 롤백 오케스트레이션 구현.
   - [transport/nested_writes_test.go](file:///D:/dev/mold/transport/nested_writes_test.go) 단위 및 에러/롤백/권한 검증 테스트 추가.
2. **커밋 2**: `feat(codegen/cloudflare): implement nested writes in Cloudflare TypeScript generator`
   - [codegen/cloudflare/generator.go](file:///D:/dev/mold/codegen/cloudflare/generator.go)의 `POST /api/{table}` 템플릿에 TS 중첩 쓰기 및 보상 롤백 구현.
   - [codegen/cloudflare/generator_test.go](file:///D:/dev/mold/codegen/cloudflare/generator_test.go)에 Miniflare E2E 검증 추가.
3. **커밋 3**: `test(e2e): verify nested writes in drink-log pilot and full repository test suite`
   - [examples/drink-log-pilot/pilot_test.go](file:///D:/dev/mold/examples/drink-log-pilot/pilot_test.go)에 `POST /api/sake_records` (with `images`, `record_tags`) 단일 요청 1-step 생성 E2E 테스트 추가.
   - `go test ./... -count=1` 전수 검증.
