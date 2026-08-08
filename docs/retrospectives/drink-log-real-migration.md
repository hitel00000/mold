# Retrospective: drink-log 실제 프로덕션 이식 및 가상 스키마 분석 회고

> **작성일**: 2026-08-08  
> **관련 마일스톤**: Task 6.1 `drink-log` 실제 프로덕션 이관  
> **핵심 주제**: 가상 스키마 상상 기반 사전 분석(Rev 1~10)과 실제 코드 실측 간 괴리 원인 규명 및 마세라티 원칙 재확인

---

## 1. 개요

`drink-log` 서비스의 Mold Resource 기반 재작성 및 D1/R2 이관 작업을 진행하면서, 기존 문서(`docs/tasks/drink-log-migration-analysis.md` Revision 1~10)가 전제했던 스키마 및 삭제 오케스트레이션 설계와 실제 `drink-log` 소스 코드(`docs/schema.sql`, `functions/`) 간에 다수의 중대한 불일치가 존재함이 실측을 통해 밝혀졌다.

본 회고 문서에서는 사전 분석과 실측 간의 주요 차이점, "실제 코드 접근 전 정교한 설계를 10차례 거듭했던" 구조적 원인, 그리고 실제 프로덕션 이관 구현 및 검증 결과를 기록한다.

---

## 2. 기존 사전 분석(Rev 1~10) vs 실제 코드(실측) 주요 차이점

| 항목 | 사전 분석(Rev 1~10) 가상 전제 | 실제 코드 (실측) | 귀결 및 조치 |
| :--- | :--- | :--- | :--- |
| **`users.id`** | 랜덤 UUID (정수 이관 대상) | **`"google:" + provider_sub`** (결정적 자연키 문자열) | `User.id`는 정수 이관 대상에서 제외, `"google:<sub>"` 문자열 유지 |
| **`tags.id` (기본 22개)** | 랜덤 UUID이므로 별도 `slug` 필드 신설 필수 (Rev 6~7 결론) | **이미 고정 슬러그가 PK 자체** (`tag_taste_fresh` 등) | `slug` 필드 신설 불필요. `INSERT OR IGNORE`로 100% 멱등 유지 |
| **FK `ON DELETE` 정책** | 전부 `RESTRICT` 가정 → 복잡한 Delete Orchestration 설계 | **전부 `ON DELETE CASCADE`** | DB Cascade delete + R2 객체 일괄 삭제로 극도로 단순화 |
| **`SakeRecord` 필드** | `notes`, `rating` 존재 가정 (파일럿 YAML 포함) | **실제 스키마에 `notes`, `rating` 컬럼 없음** | 해당 필드 제거 (`docs/schema.sql`과 1:1 일치) |
| **API 응답 구조** | Mold 표준 envelope (`{data: ...}`) | **Envelope 없는 Raw JSON 복합 객체** (`buildEntry()`) | Hono glue 라우트에서 기존 Raw JSON 응답 모양 100% 보존 |
| **태그 중복 비교** | SQL 대소문자 구분 비교 | **`toLowerCase()` 기반 대소문자 무시** | Hono glue 코드에서 대소문자 무시 dedup 재현 |
| **데이터 규모** | 대량 실사용자 무손실 이관 전제 | **실사용자 1명, 레코드 100개 미만** | 과잉 설계(Abort/Retry 세션 오케스트레이션) 배제 |

---

## 3. 원인 분석 (Root Cause Analysis)

### 3.1 왜 소스 코드 확인 없이 10차례나 정교한 설계를 반복했는가?

1. **상상 기반 설계의 함정**: 소스 코드를 직접 열어보지 않고 추론이나 이전 기억만으로 "아마 DB 스키마가 이렇게 생겼을 것이다", "ON DELETE RESTRICT일 것이다"라는 가정을 쌓아 올렸다.
2. **복잡성 선호 비타협적 원칙 위반**: 발생하지도 않은 대규모 마이그레이션 실패 시나리오, Abort/Retry 계약, 복잡한 HTTP 셀프콜 Delete Orchestration 등 "마세라티 원칙(아직 발생하지 않은 문제를 미리 해결하지 않는다)"에 위배되는 과잉 설계를 지속적으로 정교화했다.
3. **실측 우선 원칙 결여**: "코드를 읽거나 실행해서 확인한다"는 가장 기본적인 검증 단계보다 "이론적 완전성"을 우선시하여 드리프트가 누적되었다.

---

## 4. 실제 이관 구현 및 검증 결과

### 4.1 스키마 및 Resource YAML 1:1 대조
- `User`, `SakeRecord`, `SakeImage`, `Tag`, `RecordTag` 5개 Resource YAML을 `docs/schema.sql` 컬럼과 1:1 정확히 일치시켰다.
- `SakeRecord`에서 가상 필드(`notes`, `rating`)를 제거하고, `Tag`에서 불필요한 `slug` 필드를 제외했다.
- `User.id`는 문자열 PK를 유지하고 `auth.ownership_field: id` 특수 케이스로 처리했다.

### 4.2 D1 마이그레이션 SQL 스크립트 (`0001_drink_log_migration.sql`)
- `sake_records`, `sake_images`, `record_tags`를 `INTEGER PRIMARY KEY AUTOINCREMENT` + `legacy_id TEXT UNIQUE` 구조로 변환했다.
- `users.id` 및 `tags.id` (기본 22개 및 커스텀)는 기존 문자열 PK를 온전히 유지했다.
- R2 버킷 객체 키 경로(`images/...`, `thumbnails/...`)는 재작성 없이 100% 보존되었다.

### 4.3 Miniflare E2E 검증 결과
- **로컬 V8 Isolate 통합 테스트 통과**:
  - `users` 및 기본 태그 PK 무변경 및 idempotency 검증 완료.
  - 정수 PK 이관 및 `legacy_id` 보존 검증 완료.
  - Raw JSON 응답 구조(`buildEntry()`) 및 `/api/sake-records`, `/api/tags` 엔드포인트 계약 보존 검증 완료.
  - `ON DELETE CASCADE` 및 R2 객체 삭제 동작 검증 완료.
  - 커스텀 태그 대소문자 무시(`toLowerCase()`) 중복 거부 동작 검증 완료.
- **Go test 회귀 검증**: `go test ./...` 전체 PASS.

---

## 5. 실 프로덕션 D1 적용 시 운영 가이드라인

실제 Remote D1(`--remote`)에 마이그레이션을 적용할 때는 아래 안전 절차를 **반드시** 준수해야 한다:

1. **최우선 백업**:
   ```bash
   npx wrangler d1 export alcohol_log --remote --output=./backup_before_migration.sql
   ```
2. **사람의 명시적 승인 획득**: AI 에이전트는 로컬 Miniflare 검증 완료 결과를 사람에게 보고하고, 실 D1 실행 여부에 대한 명시적 확답을 받은 후에만 실행 명령을 제시해야 한다.

---

## 6. 결론

"실제 소스 코드가 유일한 참(Single Source of Truth)이다."  
추측에 근거한 정교한 문서 작성보다, 실제 코드를 1줄이라도 읽고 1번이라도 실행하여 검증하는 것이 월등히 안전하고 효율적임을 본 마이그레이션을 통해 다시 한번 입증하였다.
