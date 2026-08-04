# Phase 7 회고: Field-level 권한 분석, 세션 Escape Hatch 및 DX Papercuts

> 이 문서는 `docs/getting-started.md` 튜토리얼 dogfooding 과정에서 발견된 마찰을 해결한 Phase 7 (Task 7.1 ~ 7.4)의 경위, 발견된 문제 패턴 및 향후 개발 가이드라인을 기록한 회고 문서입니다.

---

## 1. 개요

Phase 7에서는 Mold 튜토리얼(`docs/getting-started.md`)을 개발자 관점에서 실제로 따라 하며(dogfooding) 발견된 두 가지 권한 문제와 DX 마찰 요소를 정밀 분석하고 완결했습니다.

1. **Task 7.1**: Field-level 권한 상승/소유권 위조 패턴 실증 및 IR 확장 여부 판정 (`[기각]` 확정)
2. **Task 7.2**: 세션 사용자 조회 공개 API (`app.SessionUser`) 구현 및 `examples/quickstart/` (`basic`/`with-auth`) 디렉터리 구조 재편
3. **Task 7.3**: 로그인 폼 UI 라벨 "Username" ➔ "Email" 정정
4. **Task 7.4**: Destructive-only Migration 트러블슈팅 FAQ 문서화 및 실측 테스트 체계 구축

---

## 2. 수집된 주요 레슨 및 문제 패턴

### 레슨 1: 추정 기반 문제 등재 금지 및 원천 코드 확인의 중요성 (Task 7.1)
* **문제 상황**: 최초 Task 7.1 발의 시 *"누구나 JSON 본문에 `role: admin`을 실어 보내면 관리자 계정이 생성된다"*고 배경에 서술되었으나, 실제 소스 코드(`auth/permission.go` L48-L55)를 검증한 결과 이미 Milestone 5 시점에 non-admin 유저의 `"role"` 필드 기재를 `403 Forbidden` (`ErrPrivilegeEscalation`)으로 차단하는 가드가 원천 작동 중이었음.
* **교훈**: 백로그 및 이슈를 등재하기 전에 반드시 실제 구현 코드와 raw HTTP 실측을 먼저 수행하여 관찰 사실의 정합성을 확인해야 함. "코드를 고치기 전에 문제 정의부터 검증하라"는 AGENTS.md 원칙 재확인.

### 레슨 2: IR 무분별한 확장 억제와 Escape Hatch 패턴 채택 (Task 7.1 ~ 7.2)
* **문제 상황**: `Post.author_id` 및 `SakeRecord.owner_id`의 소유권 필드 위조 문제에 대해 IR 수준의 필드 단위 권한(field-level permission) 도입이 논의됨.
* **판정 및 교훈**: 위조 가능 케이스가 2건으로 채택 기준(3건 이상)에 미달하며, 필드 권한을 IR에 추가할 경우 파이프라인(Storage/Transport/View/Codegen) 전체에 거대한 복잡성을 유발함. 마세라티 원칙 및 단일 소스 오브 트루스 철학에 따라 **IR 확장은 기각**하고, 애플리케이션 레이어에서 `app.SessionUser(r)`를 활용해 서버가 소유권 필드를 고정하는 **glue 핸들러 우회 패턴**을 확정 채택함.

### 레슨 3: 0-Duplicate 및 Idiomatic Go API 설계 (Task 7.2)
* **설계 포인트**:
  - HTTP 핸들러의 미인증 상태는 예외(error)가 아닌 정상 제어 흐름이므로 `(userID int64, role string, ok bool)` 시그니처를 채택하여 `if !ok` 분기를 간결하게 만듦.
  - `auth.SessionManager.GetSessionFromRequest(r)`를 신설하여 `transport.Router`와 `runtime.App` 간 세션 쿠키 추출 및 DB 조회 로직 중복을 100% 제거함.

### 레슨 4: 예제 디렉터리 분리 및 문서 패리티 (Task 7.2)
* **문제 상황**: `.txt` 파일 확장자로 signup 버전을 우회 보관하거나 기본 `main.go`를 덮어쓸 경우, 튜토리얼 1~4절(Post 단일)과 5절(auth/signup)의 예제 코드가 어긋나는 마찰 발생.
* **해결**: `examples/quickstart/basic/` (Post 단일)과 `examples/quickstart/with-auth/` (User+Post+Session)로 디렉터리를 깔끔히 분리하고, `quickstart_test.go` E2E 실측 테스트로 문서 예제의 정상 동작을 자동 검증함.

### 레슨 5: 테스트 Assertion 조건식의 엄격성 (Task 7.4 리뷰)
* **문제 상황**: `migration_troubleshooting_test.go` 작성 시 De Morgan의 법칙 오류로 `if w2.Code != 400 && !strings.Contains(...)` 형태로 작성되어, 상태 코드가 400이 아니거나 에러 메시지가 일치하지 않아도 조건문이 뚫리는 느슨한 assertion이 유입됨.
* **교훈**: `if w2.Code != 400 || !strings.Contains(...)`로 수정을 진행함. "테스트가 통과했다"에 만족하지 않고, **테스트 assertion 조건 자체가 기대하는 에러 코드와 메시지를 모두 엄격하게 평가하는지** 2차 검증해야 함.

---

## 3. 체크리스트 (다음 마일스톤 적용)

- [ ] 새 이슈 발의 시 관련 소스 코드를 읽고 실제 동작(Empirical Ground Truth)을 확인했는가?
- [ ] 런타임 코어 IR 확장 대신 애플리케이션 우회(Escape Hatch / Glue Handler)로 더 단순하게 해결할 수 있는지 검토했는가?
- [ ] 예제 코드가 문서의 각 절/단계와 1:1로 일치하며, 컴파일 가능한 형태로 독립 관리되고 있는가?
- [ ] 테스트 코드의 assertion 조건문(`||` vs `&&`)이 부정 실패 조건을 엄격하게 포획하고 있는가?
