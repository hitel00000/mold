# TASKS

> 이 문서는 사전에 미리 상상한 계획이 아니라, **"실험 ➔ 관찰 ➔ 마찰 제거"**의 실증적 흐름으로 검증하는 살아있는 백로그 문서입니다.

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

> [!IMPORTANT]
> 사전 해결책을 강제하지 않으며, 실제 외부 사용 실험 과정에서 관찰된 마찰을 바탕으로 채택/기각을 판정합니다.

### [가설 1] 외부 모듈 제품성 (External Consumer) 가설
* **질문**: Mold를 완전히 독립된 외부 프로젝트에서 단 하나의 패키지로 임포트할 때 마찰이 없는가?
* **채택 조건**: 외부 프로젝트에서 Mold 패키지 1개만 임포트하고 `resources/` 경로만 넘겨주면, 아무 보일러플레이트 없이 부팅 및 서빙될 때.
* **기각 조건**: 외부 프로젝트 연동 시 내부 상태 강결합이나 불필요한 인프라 코드가 요구될 경우 (마찰 발견 시 구조 단순화 재작업).

### [가설 2] Invisible Infrastructure DX (`mold dev`) 가설
* **질문**: 소스 저장(`Ctrl + S`) 후 브라우저 새로고침만으로 백그라운드 리로드가 투명하게 체감되는가?
* **채택 조건**: 개발자가 인프라 명령어를 직접 칠 필요 없이, 소스 저장 시 원자적 리로드가 안정적으로 반영될 때.
* **기각 조건**: 수동 명령어가 더 명확하거나 워처가 비결정적 동시성 오류를 유발할 경우.

### [가설 3] Feature & Plan 계층 가설
* **질문**: DDL/API/View 전반에 걸쳐 반복되는 수직적 중복 로직이 실제로 존재하는가?
* **채택 조건**: 독립 프로젝트 구동 중 3개 이상의 Resource에서 수직적 중복 로직이 실증되고, Plan 도입 시 구조가 더 단순해질 때.
* **기각 조건**: 중복이 미미하거나 Plan 도입 시 단순 변환 코드만 늘어날 경우 (현재 단일 컴파일러 구조 유지).

---

## 3. Post-MVP 실증 백로그 (실험 ➔ 관찰 ➔ 마찰 제거)

### Phase 1: 독립 프로젝트(`drink-log`) 적용 실험 및 마찰(Friction) 제거

- [ ] **Task 1.1: [실험] 외부 프로젝트 `drink-log`에서 Mold 임포트 및 초기 부팅**
  - **실험 내용**: Mold 레포 외부(별도 디렉터리/프로젝트)에서 `drink-log`를 만들고 Mold 패키지를 불러와 실행한다.
  - **관찰 항목**: 패키지 임포트, 초기화 함수, 설정 전달 과정에서 어떤 마찰이나 불편이 발견되는가?
  - **완료 조건**: 발견된 마찰을 기록하고, 외부 프로젝트에서 Mold 엔진을 단 한 줄로 부팅 성공시킨다.
- [ ] **Task 1.2: [실험] `drink-log`에 도메인 Resource 정의 및 외부 CRUD/권한 서빙**
  - **실험 내용**: `drink-log`에 `User.yaml`, `Drink.yaml`을 추가하고 REST API 및 권한 가드를 작동시킨다.
  - **관찰 항목**: 외부 프로젝트 환경에서 스키마 생성, 로그인 세션, API 서빙 시 발생하는 문제점 관찰.
  - **완료 조건**: 외부 프로젝트에서 기본 CRUD 및 권한 가드가 오류 없이 작동함을 확인한다.
- [ ] **Task 1.3: [실험] `drink-log` 전용 Custom UI (Template Override) 서빙**
  - **실험 내용**: 기본 HTML View 대신 `drink-log` 전용 커스텀 HTML/CSS를 오버라이드해본다.
  - **관찰 항목**: 프론트엔드 이관 및 커스텀 템플릿 바인딩 과정에서 발생하는 마찰 관찰.
  - **완료 조건**: Mold 기본 View를 깨뜨리지 않고 커스텀 템플릿이 자연스럽게 우선 렌더링됨을 확인한다.

### Phase 2: 개발자 경험(DX) 관찰 및 마찰 제거

- [ ] **Task 2.1: [실험] 외부 프로젝트의 `resources/*.yaml` 변경 시 백그라운드 리로드 연결**
  - **실험 내용**: 파일 저장(`Ctrl + S`) 시 수동 재구동 없이 투명하게 컴파일 및 리로드되도록 만든다.
  - **관찰 항목**: 파일 저장과 브라우저 반영 사이의 지연, 동시성 에러, 개발자가 느끼는 마찰을 기록한다.
  - **완료 조건**: 수동 명령어 없이 파일 저장만으로 핫컴파일 반영이 마찰 없이 완료된다.

### Phase 3: 관찰된 패턴 기반으로 구조 판정 및 정리

- [ ] **Task 3.1: [관찰 및 판정] Phase 1~2 동안 기록된 마찰과 중복 코드 복기**
  - **관찰 내용**: 실제 수직적 중복 패턴이 존재하는지, Feature/Plan 계층이 진짜 필요한지 판정한다.
  - **완료 조건**: 가설 3의 채택 또는 기각을 확정하고 필요 시 최소한의 계층만 추출한다.

### Phase 4: Cloudflare Workers Static Generator 실험 (필요성 확정 시)

- [ ] **Task 4.1: [실험] `drink-log` 명세를 TypeScript + Hono + D1 코드로 생성 및 로컬 Wrangler 실행**
  - **완료 조건**: 생성된 TS+D1 코드가 로컬 Wrangler 환경에서 기존 Go API와 동일하게 반응함을 확인한다.
