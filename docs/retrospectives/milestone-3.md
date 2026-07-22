# Milestone 3 (Transport) 회고

> 이 문서는 Mold Milestone 3 (Transport / REST API) 구현 과정에서 반영된 핵심 라우팅 설계, 검증 레이어 통합, 피드백 반영 사항 및 향후 마일스톤(Default View, Auth 등)에 적용할 지침을 정리한다.

---

## 1. 개요

Milestone 3에서는 Resource IR과 Storage 레이어를 노출하는 REST API 런타임(`transport`)을 구축하였다.
단일 Wildcard 동적 라우터와 `atomic.Pointer[Registry]` 기반의 스냅샷 스왑 구조를 적용하여, `POST /_mold/reload` 시 프로세스 재시작 없이 원자적으로 최신 IR 및 스키마 변경 사항을 라우터에 반영할 수 있도록 완성하였다.

---

## 2. 주요 구현 및 반영 사항

### 1) System Column 검증 순서 및 IR 설정 분기
- `resource.ValidateRecord`에서 `r.Timestamps` 및 `r.SoftDelete` 설정에 맞춰 system column(`created_at`, `updated_at`, `deleted_at`)의 거부 규칙을 동적으로 세분화함.
- `timestamps: true`일 때 `created_at`, `updated_at` 입력 시 "system column" 400 거부.
- `soft_delete: true`일 때 `deleted_at` 입력 시 "system column" 400 거부. (`soft_delete: false`이면 "unknown field" 400 거부).
- 단위 테스트(`TestValidateRecord_SystemColumnRejection`)로 예외 분기 검증 완료.

### 2) Deprecated 필드 응답 Sanitization
- `SanitizeRecord` 및 `SanitizeRecordList` 헬퍼 함수를 추가하여 `Get`, `List`, `Create`, `Update` 4개 모든 API 응답 출력 직전에 IR의 `deprecated: true` 필드가 맵에서 완전히 제외되도록 구현함.

### 3) DB Foreign Key 무결성 에러 실측 매핑
- `modernc.org/sqlite` 드라이버 사용 환경에서 `?_pragma=foreign_keys(1)` 옵션을 적용하고, 존재하지 않는 FK 참조 입력 시 실측 에러 문자열(`FOREIGN KEY constraint failed`)을 포획하여 `400 Bad Request` (`code: INVALID_FOREIGN_KEY`)로 구조화된 에러 응답을 반환하도록 매핑함.

### 4) Atomic Registry Swap 및 Dynamic Router
- `atomic.Pointer[Registry]` 기반으로 단일 wildcard 동적 라우터(`/api/{table}`, `/api/{table}/{id}`)를 구현함.
- `POST /_mold/reload` 호출 시 파싱/검증 성공 시점에 atomic pointer swap으로 최신 IR을 원자적 교체함.

---

## 3. 알려진 제약 및 다음 마일스톤(Milestone 4, 5) 적용 체크리스트

1. **[Auth 무인증 상태]**
   - Milestone 5(Identity / Auth) 착수 전까지 모든 REST API는 완전히 무인증(Unauthenticated) 상태로 공개되어 동작한다.
2. **[Soft-Delete와 DB FK 제약의 상호작용]**
   - SQLite의 DB `FOREIGN KEY` 제약은 `DELETE` 구문에만 동작하며, `soft_delete: true`로 인한 `UPDATE`(`deleted_at` 마킹) 시에는 DB FK 제약이 개입하지 않는다.
   - 따라서 `on_delete: restrict` / `soft_cascade`와 같은 레포지토리/연관관계 삭제 정책은 Milestone 5 또는 연관관계 심화 구현 시 애플리케이션 레벨에서 별도로 처리되어야 함을 확인하였다.
3. **[Reload 중 In-flight 요청 스왑 경계]**
   - Destructive migration이 진행되는 동안 실행 중인 in-flight HTTP 요청이 atomic swap 경계에 걸릴 경우의 원자적 격리는 마세라티 원칙에 따라 프로토타이핑 단계에서는 별도 복잡성을 도입하지 않고 알려진 제약으로 유지한다.

---

## 4. 커밋 요약

- `feat(resource): refine ValidateRecord system column rules & test cases`
- `fix(repo): remove accidental binary mold.zip artifact`
- `feat(transport): implement REST API router, handlers, dynamic lookup & FK error mapping`
- `docs(retrospectives): add milestone-3 retrospective`
