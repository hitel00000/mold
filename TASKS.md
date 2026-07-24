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
* **최종 판정**: **[기각 (Rejected)]** (현 형태 기준 - 단일 패키지 미지원 및 ~50줄 보일러플레이트 실증. 상세 근거는 [Phase 1 회고](docs/retrospectives/phase1-retrospective.md) 참조)

### [가설 2] Invisible Infrastructure DX (`mold dev`) 가설
* **질문**: 소스 저장(`Ctrl + S`) 후 브라우저 새로고침만으로 백그라운드 리로드가 투명하게 체감되는가?
* **채택 조건**: 개발자가 인프라 명령어를 직접 칠 필요 없이, 소스 저장 시 원자적 리로드가 안정적으로 반영될 때.
* **기각 조건**: 수동 명령어가 더 명확하거나 워처가 비결정적 동시성 오류를 유발할 경우.
* **최종 판정**: **[판정 불가 (Pending)]** (Phase 2 진행 후 판정 예정)

### [가설 3] Feature & Plan 계층 가설
* **질문**: DDL/API/View 전반에 걸쳐 반복되는 수직적 중복 로직이 실제로 존재하는가?
* **채택 조건**: 독립 프로젝트 구동 중 3개 이상의 Resource에서 수직적 중복 로직이 실증되고, Plan 도입 시 구조가 더 단순해질 때.
* **기각 조건**: 중복이 미미하거나 Plan 도입 시 단순 변환 코드만 늘어날 경우 (현재 단일 컴파일러 구조 유지).
* **최종 판정**: **[채택 (Adopted)]** (본체 코드 5개 패키지 내 `switch f.Type` 6개 지점 및 필드 루프 11개 지점 등 수직적 중복 실증. 단, 구현 착수는 마세라티 원칙에 따라 Phase 4 멀티 타깃 시점으로 보류. 상세 근거는 [Phase 1 회고](docs/retrospectives/phase1-retrospective.md) 참조)

---

## 3. Post-MVP 실증 백로그 (실험 ➔ 관찰 ➔ 마찰 제거)

### Phase 1: 독립 프로젝트(`drink-log`) 적용 실험 및 마찰(Friction) 제거

- [x] **Task 1.1: [실험] 외부 프로젝트 `drink-log`에서 Mold 임포트 및 초기 부팅**
  - **실험 내용**: Mold 레포 외부(별도 디렉터리/프로젝트)에서 `drink-log`를 만들고 Mold 패키지를 불러와 실행한다.
  - **관찰 항목**: 패키지 임포트, 초기화 함수, 설정 전달 과정에서 어떤 마찰이나 불편이 발견되는가?
  - **완료 조건**: 발견된 마찰을 기록하고, 외부 프로젝트에서 Mold 엔진을 단 한 줄로 부팅 성공시킨다.
  - **Task 1.1 완료 메모 (마찰 기록 및 가설 1 중간 상태)**:
    - **발견된 4가지 마찰**:
      1. *임포트 단일 진입점 부재*: 단일 `runtime` 패키지가 없어 `adapters/sqlite`, `auth`, `resource`, `transport`, `view` 5개 서브 패키지를 개별 파악 및 임포트해야 함.
      2. *조립 보일러플레이트 약 50줄*: DB 개설, 세션 관리자, IR 로드, DDL 생성, Router/ViewHandler 조립 및 리로드 콜백 작성에 ~50줄의 코드가 요구됨.
      3. *Config 구조체 부재*: DB 경로, Resource 경로, 포트 번호 등을 일괄 전달하는 통합 설정 객체가 없음.
      4. *의존성 해석 수동 개입*: 모듈 replace 지시어 지정 후 간접 의존성을 `go mod edit -require` 및 `go.sum`으로 수동 동기화해야 했음.
    - **가설 1(외부 모듈 제품성) 판정 상태**: 관찰된 마찰로 보아 현재 상태는 "단 한 줄 부팅" 채택 조건과 거리가 있으나, 조기 판정하지 않고 Task 1.2까지 마친 뒤 Phase 3에서 최종 판정(구조 단순화/개선)을 확정함.
- [x] **Task 1.2: [실험] `drink-log`에 도메인 Resource 정의 및 외부 CRUD/권한 서빙**
  - **실험 내용**: `drink-log`에 `User.yaml`, `Drink.yaml`을 추가하고 REST API 및 권한 가드를 작동시킨다.
  - **관찰 항목**: 외부 프로젝트 환경에서 스키마 생성, 로그인 세션, API 서빙 시 발생하는 문제점 관찰.
  - **완료 조건**: 외부 프로젝트에서 기본 CRUD 및 권한 가드가 오류 없이 작동함을 확인한다.
  - **Task 1.2 완료 메모 (관찰 결과 및 마찰 재평가)**:
    - **검증 완료 항목 (마찰 0건)**: FK 스키마 자동 생성, 로그인 세션 쿠키 발급, 401➔404➔403 3단계 가딩 순서, FK 무결성 위반 차단(`INVALID_FOREIGN_KEY`), role 권한 상승 차단(`ErrPrivilegeEscalation`) 실측 성공 (`go test -v -count=1` fresh PASS).
    - **보일러플레이트 마찰 반증 데이터**: Resource 개수가 1개(`Post`)에서 3개(`Post`, `User`, `Drink`)로 늘어나더라도 `resource.LoadAll` 동적 탐색 덕분에 `main.go` 조립 보일러플레이트(~50줄)는 0줄 증가($O(1)$ 상수 유지)함 (Task 1.1 마찰 #2가 리소스 증가에 비례하여 악화되지 않는다는 반증).
    - **새로 관찰된 파편화 데이터 포인트**: 외부 프로젝트(`drink-log`)에서 에러 응답 디코딩을 위해 `transport.ErrorEnvelope` 구조체를 직접 임포트해서 사용하고 있음. 이는 Task 1.1 마찰 #1(단일 entrypoint 부재)과 동일한 계열의 파편화 데이터 포인트임.
    - **가설 1(외부 모듈 제품성) 판정 상태**: 조기 확정짓지 않고 Phase 1의 나머지 실험(Task 1.2.5, Task 1.3)까지 마친 뒤 Phase 3에서 종합 판정함.
- [x] **Task 1.2.5: [실험] Blob Storage(R2) 갭 분석 및 `blob` type 초안 검증**
  - **배경**: 실제 배포된 사케 앱(`docs/schema.sql`)은 이미지 바이트를 R2에,
    key만 D1에 저장하는 구조다. 현재 IR 스펙(`docs/ir-spec.md`)엔 이 패턴이
    없어서, Mold를 사케 앱 같은 실서비스에 적용하려면 이 갭을 먼저 메워야 한다.
  - **실험 내용**: drink-log에 이미지 필드가 있는 Resource(예: `Drink`가
    `has_many` `DrinkImage`, `blob` 필드 보유)를 추가하고, `docs/ir-spec.md`
    5.5절 초안대로 `storage.BlobStore` 인터페이스를 구현해서 업로드/조회/삭제 및 1-Step 멀티파트 생성을 검증한다.
  - **관찰 항목**: `Store`/`BlobStore` 책임 분리가 실제로 깔끔하게 되는가?
    reload가 blob 필드가 있는 Resource를 스키마 변경 없이 잘 처리하는가?
    권한 가드(`auth.permissions`)가 서브 엔드포인트에도 자연스럽게
    적용되는가, 아니면 별도 규칙이 필요한가?
  - **완료 조건**: `docs/ir-spec.md` 5.5절의 [미결정 사항 3가지](docs/ir-spec.md#결정된-사항-task-125-확정)(인터페이스 시그니처, key 발급 규칙, 권한 및 롤백 메커니즘)를 모두 확정하여 명시함.
  - **Task 1.2.5 완료 메모 (설계 확정, 관찰 결론 및 리뷰 회고)**:
    - **최종 확정된 설계**:
      - `TypeBlob` FieldType (`resource/ir.go`, `validate.go`).
      - `storage.BlobStore` 인터페이스 (`storage/store.go` - 관계형 `Store` CRUD와 100% 독립 분리).
      - `adapters/fsblob` 어댑터 (로컬 파일시스템 저장 + `.meta` Content-Type 메타데이터 파일 동시 관리).
      - **엔드포인트 및 권한 이원화**: 1-Step 멀티파트 `POST /api/{table}` (`ActionCreate` 1회 원자 평가) + 2-Step overwrite `POST /api/{table}/{id}/upload/{field}` (`ActionUpdate`), 조회 (`ActionRead`), 삭제 (`ActionDelete`).
      - **원자적 롤백**: `adapters/sqlite.Store` 내 unexported internal helper `HardDeletePhysically` (`DELETE FROM table WHERE id = ?`) 사용 (공개 `storage.Store` 인터페이스 비오염).
    - **리뷰 사이클에서 밝혀진 4가지 문제점 복기 (Milestone 2 회고 톤)**:
      1. *Key 발급 규칙 명칭 혼동*: 초안의 "결정적(deterministic)" 용어가 timestamp/UUID 특성과 충돌 ➔ philosophy.md ③과의 원칙적 충돌 방지를 위해 "고유성이 보장되는(collision-free)" 표현으로 정정 (`blobs/{table}/{record_id}/{field}_{ts}{ext}`).
      2. *Reload 실패 시 스키마 보존 케이스 누락*: reload 실패 시 기존 IR 및 Blob schema 보존 여부 미검증 ➔ reload 실패 시 기존 IR 유지 및 Blob 데이터 손상 없음을 실측 검증하는 테스트 추가.
      3. *Upload 서브 엔드포인트 권한 모순 및 보안 구멍*: initial upload에 `ActionCreate`, overwrite에 `ActionUpdate` 분기 시 `create: public`인 Resource의 타인 비어있는 필드에 무단 업로드 가능한 권한 우회 구멍(Authorization Bypass) 발견 ➔ 1-step multipart create (`ActionCreate`) + 2-step overwrite (`ActionUpdate`)로 명확히 분리하여 보안 구멍 원천 차단.
      4. *SoftDelete 롤백의 비원자성 및 조용한 폴백 회귀 위험*: 1-step create 롤백에 `SoftDelete` 사용 시 row가 남아 원자성이 깨지고 retry 시 unique 충돌 발생. 또한 hardDeleter 미지원 어댑터에서 `SoftDelete` 조용한 폴백 시 무결성 회귀 위험 ➔ 내부 전용 hard delete(`DELETE FROM table WHERE id = ?`)로 교체하고, 미지원 어댑터 조용한 폴백을 엄격히 금지하여 `BLOB_STORE_FAILED_RECORD_PRESERVED` 에러로 보존 사실을 명확히 응답.
      - **핵심 원칙**: *"권한 모델과 원자성 롤백은 새 기능(blob)을 기존 CRUD 패턴에 끼워넣을 때 가장 놓치기 쉬운 지점이다."* (Task 1.3 및 Phase 4 codegen 적용 원칙).
    - **관찰 항목 3가지 최종 결론**:
      1. *`Store`/`BlobStore` 책임 분리*: 바이트 스트림과 관계형 CRUD 인터페이스가 완전히 독립되어 레벨 차원의 책임 혼동 없이 깔끔함.
      2. *Reload 영향성*: Blob Storage에는 스키마 컴파일 대상이 없어 `POST /_mold/reload`와 100% 격리 및 영향성 0건 보장 (reload 실패 시에도 완벽히 보존됨).
      3. *권한 가드 재사용성*: 서브 엔드포인트에 별도 가드 코드 0줄 신설, 기존 `auth.Evaluate` 엔진 100% 재사용 성공.
    - **가설 1(외부 모듈 제품성) 판정 상태**: 조기 확정짓지 않고 Task 1.3 (Custom UI)까지 마친 뒤 Phase 3에서 종합 판정함.
- [x] **Task 1.3: [실험] `drink-log` 전용 Custom UI (Template Override) 서빙**
  - **실험 내용**: 기본 HTML View 대신 `drink-log` 전용 커스텀 HTML/CSS를 오버라이드해본다.
  - **관찰 항목**: 프론트엔드 이관 및 커스텀 템플릿 바인딩 과정에서 발생하는 마찰 관찰.
  - **완료 조건**: Mold 기본 View를 깨뜨리지 않고 커스텀 템플릿이 자연스럽게 우선 렌더링됨을 확인한다.
  - **Task 1.3 완료 메모 (설계 확정, 관찰 결론 및 회고)**:
    - **최종 확정된 설계**:
      - `view.TemplateOverrides` 지속적 레지스트리 (`view/overrides.go`): 부팅 시 1회 생성되어 `POST /_mold/reload` 경계를 관통해 `ViewHandler` 간 참조 공유.
      - `SetCustomTemplateString(table, viewType, tplStr)` 메서드 단일 채택: `createBaseTemplate().Clone()` 기반으로 Mold 기본 템플릿 헬퍼(`canAccess`, `renderMarkdown` 등)를 자동 바인딩하고 Resource 간 템플릿 트리 오염을 100% 차단.
      - `*template.Template` 오버로드는 사전 파싱 시 Mold 헬퍼 부재 및 템플릿 격리 체계 붕괴 위험으로 의도적 배제 (마세라티 원칙 준수).
    - **검증 완료 항목**:
      - Resource 단위 오버라이드 (`Drink` 리소스에 카드형 커스텀 UI 및 별점 배지 렌더링 성공).
      - 미오버라이드 리소스 무손상 공존 (`User` 리소스는 Mold 기본 HTML View로 깨짐 없이 렌더링됨).
      - `template.Clone()` 기반 다중 리소스 격리 (Milestone 6 다중 Resource 템플릿 오염 버그 재현 0건).
      - Reload 관통 유지 (`POST /_mold/reload` 실행 후에도 커스텀 UI 유실 없이 지속 서빙).
    - **관찰 항목 4가지 결론 및 DX 마찰 파편화 데이터 포인트 발견**:
      1. *프론트엔드 이관 마찰*: 빌드 도구 없는 SSR 서버사이드 오버라이드로 이관 마찰 0건.
      2. *PageData 계약 미문서화 (★ 신규 DX 마찰 발견)*: 커스텀 템플릿 바인딩 시 `PageData` 구조체 필드명 및 템플릿 상속 규칙(`{{ define "content" }}`)이 문서화되어 있지 않아, Mold 본체 소스 코드(`view/templates.go`)를 직접 읽어야만 개발이 가능했음. 이는 Task 1.1 마찰 #1(단일 entrypoint 부재) 및 Task 1.2 마찰 #4(`transport.ErrorEnvelope` 직접 참조)와 동일한 계열의 **개발자 경험(DX) 파편화 데이터 포인트**임.
      3. *다중 Resource 템플릿 격리*: `baseLayout.Clone()` 기반 파싱으로 Resource 간 템플릿 침범 0건 실측.
      4. *Reload 지속성*: `TemplateOverrides` 참조 공유 아키텍처로 리로드 후 유실 0건 실측.
    - **가설 1(외부 모듈 제품성) 판정 상태**: Phase 1의 4대 실험(Task 1.1, 1.2, 1.2.5, 1.3)이 모두 완결되었으므로, 다음 세션에서 **Phase 3(Task 3.1) 종합 회고를 통해 수집된 마찰 전체를 모아 최종 판정 예정**.

### Phase 2: 개발자 경험(DX) 관찰 및 마찰 제거

- [ ] **Task 2.1: [실험] 외부 프로젝트의 `resources/*.yaml` 변경 시 백그라운드 리로드 연결**
  - **실험 내용**: 파일 저장(`Ctrl + S`) 시 수동 재구동 없이 투명하게 컴파일 및 리로드되도록 만든다.
  - **관찰 항목**: 파일 저장과 브라우저 반영 사이의 지연, 동시성 에러, 개발자가 느끼는 마찰을 기록한다.
  - **완료 조건**: 수동 명령어 없이 파일 저장만으로 핫컴파일 반영이 마찰 없이 완료된다.

### Phase 3: 관찰된 패턴 기반으로 구조 판정 및 정리

- [x] **Task 3.1: [관찰 및 판정] Phase 1 동안 기록된 마찰과 중복 코드 복기 및 판정 완료**
  - **관찰 내용**: 실제 수직적 중복 패턴이 존재하는지, Feature/Plan 계층이 진짜 필요한지 판정한다.
  - **완료 조건**: 가설 3의 채택 또는 기각을 확정하고 필요 시 최소한의 계층만 추출한다.
  - **Task 3.1 판정 완료 메모**:
    - 가설 1 기각 (단일 패키지 및 부트스트래핑 컨테이너 부재).
    - 가설 3 **[채택 (Adopted)]**: Mold 본체 5개 패키지 내 `switch f.Type` 6개 지점, 필드 루프 11개 지점, 가딩 분산 실측 완료. 단, 마세라티 원칙에 따라 실제 Plan 계층 구조 추출 및 설계 착수는 Phase 4(두 번째 멀티 타깃 발생 시점)로 보류함.
    - 상세 내용은 [Phase 1 회고 문서](docs/retrospectives/phase1-retrospective.md) 참조.

### Phase 4: Cloudflare Workers Static Generator 실험 (필요성 확정 시)

- [ ] **Task 4.1: [실험] `drink-log` 명세를 TypeScript + Hono + D1 코드로 생성 및 로컬 Wrangler 실행**
  - **완료 조건**: 생성된 TS+D1 코드가 로컬 Wrangler 환경에서 기존 Go API와 동일하게 반응함을 확인한다. (※ 두 번째 타깃이 발생하는 이 시점에 채택된 **가설 3 (Plan 계층)**의 실구현 및 다형성 매핑 추상화 작성을 함께 진행/재검토함)
