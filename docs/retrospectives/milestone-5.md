# Milestone 5 Retrospective: Identity & Permissions

## 1. 개요
Milestone 5에서는 Resource IR 스펙에 선언된 `auth.ownership_field`, `auth.permissions`(`public`, `authenticated`, `owner`, `role:<name>`) 스펙을 기반으로 사용자 세션 관리(Session), 비밀번호 타입 지원(`password`), REST Transport 5개 엔드포인트 및 View 5개 UI 지점에 일관된 권한 가딩을 완성하였습니다.

---

## 2. 해결한 주요 문제 및 보안 강화 패턴

1. **Storage 레이어로 "평문 검증 -> 해싱 -> DB 쓰기" 원자적 단일화**:
   - 기존 Transport/View 핸들러 4곳에 파편화되어 유출 가능성이 있던 비밀번호 처리 순서를 `adapters/sqlite/store.go` (`Store.Create` / `Store.Update`) 내부로 캡슐화 단일 통합하였습니다.
   - 호출부가 `store.Create` / `store.Update`를 직접 호출하더라도 평문 검증과 bcrypt 해싱이 항상 100% 원자적으로 보장되며, 60자 해시 문자열에 대한 이중 검증(`max_length` 오작동) 문제를 근본적으로 해결했습니다.
2. **401 -> 404 -> 403 3단계 가딩 순서 정립**:
   - 비인증 사용자의 경우 401 Unauthorized로 로그인 유도.
   - 인증된 사용자의 경우 존재하지 않거나 soft-deleted 된 레코드에 접근 시 403이 아닌 404 Not Found를 먼저 반환하여 타인의 레코드 존재 여부를 알아내는 정보 유출 공격(Resource Enumeration) 차단.
3. **`password` Semantic Type의 IR 통합**:
   - 평문 제약조건 검증(`min_length`, `max_length` 및 72바이트 상한 체크) -> 검증 성공 후 `bcrypt` 해싱 -> REST API/View sanitization 응답 시 비밀번호 자동 은폐(strip)를 IR 레벨에서 완전 강제.
4. **User `role` 필드의 권한 상승 (Privilege Escalation) 차단**:
   - 일반 유저(`user`)가 자기 프로필 수정 시 `role` 필드를 `admin`으로 넘겨 승격하려는 시도를 403 Forbidden으로 차단하고, `role` 필드를 포함하지 않은 일반 프로필 수정(email, username)은 정상 허용(과잉 차단 방지).
5. **Transport ↔ View 간 `auth.Can` 로직 100% 재사용**:
   - `auth.Can` 및 `auth.Evaluate` 단일 엔진을 `auth` 패키지에 구축하고, REST 백엔드와 HTML View 템플릿(버튼 렌더링/액션 가드)에서 공통 호출.

---

## 3. 실제 커밋 히스토리 (Git Log)

- `8ad9f68`: `feat(ir): add password semantic type, password hashing & response sanitization`
- `8f84ff1`: `feat(auth): implement session management, User resource schema & auth.Can evaluation engine`
- `f176ea7`: `feat(transport): apply auth middleware, 401/404/403 permission guards & role escalation protection`
- `83432ab`: `feat(view): integrate session authentication UI, login/logout & auth.Can button guards`
- `7c3d9e8`: `test(auth): add matrix test cases for permissions, privilege escalation & password hashing`
- `eb0863d`: `docs(retrospectives): add milestone-5 retrospective and update NOW.md & TASKS.md`
- `2444262`: `fix(auth): clean up unused imports in auth_test.go`
- `a6c8d2f`: `fix(auth): update TestPassword_ValidationAndHashing to pass admin session for role field writes`
- `8591922`: `fix(auth): enforce record validation before password hashing in write paths`
- `3e938d2`: `refactor(sqlite): encapsulate plain text validation & password hashing inside Store Create and Update`
