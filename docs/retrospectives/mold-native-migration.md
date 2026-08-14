# Mold 네이티브 이관 회고 (Drink-Log Mold Native Migration)

## 1. 개요
본 회고는 `drink-log` 서비스의 백엔드를 기존 Cloudflare Functions/Glue Layer 방식에서 **Mold 네이티브 REST API 방식**으로 완전히 교체하고, 프론트엔드 데이터 레이어(`storage.ts`)를 그에 맞춰 리팩터링한 작업의 경위와 주요 교훈을 정리한 문서이다.

---

## 2. 방향 전환의 배경: Glue Layer 폐기와 Mold 네이티브 채택

### 기존 접근 (Glue Layer)의 문제점
- 초반 세션들에서는 기존 프론트엔드가 기대하던 옛 API 응답 포맷(`buildEntry()` 복합 객체, UUID ID 등)을 백엔드가 100% 그대로 재현하는 "Glue Layer"를 세우려고 시도하였다.
- 이 접근은 ID 재매핑, `buildEntry()` 조립, `legacy_id` 은폐 등 불필요한 번역/변환 레이어를 백엔드 및 전역 코드베이스에 영구히 남기게 되었으며, 이는 Mold의 핵심 철학인 **"반복 제거, 결정적 백엔드"**와 정면으로 배치되었다.

### 새로운 방향 (Mold Native + Frontend Adapt)
- **API 계약과 UI(Frontend Product)의 명확한 분리**:
  - 옛 API 응답 모양을 보존해야 한다는 제약을 버리고, 백엔드는 Mold 네이티브 REST API(정수 Primary Key, `{ data: ... }` envelope, 리소스별 개별 엔드포인트)를 있는 그대로 노출한다.
  - 프론트엔드의 데이터 레이어(`src/lib/storage.ts`)가 Mold API를 직접 소비하고 조인하여 `SakeRecordEntry` 형태를 구성함으로써, 기존 컴포넌트(`App.tsx`)의 UI/UX 코드는 최소한의 수정만으로 100% 완전하게 보존하였다.
- **결과**: 백엔드는 불필요한 변환 코드 0개로 극도로 단순해졌으며, 프론트엔드 데이터 레이어도 표준 REST API 호출 패턴으로 깔끔하게 정돈되었다.

---

## 3. 핵심 교훈 (Key Takeaways)

### 1) "타입체크/컴파일 성공 ≠ 작동 보장" — 실측 Integration 검증의 중요성
- 이번 세션을 통틀어 가장 값진 교훈은 **단위 테스트나 타입체크(컴파일 통과)만으로는 실제 런타임의 결함을 찾아낼 수 없다**는 점이다.
- 실제로 로컬 Mold Go HTTP 백엔드 웹서버를 띄우고 프론트엔드 TS 데이터 레이어가 직접 통신하는 실측 integration 테스트를 돌린 뒤에야 비로소 다음 결함들이 드러났다:
  - **SQLite Foreign Key RESTRICT 에러 (500 Internal Server Error)**: 부모 `SakeRecord` 삭제 시 자식 `RecordTag` 및 `SakeImage`가 존재하면 백엔드 DB에서 500 에러가 발생하는 현상을 발견하고, 프론트엔드가 자식을 먼저 명시적 `DELETE` 하도록 보완함.
  - **List API Limit 잘림 예방**: `fetchAllPages`의 `offset`/`total` 루프 세이프가드 및 무한루프 방지 구문 보완.
  - **보상 트랜잭션 (Cleanup)**: 다중 POST 생성 실패 시, 생성된 이미지/태그/부모 항목을 역순으로 명시적 DELETE 하는 롤백 로직 완비.

### 2) Resource 중심 선언적 접근과 마세라티 원칙
- `RecordTag`의 인가 보안 이슈 해결 시, 얇은 Glue 코드나 백엔드 핸들러를 추가하지 않고 Resource YAML (`RecordTag.yaml`)에 `owner_id: int` 및 `permissions: owner` (create: authenticated)를 선언하여 해결하였다.
- **알려진 제약 (Known Limitation)**:
  - `RecordTag.owner_id`가 세션 유저로 자동 채워지므로 "유저 A가 유저 B의 사케 ID에 태그를 부착하는" 부모 릴레이션 교차 무결성 리스크가 존재한다.
  - 하지만 본 프로젝트의 포지셔닝(개인용 프로토타입/소규모 서비스)과 **마세라티 원칙(아직 발생하지 않은 문제로 코드를 복잡하게 만들지 않는다)**에 따라, 백엔드에 과도한 커스텀 교차 검증을 넣지 않고 본 리스크는 감수하고 단순한 구조를 유지하기로 결정하였다.

---

## 4. 최종 결과물 요약
1. **RecordTag.yaml**: `owner_id` 및 `permissions: owner` 추가로 세션 사용자 인가 보호.
2. **sake.ts**: Mold 네이티브 정수 ID (`number | string`) 및 `MoldResponse<T>` 타입 수용.
3. **storage.ts**: Mold 네이티브 REST API 호출, All-Fetch 루프 페이징, FK RESTRICT 안전 삭제, Client-side 보상 트랜잭션 롤백 완비.
4. **App.tsx**: 정수 ID 호환 및 기존 UI/UX 100% 보존.
5. **Real HTTP E2E Test**: Mold Go Runtime HTTP 백엔드 + 프론트엔드 TS 데이터 레이어 간 실제 통신 및 생애주기 검증 완료 (Pass).
