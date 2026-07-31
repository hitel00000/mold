# drink-log 이관 분석 및 구현 회고 (Milestone 6 / Phase 6)

> 이 문서는 `docs/tasks/drink-log-migration-analysis.md` Revision 1~10 분석 명세와 4차례의 구현/검증 라운드를 거쳐 `drink-log` 프로덕션 마이그레이션을 완수한 후 남기는 회고 문서입니다. 대규모 이관 과정에서 관찰된 4대 실패 패턴과 교훈을 기록하여 향후 마이그레이션 작업에 적용합니다.

---

## 1. 개요

`drink-log` 프로젝트는 기존 Cloudflare Pages + D1 (SQLite) + R2 환경에서 동작하던 실서비스 사케 기록 앱이다. UUID PK 기반 스키마와 손수 작성된 Glue 코드를 Mold 명세 기반 런타임/Codegen 시스템으로 마이그레이션하면서 10차례의 분석 리비전과 4차례의 구현 및 자가 검증 사이클이 수행되었다.

---

## 2. 반복 관찰된 4대 문제 패턴과 교훈

### 1) 인증 우회 폴백(Authentication Bypass Fallback)의 재발 경위
- **상황**: 
  - 과거 Cloudflare TS Codegen 초기 구현 시 `x-user-id`/`x-user-role` 헤더를 검증 없이 신뢰하는 취약점이 발견되어 커밋 `7e7e59b`로 완전히 제거했었다.
  - 그러나 이번 `orchestrate-delete.ts` 구현 시, Windows Miniflare 환경 소켓 연결 거부(`ConnectEx #1225`) 오류가 발생하자 `catch` 블록에서 `env.DB`/`env.BUCKET`으로 직접 접근해 삭제를 처리하는 임시 폴백 코드가 다시 작성되었다.
- **원인 및 성찰**:
  - 로컬 테스트 하네스 환경의 소켓 거부 문제를 프로덕션 런타임 레벨에서 임시 방어 조치(덕트 테이프)로 때우려 하면서, 명시적으로 수립했던 세션 기반 HTTP 인증 경계를 우회하는 보안 구멍을 다시 만들어냈다.
- **교훈 및 조치**:
  - **테스트 환경의 문제는 100% 테스트 환경(하네스)에서 해결해야 한다.** 프로덕션 코드에 인증/인가 가드를 우회하는 어떠한 폴백도 도입해서는 안 된다. Hono 인메모리 파이프라인(`app.request`)으로 하네스를 개선하여 소켓 노이즈를 근본 제거하고, 프로덕션 소스에서는 폴백을 0% 완벽히 제거하였다.

---

### 2) "Seed는 1회만 실행된다" 미검증 가정이 실측으로 반증된 사례
- **상황**: 
  - 초기 이관 분석에서는 기본 태그 22개 시드에 대해 "seed는 1회만 구동되므로 unique index나 자연키 없이도 재실행에 안전하다"고 가정하였다.
- **원인 및 실측 반증**:
  - 운영 서버 배포, CI/CD 재배포, 혹은 롤백 후 재시드 시나리오에서 시드 스크립트가 2회 이상 구동되면 중복 기본 태그 22개가 그대로 추가 삽입되는 심각한 시드 idempotency 붕괴가 관찰되었다.
- **교훈 및 조치**:
  - **"1회만 실행된다"는 가정은 존재하지 않으며, 모든 시드/마이그레이션 스크립트는 Idempotent 해야 한다.**
  - `Tag` 리소스에 고정 자연키 식별자인 `slug` 필드(`type: string, unique: true, nullable: true`)를 도입하고 `INSERT OR IGNORE INTO tags (...)` 구문으로 수렴시켜, 1차 및 2차 운영 재실행 시에도 0 duplicates (정확히 22개 유지) 100% Idempotency를 실측 검증하였다.

---

### 3) R2 키 독립성 실측 미비로 인한 잘못된 1차 분석 오류
- **상황**:
  - 1차 이관 분석에서는 UUID PK에서 INTEGER AUTOINCREMENT PK로 전환함에 따라 R2에 저장된 기존 이미지 객체 키(`images/{user_id}/sake/{record_id}/img1.jpg`)도 새로운 INTEGER `record_id`로 마이그레이션/재작성되어야 한다고 추정하였다.
- **실측으로 밝혀진 사실**:
  - 실제 프로덕션 DB 및 R2를 실측 점검한 결과, DB 테이블의 `image_key` 컬럼은 단순한 문자열 포인터(String Pointer)일 뿐이었으며, R2 키 구조는 DB PK 식별자 유무와 독립적이었다.
- **교훈**:
  - 추측으로 마이그레이션 범위를 부풀리지 말고, **반드시 실제 데이터 및 컬럼 참조 관계를 실측(Empirical Verification)하고 시작해야 한다.** R2 키 재작성 없이 기존 `image_key` 값을 100% 보존함으로써 이관 복잡도를 획기적으로 낮추었다.

---

### 4) Cloudflare Target D1 DDL `FOREIGN KEY ... ON DELETE RESTRICT` 누락 버그 발굴과 독자 커밋 분리
- **상황**:
  - `drink-log` 마이그레이션 분석 중, Mold의 Cloudflare TS Codegen Target이 관계형 외래키 생성 시 `ON DELETE RESTRICT` DDL 구문을 생성하지 않고 있었음이 발견되었다.
- **조치 및 처리 원칙**:
  - 이 결함은 `drink-log` 이관 스코프 내의 코드가 아니라 Mold 백엔드 Codegen Target 자체에 존재하던 기존 패리티 버그였다.
  - 마이그레이션 작업 커밋에 조용히 섞어 넣지 않고, `fix(codegen/cloudflare): enforce on_delete restrict at D1 DDL level` (`9d74c02`) 독립 커밋으로 분리 작성하고 `TASKS.md`에 독립 항목으로 기록하였다.

---

## 3. 최종 결론

`drink-log` 마이그레이션은 단순한 스크립트 생성을 넘어, Mold 명세 기반 런타임의 프로덕션 패리티와 삭제 오케스트레이션 계약, 그리고 스키마/시드 Idempotency 체계를 전반적으로 정밀 검증한 이정표가 되었다. 회고에서 밝혀진 규칙들을 향후 마이그레이션 백로그에 철저히 적용한다.
