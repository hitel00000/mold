# Retrospective: Thin Auth Glue Layer (`authglue`)

> **시작일**: 2026-08-06  
> **완료일**: 2026-08-07  
> **주요 기능**: 얇은 Auth 레이어 (`authglue` 패키지: `/signup` 가입 핸들러 및 `/auth/google/callback` OAuth 핸들러) 추가 및 세션 발급  
> **핵심 제약**: Mold 코어 패키지(`resource/`, `runtime/`, `auth/`, `transport/`, `view/`, `codegen/`) 변경 **0줄**

---

## 1. 개요 및 배경

Mold는 특정 OAuth Provider(Google 등)와 직접 통신하는 비즈니스 코드를 코어 런타임에 내장하지 않는다 (`docs/ir-spec.md` 5절, 특정 벤더 종속성 회피 원칙).

대신 `runtime.App`이 제공하는 4가지 최소한의 escape hatch(`CreateRecord`, `IssueSessionForUser`, `SessionUser`, `SanitizeRecord`)와 `app.Store()` 접근자를 조합하여 애플리케이션 레이어(`authglue`)에서 가입 및 OAuth 세션 발급을 완전하게 처리할 수 있음을 검증했다.

---

## 2. 주요 보안 검증 및 문제 해결 패턴

### ① Pre-Account Takeover 차단 (Payload Whitelisting)
- **문제**: 클라이언트가 `/signup` 호출 시 `provider`, `provider_user_id`를 임의로 제출하여 특정 OAuth 사용자 계정을 사전 선점/연동하려는 시도.
- **해결**: `/signup` 핸들러에서 payload 수신 시 `email`, `password`, `name`, `role`만 화이트리스트 추출하고 `provider`/`provider_user_id`는 원천 무시하여 차단.

### ② Client-Writable 규칙 준수를 통한 권한 상승 차단
- **문제**: `role` 필드를 핸들러에서 그냥 제거(silent ignore)하면, 클라이언트가 `role: "admin"`으로 가입 시도 시 에러 없이 `200 OK` (role="user")로 응답하게 되어 Mold의 "silent ignore보다 explicit reject" 원칙 위배.
- **해결**: `role` 필드가 제출된 경우 `payload`에 포함시켜 `app.CreateRecord`로 전달하여, `User.yaml`의 `client_writable: false` 규칙에 의해 `resource.ErrClientWriteForbidden` -> `400 CLIENT_WRITE_FORBIDDEN`으로 명시적 거부되도록 보장.

### ③ Pre-Account Hijacking (Trojan Horse Account) 차단 및 이메일 자동 연동 거부
- **문제**: 이메일 소유권이 검증되지 않은 로컬 가입 계정이 존재하는 상태에서, 동일 이메일의 OAuth 로그인 시 계정을 자동 병합(Auto-merge)하면 공격자가 미리 설정해둔 비밀번호로 OAuth 계정까지 탈취할 수 있는 취약점 (Classic Federation Merge Attack / Pre-Account Hijacking).
- **해결**: 이메일이 일치하는 기존 로컬 계정이 발견될 경우 검증되지 않은 자동 연동을 원천 거부하고 `409 ACCOUNT_LINKING_REQUIRED` ("an account with this email already exists; please log in with email and password")로 명시적 거부 (옵션 a 선택).

### ④ OAuth Provider 충돌 에러 안내 분기
- **문제**: 기존 계정이 이메일/비밀번호가 아니라 다른 OAuth Provider(예: GitHub)로 가입된 계정인 경우, "비밀번호로 로그인하세요"라는 오해의 여지가 있는 안내 응답 발생.
- **해결**: 기존 레코드의 `provider` 필드 상태에 따라 `ACCOUNT_LINKING_REQUIRED` vs `OAUTH_PROVIDER_CONFLICT` (`"email is already registered with provider 'github'"`)로 분기 처리.

### ⑤ 계정 스쿼팅 (Account Squatting) Known Constraint 문서화
- **상황**: 얇은 Auth 레이어 특성상 이메일 발송/토큰 검증 서버를 포함하지 않으므로, 공격자가 타인의 이메일로 먼저 가입하여 OAuth 가입을 막는 스쿼팅이 이론상 가능함.
- **해결**: 이를 묻어두지 않고 `authglue/README.md`에 알려진 제약 사항(Known Constraint)으로 명확히 기록.

---

## 3. 핵심 학습 및 다음 작업을 위한 체크리스트

1. **편의 기능 추가 시 조합 검증 필수**:
   - 단독 기능(Pre-account takeover 방지, Unique email 제약)은 안전하더라도, 편의 기능(이메일 기반 계정 자동 연동)이 조합되면 Pre-Account Hijacking 같은 새로운 취약점이 발생할 수 있다.
2. **어댑터 레벨 에러 센티널 확인 필수**:
   - `storage.ErrAlreadyExists` 센티널이 존재하더라도 어댑터(`sqlite`)에서 실제 반환 경로가 연결되어 있는지 `grep`과 구현 확인이 필요하다.
3. **Soft-delete와 Unique 제약 조합 검증**:
   - `soft_delete: true` 환경에서 partial unique index(`WHERE deleted_at IS NULL`)가 재가입 시 정상 동작하는지 E2E 테스트로 검증할 것.
