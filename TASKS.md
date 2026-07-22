# TASKS

> 이 문서는 현재 진행 중인 작업과 검증해야 할 가설(Hypotheses)을 관리하는 살아있는 백로그 문서입니다.

---

## 1. 현재 완료된 상태 (MVP 100% 완결)

* [x] **Milestone 0. 철학 고정**: 명세 기반 런타임, Non-goals 및 MVP 범위 수립
* [x] **Milestone 1. Resource**: Resource Schema, Primitive Type, Loader & Registry 구현
* [x] **Milestone 2. Storage**: SQLite Adapter, Schema ➔ DDL 자동 생성, Destructive Migration
* [x] **Milestone 3. Transport**: Dynamic Wildcard REST API (`/api/{table}`), 원자적 Reload (`POST /_mold/reload`)
* [x] **Milestone 4. Default View**: List/Detail View 및 Form SSR 렌더링, XSS Sanitization
* [x] **Milestone 5. Identity & Security**: SQLite Session, bcrypt 비밀번호 해싱, 3단계 ACL Engine (`auth.Can`)
* [x] **Milestone 6. AI Workflow**: `resource-guide.md`, `AGENTS.md`, Go 코드 수정 0줄 기반 Pure YAML Reload E2E 검증 완료

---

## 2. 검증해야 할 가설 (Hypotheses)

> [!NOTE]
> 아래 항목들은 아직 확정된 설계가 아니며, 실제 서비스(Drink Log)를 포팅하고 구축하면서 필요성을 검증해나갈 가설들입니다.

### [가설 1] Feature & Plan 계층 가설
* **내용**: IR(도메인 의미)과 Target(플랫폼 이행체) 사이에 수직적 기능 모듈(`Feature`)과 실행 계획(`Plan`)을 별도로 두어, Target을 "멍청한 이행체(Dumb Target)"로 남겨둘 것인가?
* **검증 방법**: Drink Log 제작 중 DDL/API/View 전반에 걸친 반복 패턴이 실증되는지 관찰.

### [가설 2] Invisible Infrastructure DX 가설
* **내용**: 개발자가 `generate`나 `sync` 명령을 의식하지 않고 `mold dev` 하나로 `Ctrl + S` 파이프라인을 구동하는 투명한 컴파일러 경험이 최상인가?
* **검증 방법**: 실제 로컬 개발 동선에서 CLI 입력 최소화 여부 및 핫 컴파일 피드백 속도 검증.

### [가설 3] Static Target Generator (Cloudflare Workers) 가설
* **내용**: 런타임 자체를 JS로 재작성하지 않고, Go Core가 Canonical IR을 읽어 TypeScript + Hono + D1 코드를 생성(Emit)하는 형태가 최선인가?
* **검증 방법**: Drink Log 스키마를 Cloudflare Workers용 Artifact로 내보내어 D1 및 Edge V8 서빙 검증.

---

## 3. Post-MVP 진행 순서 (우선순위)

아키텍처를 먼저 구축하려 하지 않고, **실제 서비스를 만들며 검증된 추상화를 발견하는 작업**을 최우선으로 진행합니다.

- [ ] **Task 1: 별도 프로젝트(`Drink Log`)에서 Mold 사용하기**
  - [ ] `Drink`, `Review`, `Category`, `User` Resource YAML 작성
  - [ ] 실사용 시 발생하는 프론트엔드 커스텀 및 비즈니스 유스케이스 검증
  - [ ] 실증 과정에서 반복되는 레이어 패턴 및 가설 1(Feature/Plan) 검증

- [ ] **Task 2: `mold dev` 중심의 투명한 개발자 경험(DX) 다듬기**
  - [ ] 파일 저장 ➔ 백그라운드 컴파일 ➔ 원자적 인메모리 스왑 ➔ 브라우저 즉시 반영 핫컴파일 연결 (가설 2 검증)

- [ ] **Task 3: CLI 구조 정비**
  - [ ] `cmd/mold` 엔트리포인트 정비 (`mold dev` 및 `mold build` 인터페이스 준비)

- [ ] **Task 4: Cloudflare Workers Generator PoC (가설 3 검증)**
  - [ ] IR ➔ TypeScript + Hono + D1 `schema.sql` 코드 생성기 실험
