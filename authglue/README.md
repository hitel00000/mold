# AuthGlue: Mold 애플리케이션 레이어 Auth 패키지

`authglue`는 Mold 코어 런타임(`runtime.App`) 위에서 동작하는 얇은 애플리케이션 레이어 가입 및 OAuth 세션 발급 패키지입니다.

---

## 제공 핸들러

1. **`SignupHandler(app *runtime.App)`**
   - `POST /signup` 가입 핸들러.
   - `email`, `password`, `name` 필드만 화이트리스트 추출하여 레코드 생성.
   - `role` 필드의 `client_writable: false` 제약조건을 준수하여 권한 상승 공격 차단 (`400 CLIENT_WRITE_FORBIDDEN`).
   - `app.SanitizeRecord`로 비밀번호 해시 노출 차단.
   - `app.IssueSessionForUser`로 세션 쿠키 (`_mold_session`) 즉시 발급.

2. **`OAuthCallbackHandler(app *runtime.App, providerName string, verifier OAuthVerifier)`**
   - OAuth 콜백 핸들러 (예: `/auth/google/callback`).
   - 검증된 `OAuthUser`를 받아 `provider` + `provider_user_id` 기준 find-or-create 수행.
   - 세션 쿠키 발급.

---

## 알려진 제약 사항 (Known Constraints)

1. **이메일 소유권 미검증에 따른 계정 스쿼팅 (Account Squatting)**
   - `authglue`는 이메일 발송 및 소유권 확인 토큰 서버를 내장하지 않는 얇은 애플리케이션 레이어입니다.
   - 따라서 `/signup` 시 제출된 이메일의 실제 소유 여부를 검증하지 않으므로, 공격자가 타인의 실재 이메일로 먼저 회원가입하는 계정 스쿼팅(Account Squatting)이 이론상 발생할 수 있습니다.
   - 타인의 이메일로 먼저 가입된 경우 해당 이메일의 향후 OAuth 가입은 `409 ACCOUNT_LINKING_REQUIRED`로 명시적 거부됩니다.
   - 완전한 스쿼팅 방지를 위해서는 상위 애플리케이션에 이메일 소유권 검증(이메일 인증 토큰 발송 및 확인) 플로우를 도입해야 하며, 이는 현 스코프의 알려진 제약사항으로 남깁니다.

2. **검증되지 않은 자동 계정 연동 차단 (Unverified Auto-Linking Prevention)**
   - 이메일/비밀번호로 먼저 가입된 미검증 계정에 대해 동일 이메일의 OAuth 로그인 시 자동 연동(Auto-merge)하지 않고 HTTP 409 `ACCOUNT_LINKING_REQUIRED`로 거부하여 Trojan Horse Account / Pre-Account Hijacking 공격을 차단합니다.
