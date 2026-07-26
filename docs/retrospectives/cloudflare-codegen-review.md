# Cloudflare Codegen 리뷰 사이클 회고

> 이 문서는 Cloudflare Workers TS Codegen 기능 확장 (Auth/Password/View/Blob) 작업 시 발생한 1차 구현 리뷰 반려 패턴과 6대 결함 수정 과정을 기록하고, 향후 Codegen 및 보안 작업에 적용할 체크리스트와 지침을 정리한다.

---

## 1. 개요

Cloudflare Workers TS+Hono+D1+R2 Target Codegen 기능 확장 (Auth/Password/View/Blob) 작업에서 1차 4단계 구현 완료 보고 후, 2차례의 리뷰 과정에서 6개의 실질적 결함이 포착되어 반려되었다. 기존 Go backend 타깃 대비 보안 및 스펙 기능의 패리티를 유지한다는 지침이 부여되었음에도 1차 구현에서 보안 구멍, 스펙 누락, 미실행 검증 보고 등이 발생한 경위와 5대 표면 결함 수정 과정, 그리고 이를 시스템적으로 예방하기 위한 원칙과 체크리스트를 남긴다.

---

## 2. 발견된 문제 패턴 5가지

### 1) 인증 헤더 우회 (Authentication Header Spoofing)
- **무엇이 문제였는가**: `getAuthUser` 헬퍼 함수 구현 시 `x-user-id` 및 `x-user-role` HTTP 헤더가 존재하는 경우 세션 쿠키 검증을 스킵하고 해당 헤더 값을 인증된 사용자 신원으로 신뢰하는 비인가 우회 경로가 포함되었음.
- **왜 놓치기 쉬웠는가**: 로컬 개발 편의나 게이트웨이 상위 전달 구조를 멋대로 상상하여 헤더 기반 폴백을 추가했으나, 프로덕션 요청 경로에서는 클라이언트가 임의로 `x-user-id: 1`, `x-user-role: admin` 헤더를 조작하여 권한을 탈취(Privilege Escalation)할 수 있는 치명적 보안 구멍이었음. "Go backend와 동일한 Auth 패리티를 맞춘다"는 지시가 있었으나 Go `auth` 패키지에는 이러한 헤더 신뢰 로직이 0줄도 없었음에도 Codegen 작성 시 검증 없이 임의 판단으로 삽입됨.
- **어떻게 고쳤는가**: `getAuthUser`에서 `x-user-id`/`x-user-role` 헤더 폴백을 100% 제거하고, D1 테이블 `_mold_sessions`와 세션 쿠키 `mold_session` 검증만을 유일한 식별 경로로 교체함 (`7e7e59b`).
- **다음 작업에 적용할 원칙**: "이 작업이 새로운 인증/식별 경로를 추가하는가? 클라이언트가 임의로 조작 가능한 입력(헤더, 쿼리 파라미터 등)을 암묵적으로 신뢰하고 있지 않은가?"를 신뢰 경계(Trust Boundary) 차원에서 항상 먼저 점검할 것.

---

### 2) 약한 비밀번호 해싱 (Weak Password Hashing)
- **무엇이 문제였는가**: 비밀번호 필드 해싱 구현 시 레코드마다 고유한 random salt가 없는 정적/취약한 SHA-256 단일 라운드 구조로 1차 작성되었음.
- **왜 놓치기 쉬웠는가**: Web Crypto API의 `subtle.digest('SHA-256')`를 쓰면 간단히 해싱할 수 있다는 개발 생산성 및 단순함의 미명 아래, "Go 타깃(bcrypt 동등)" 수준의 보안 스펙 요건을 자의적으로 낮춰 잡았음.
- **어떻게 고쳤는가**: Web Crypto API의 `PBKDF2` (SHA-256, 100,000 iterations, 16-byte random salt per record, `$pbkdf2$<iterations>$<salt>$<hash>` format) 표준 해싱 알고리즘으로 전면 교체하고 `/login` 비동기 패스워드 검증(`verifyPassword`)을 통일 연결함 (`fd2e0c7`).
- **다음 작업에 적용할 원칙**: 해싱/암호화가 필요한 경우 알고리즘 이름뿐만 아니라 **salt 정책(레코드별 무작위 고유성)과 반복 횟수/cost factor를 요구사항 및 검증 조건에 명시적으로 정의**하고 이행할 것.

---

### 3) 스펙 일부 누락 (Spec Omission: 1-Step Multipart Create)
- **무엇이 문제였는가**: `docs/ir-spec.md` 5.5절의 Blob 스펙 중 2-Step 업로드/다운로드/삭제만 생성하고, 1-Step `multipart/form-data` 레코드 생성 시 R2 blob 동시 업로드 및 실패 시 물리적 hard delete(`DELETE FROM table WHERE id = ?`) atomic rollback 처리를 Stage D 최초 구현 시 누락한 채 "Blob 완성"으로 보고함.
- **왜 놓치기 쉬웠는가**: JSON API와 2-step REST 엔드포인트 구현만으로 Blob 기능이 동작한다고 착각하여 스펙 문서(5.5절)의 1-Step 원자적 롤백 명세를 꼼꼼히 대조하지 않고 보고 절차를 서둘렀음.
- **어떻게 고쳤는가**: `POST /api/{table}` 엔드포인트에 `multipart/form-data` 파싱 로직을 추가하여 텍스트 데이터 DB 생성과 R2 blob 업로드를 1-Step으로 통합하고, R2 업로드 실패 시 D1 레코드를 hard delete로 원자적 롤백 처리(실패 시 `500 BLOB_STORE_FAILED_RECORD_PRESERVED` 에러 엔벨로프 반환)를 구현함 (`e7b3358`).
- **다음 작업에 적용할 원칙**: 기능 완료를 선언하기 전 **스펙 문서(`docs/ir-spec.md`)의 각 절과 항목별로 1:1 대조 체크리스트를 작성하여 누락된 서브 명세가 없는지 전수 검증**할 것.

---

### 4) 미실행 검증 보고 (Unexecuted Verification Reporting)
- **무엇이 문제였는가**: `go test ./...` 단위 테스트(생성된 TS 코드 문자열의 regex match) 통과만을 근거로 "Cloudflare Workers / Miniflare 환경에서 실측 검증 완료"라고 보고함.
- **왜 놓치기 쉬웠는가**: 코드 생성기(Codegen) 작업에서는 "Go 런타임이 TS 소스 코드를 생성하는 단위 테스트"와 "생성된 TS 코드가 실제 V8 Isolate runtime(Miniflare)에서 HTTP 요청에 제대로 구동되는가"를 구별하지 못하고 쉽게 혼동했음.
- **어떻게 고쳤는가**: Node / Miniflare V8 Isolate 기반 실측 검증 러너(`scratch/run_empirical_miniflare.js`)를 구축하여 D1 DDL/DML, R2 bucket, Web Crypto PBKDF2, HTML Detail View, Multipart upload/download 등 8대 실제 HTTP 실측 시나리오를 100% 구동하고 raw 텍스트 로그를 추출해 검증을 완결함.
- **다음 작업에 적용할 원칙**: "구현되어 있다"와 "실제 Target 런타임 환경에서 구동하여 로그로 증명했다"를 엄격히 구분하고, 완료 조건에 **Target 런타임 실측과 raw 로그 첨부를 의무화**할 것.

---

### 5) 정규식 기반 Sanitizer의 우회 가능성 (Sanitizer Evasion Vulnerability)
- **무엇이 문제였는가**: Detail View XSS 방지를 위한 `sanitizeHTML` 정규식이 큰따옴표 이벤트 핸들러(`on\w+="[^"]*"`)만 제거하여 작은따옴표(`onload='...'`) 및 따옴표 없는(`onload=...`) XSS 주입을 막지 못했음.
- **왜 놓치기 쉬웠는가**: Go backend 타깃은 검증된 라이브러리(`bluemonday`)를 사용하여 안전하나, Cloudflare Workers 타깃은 경량화를 위해 정규식 대체 구현을 채택하면서 우회 케이스에 대한 정교한 보안 회피 공격 시나리오(Evasion Test Case)를 대조 검증하지 못했음.
- **어떻게 고쳤는가**: `sanitizeHTML` 정규식을 보강하여 큰따옴표, 작은따옴표, 따옴표 없음 (`on\w+\s*=\s*[^\s>]+`) 패턴 전체를 우회 없이 제거하도록 수정하고 회귀 단위 테스트를 추가함 (`3f323bc`).
- **다음 작업에 적용할 원칙**: 기존 타깃의 검증된 서드파티 라이브러리를 경량화 목적으로 자체 정규식/유틸로 대체할 때는 **커버리지 격차를 분석하고 우회 가능한 공격 패턴 목록(Evasion Test Vector)을 수립하여 대조 검증**할 것.

---

## 3. 근본 원인 재분류

이번 리뷰 반려 및 5개 표면 결함은 다음 2가지 근본 원인으로 압축된다:

1. **[실행 가능한 검증 절차 미비로 인한 자의적 검증 기준 하향]**  
   "Go 타깃 대비 보안/스펙 패리티를 유지하라"는 지시가 프롬프트에 명시되었음에도, 1차 구현 시 실행 가능한 구체적 검증 환경(Target 런타임 로그, raw HTTP 응답, Evasion 벡터 등)이 완료 조건으로 주어지지 않자 구현체 작성자가 `go test` 정적 문자열 포함 여부만으로 검증 완료 기준을 스스로 낮춰 잡는 경향이 나타남.
2. **[검증된 타깃 기능의 1:1 스펙 대조 누락]**  
   Go backend 런타임에서 이미 검증된 기능(세션 쿠키 전용 인증, bcrypt 동등 해싱, 1-step multipart create, bluemonday XSS sanitization)을 TS codegen 타깃으로 재구현할 때, 소스 코드 스펙 문서(`docs/ir-spec.md`)를 항목별로 1:1 대조하여 커버리지 격차가 없는지 전수 검사하는 절차가 부재했음.

---

## 4. 다음 Codegen/보안 관련 작업에 적용할 체크리스트

다음번 Codegen 확장이나 보안 관련 작업 착수 전 아래 5대 질문을 반드시 사전에 검토한다:

1. **[실행 가능한 검증 명시]** 완료 조건에 "단위 테스트 통과"뿐만 아니라 실제 Target 런타임 환경(Miniflare, 로컬 Isolate 등)에서의 구체적 시나리오 및 raw 실행 로그 제출 요구가 명시되어 있는가?
2. **[신뢰 경계 점검]** 이 작업이 새로운 인증/식별/인가 경로를 추가하는가? 추가한다면 각 경로가 클라이언트가 임의로 위조 가능한 입력(헤더, 쿼리 파라미터, 양식 필드)에 의존하지 않는지 신뢰 경계(Trust Boundary) 관점에서 점검했는가?
3. **[암호화 파라미터 명시]** 해싱/암호화가 필요한 경우, 알고리즘 이름뿐만 아니라 salt 정책(레코드별 무작위 고유성), 반복 횟수/cost factor까지 완료 조건 및 스펙에 구체적으로 명시했는가?
4. **[대체 구현의 커버리지 격차 점검]** 기존 타깃이 검증된 서드파티 라이브러리를 쓰는 지점을, 새 타깃에서 자체 정규식/유틸로 대체할 때 그 커버리지가 기존과 동등한지 우회 공격 케이스 목록(Evasion Vectors)으로 대조했는가?
5. **[diff 제출 시 출처 명시]** 코드 리뷰용으로 보고서에 제출하는 diff/코드 스니펫이 "실제 소스 코드(Go template/generator.go)"인지 "생성된 런타임 출력 예시(TS file)"인지 명확히 구별하여 명시하고 있는가?

---

## 5. 커밋 및 리뷰 사이클 요약

- **전체 커밋 수**: 총 12개 커밋 (1차 4단계 구현 4커밋 ➔ 리뷰 반려 ➔ 후속 수정 5커밋 ➔ 제네릭성 테스트 1커밋 ➔ 문서화 2커밋)
- **진행 과정**:
  1. **1차 구현 (4단계 4커밋)**: Stage A (Auth 가드), Stage B (SHA256 패스워드), Stage C (SSR View), Stage D (2-Step R2 Blob)
  2. **1차 리뷰 반려 및 보안/스펙 후속 수정 (5커밋)**:
     - `7e7e59b`: 인증 헤더 우회 제거 (`getAuthUser`)
     - `fd2e0c7`: Web Crypto PBKDF2 (100k iter, random salt) 해싱 및 `/login` 연결
     - `e7b3358`: 1-Step Multipart Create & D1 Hard Delete Atomic Rollback (`500 BLOB_STORE_FAILED_RECORD_PRESERVED`)
     - `3f323bc`: `sanitizeHTML` single-quote/unquoted XSS 이벤트 핸들러 보강
     - `64192dc`: D1 SQL 문자열 리터럴 홑따옴표(`''`) 정정 및 검증 테스트
     - `0a69dfc`: Form 데이터 숫자인 형태 파싱, TypeBlob initial insert `null` 바인딩 및 $pbkdf2$ string concatenation 보정
  3. **Miniflare 8대 실측 검증 통과**: 401, 404, 403 Cross-user, 403 Role escalation, PBKDF2 D1 Query & Login, XSS Sanitization, 1-Step Multipart Create/Download/Delete, Cross-user Upload 403 100% PASS raw 로그 첨부
  4. **2차 리뷰 및 제네릭성 검증 (1커밋)**:
     - `29d6b3e`: `SakePost` (테이블 `sake_posts`, blob 2개 `cover_image`, `attachment_file`) 제네릭성 및 다중 blob 검증 테스트 수립
  5. **문서화 (2커밋)**:
     - `fca4fac`: `NOW.md`, `TASKS.md`, `docs/ir-spec.md` 5.5절 다중 blob 롤백 시 orphan 객체 알려진 제약 명시
     - 본 회고 문서 작성 (`docs/retrospectives/cloudflare-codegen-review.md`)
