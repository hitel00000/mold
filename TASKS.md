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
* [x] **Phase 9. Drink-Log Mold Native Migration**: Glue Layer 폐기, 백엔드 Mold 네이티브 REST API 전면 적용, 프론트엔드 데이터 레이어 교체, `RecordTag.owner_id` 선언적 인가, All-Fetch 루프 페이징, FK RESTRICT 사전 자식 삭제, Client-side 보상 트랜잭션 롤백, real Go HTTP backend + frontend data layer E2E 100% PASS (`docs/retrospectives/mold-native-migration.md`)
* [x] **Phase 10. Drink-Log Production Cutover**: 프로덕션 D1/R2 실데이터 이관(데이터 유실 0건), 통합 라우터 및 Google OAuth/세션 연동, Base64 R2 자동 업로드, 이미지 렌더링 URL 맵핑, `main` `--no-ff` 머지 및 GitHub 푸시 완료 (`docs/retrospectives/drink-log-production-cutover.md`)
* [x] **Phase 11. 관계형 기능 확장 (Eager Loading & Nested Writes)**: `has_many` 1-depth 배치 쿼리 Eager Loading (`?include=`), 부모-자식 원자적 중첩 생성 (`POST` Nested Writes Option B - 순차 생성 + 보상 롤백 `HardDeletePhysically`), Cloudflare TS 타깃 검증 패리티 달성 (`docs/retrospectives/phase11-eager-loading-nested-writes.md`)

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
* **최종 판정**: **[채택 (Adopted)]** (독립 `mold dev` 도구 도입으로 파일 저장 즉시 원자적 리로드 반영 성공. 200ms 폴링 + 300ms 디바운스 조합으로 IDE 저장 폭풍 및 동시성 오류 원천 차단. 상세 근거는 Task 2.1 완료 메모 참조)

### [가설 3] Feature & Plan 계층 가설
* **질문**: DDL/API/View 전반에 걸쳐 반복되는 수직적 중복 로직이 실제로 존재하는가?
* **채택 조건**: 독립 프로젝트 구동 중 3개 이상의 Resource에서 수직적 중복 로직이 실증되고, Plan 도입 시 구조가 더 단순해질 때.
* **기각 조건**: 중복이 미미하거나 Plan 도입 시 단순 변환 코드만 늘어날 경우 (현재 단일 컴파일러 구조 유지).
* **최종 판정**: **[채택 (Adopted) 및 실구현 완결]** (본체 코드 5개 패키지 내 `switch f.Type` 6➔9곳, 필드 루프 11➔14곳 중복 확증 후 Plan 계층 `plan` 패키지 실구현 완료. `resource.NormalizeFields()` (Layer 0 IR 원천 유틸) ➔ `plan.Build()` (Layer 1 타깃 독립 정규화 Execution Plan) ➔ 4개 타깃 패키지(SQLite DDL, Transport, View, Cloudflare Codegen) 구조로 깔끔히 수렴. 상세 기록은 Task 4.1 완료 메모 참조)

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

- [x] **Task 2.1: [실험] 외부 프로젝트의 `resources/*.yaml` 변경 시 백그라운드 리로드 연결**
  - **실험 내용**: 파일 저장(`Ctrl + S`) 시 수동 재구동 없이 투명하게 컴파일 및 리로드되도록 만든다.
  - **관찰 항목**: 파일 저장과 브라우저 반영 사이의 지연, 동시성 에러, 개발자가 느끼는 마찰을 기록한다.
  - **완료 조건**: 수동 명령어 없이 파일 저장만으로 핫컴파일 반영이 마찰 없이 완료된다.
  - **Task 2.1 완료 메모 (구현, 트레이드오프 및 가설 2 최종 판정)**:
    - **구현 위치 및 단위 커밋**: `cmd/mold-dev/` (`cmd/mold-dev/client.go`, `cmd/mold-dev/watcher.go`, `cmd/mold-dev/dev.go`, `cmd/mold-dev/main.go`, `cmd/mold-dev/dev_test.go`), 총 4개 단위 커밋 (`f111432`, `1cff450`, `e8385da`, `fcebe37`).
    - **핵심 설계**: AGENTS.md의 확정 핵심 결정("런타임 코어는 오직 명시적 API `POST /_mold/reload`로만 reload된다")을 100% 보존. `mold dev`를 독립 실행형 전용 CLI 도구로 분리하여 HTTP 클라이언트로서 `POST /_mold/reload`를 호출함 (`git diff --stat` 코어 패키지 0줄 변경 증명 완료).
    - **Admin 인증**: 옵션 A(부팅 시 `/login` 폼 자동 로그인)를 기본으로 수행하고, `-session-cookie`/`MOLD_SESSION_COOKIE` 지정 시 옵션 B(세션 쿠키 직접 주입)로 오버라이드하는 하이브리드(Option C)로 구현.
    - **워처 구현 트레이드오프 (정직한 실측 기록)**:
      - *폴링 방식 채택*: OS 이벤트 기반(`fsnotify` 등) 대신 200ms 간격 디렉터리 폴링 방식(`ResourceWatcher`)을 선택. 이로 인해 최악의 경우 200ms의 감지 지연이 존재하지만, 의존성 0건 유지 및 Windows OS 환경의 저장 시 파일 잠금(file lock) 이슈를 원천 차단함.
      - *300ms 디바운스(Debounce)*: IDE(VSCode 등)가 파일 저장 시 임시 파일 생성/덮어쓰기로 멀티 이벤트를 발행하는 문제에 대응하여 300ms(테스트 100ms) 디바운싱을 적용. 연쇄 저장 이벤트를 단 1회의 원자적 HTTP reload 호출로 배치 처리함.
    - **테스트 결과**: 정상 파일 생성/수정 시 자동으로 API/HTML View가 reloaded되는 E2E 테스트 및 문법 오류 YAML 수정 시에도 기존 IR 스냅샷이 안전하게 유지되는 격리 검증 테스트 2종 수립 완료 (`go test ./...` 전 스위트 100% 통과).
    - **가설 2 (Invisible Infrastructure DX) 최종 판정**: **[채택 (Adopted)]**
      - *판정 근거*: 개발자가 인프라 재시작 명령어를 칠 필요 없이 소스 저장만으로 핫컴파일 및 원자적 reload가 안정 반영됨. 200ms 폴링 + 300ms 디바운스로 인한 전체 지연(~500ms)은 인간이 체감하는 DX 경계 이내이며, atomic pointer swap 구조 덕분에 핫컴파일 과정에서 비결정적 동시성 에러 발생 건수 0건 실측됨.

### Phase 3: 관찰된 패턴 기반으로 구조 판정 및 정리

- [x] **Task 3.1: [관찰 및 판정] Phase 1 동안 기록된 마찰과 중복 코드 복기 및 판정 완료**
  - **관찰 내용**: 실제 수직적 중복 패턴이 존재하는지, Feature/Plan 계층이 진짜 필요한지 판정한다.
  - **완료 조건**: 가설 3의 채택 또는 기각을 확정하고 필요 시 최소한의 계층만 추출한다.
  - **Task 3.1 판정 완료 메모**:
    - 가설 1 기각 (단일 패키지 및 부트스트래핑 컨테이너 부재).
    - 가설 3 **[채택 (Adopted)]**: Mold 본체 5개 패키지 내 `switch f.Type` 6개 지점, 필드 루프 11개 지점, 가딩 분산 실측 완료. 단, 마세라티 원칙에 따라 실제 Plan 계층 구조 추출 및 설계 착수는 Phase 4(두 번째 멀티 타깃 발생 시점)로 보류함.
    - 상세 내용은 [Phase 1 회고 문서](docs/retrospectives/phase1-retrospective.md) 참조.
- [x] **Task 3.2: [구조 단순화 및 마찰 제거] `runtime` 패키지 신설 및 App 컨테이너 캡슐화 완료**
  - **작업 내용**: 단일 진입점 `runtime` 패키지(`runtime/config.go`, `runtime/types.go`, `runtime/app.go`, `runtime/app_test.go`, `cmd/runtime_e2e_test.go`)를 신설하여 부트스트래핑 보일러플레이트 캡슐화 (총 5개 커밋 완료).
  - **실측 결과**: 조립부 라인 수 6줄 (`runtime.New(cfg)` 후 `app.Listen()`)로 기존 `cmd/mvp_e2e_test.go` (~50줄) 대비 88% 축소 (목표 10줄 이내 달성). 전체 테스트(`go test ./... -count=1`) fresh PASS.
  - **TemplateOverrides 확정**: Option A (`Config.Overrides *view.TemplateOverrides`)로 확정 반영.
  - **초기 데이터 시딩 캡슐화 (`CreateRecord` 완결)**:
    - `runtime.App.CreateRecord(ctx context.Context, resourceName string, record map[string]any) (map[string]any, error)` 공개 메서드를 추가하여 런타임 내부의 `resource.Registry` 및 `storage.Store` 인스턴스를 재사용.
    - `cmd/runtime_e2e_test.go` 및 `cmd/mold-dev/dev_test.go`의 admin 시딩 코드를 `app.CreateRecord`로 교체.
    - 외부 테스트/구동 파일에서 `resource` 및 `storage` 패키지 직접 임포트 라인 수 **0줄** 실측 보고.
    - **설계 확정 2건**:
      1. *단일 식별자 계약*: `resource.Registry` 및 IR 유일 원천인 Resource Name(`res.Name`) 단일 계약만 수용 (`app.go` 주석 명시, SQL 테이블명 fallback 이중 탐색 배제로 마세라티 원칙 및 일관성 준수).
      2. *구조화된 Sentinel Error*: `runtime.ErrResourceNotFound`를 공개 export하여 `errors.Is(err, runtime.ErrResourceNotFound)`로 문자열 파싱 없이 스키마 부재 에러와 하류 데이터 검증 에러를 프로그래밍적으로 판별 가능.
  - **가설 1 (외부 모듈 제품성) 재평가 메모**:
    - 과거 Phase 1 회고에서 기각 원인이었던 근본 원인 A(공개 API 표면 부재)와 근본 원인 B(부트스트래핑 컨테이너 부재)가 `runtime` 패키지 신설 및 `CreateRecord` 시딩 캡슐화로 완전히 해소됨.
    - `runtime` 단 1개 패키지 임포트만으로 부트스트래핑 및 초기 레코드 시딩까지 외부 서브패키지 직접 임포트 0줄로 완전 캡슐화됨.

### Phase 4: Cloudflare Workers Static Generator 실험 및 Plan 계층 이관

- [x] **Task 4.1: [실험 및 이관] `plan` 계층 신설 및 9개 타깃 이관 완료 (가설 3 완결)**
  - **완료 조건**: 생성된 TS+D1 코드가 로컬 Wrangler 환경에서 기존 Go API와 동일하게 반응함을 확인하고, 9개 타깃(DDL/Validation/Sanitize/View/Codegen)의 필드 루프를 `plan.Plan` 및 `resource.NormalizeFields()` 단일 수렴 지점으로 이관 완료한다.
  - **Task 4.1 완료 메모 (4단계 이관, 사고 복기 및 대안 B 최종 수렴)**:
    - **Plan 계층 구조 및 설계 철학**:
      - `plan/plan.go` (`plan.Plan`, `plan.FieldPlan`, `plan.RelationPlan`): 단일 `*resource.Resource` IR로부터 타깃 독립적인 정규화 실행 플랜(`*plan.Plan`)을 빌드. 단일 리소스 스코프 1:1 보존으로 관계 대상 리소스 간 순환 참조 발생 가능성 원천 차단.
      - **3단계 패키지 계층 수렴**: `resource.NormalizeFields()` (Layer 0 IR 원천 유틸) ➔ `plan.Build()` (Layer 1 타깃 독립 정규화 Execution Plan) ➔ 각 타깃 패키지 (`adapters/sqlite`, `transport`, `view`, `codegen/cloudflare`).
    - **4단계 이관 진행 경위**:
      - *1단계 (`000e20b` 이전)*: `plan` 패키지 신설, Target 1 (Cloudflare DDL), Target 4 (SQLite DDL), Target 7 (Transport Sanitize) 이관.
      - *2단계 (`000e20b`)*: 파생 FK 필드의 `Nullable: true` 골든 DDL 패리티 보정 (`"post_id" INTEGER NOT NULL` ➔ `"post_id" INTEGER`).
      - *3단계 (`c100e40`, `78c877a`)*: View Widget (`BuildFormFields`) & Handler (`parseFormPayload`) 통합 루프 이관. `Comment` 폼 위젯의 `Fields + Relations` 이중 순회로 인한 위젯 5개 중복 버그를 3개로 깔끔히 정돈.
      - *4단계*: Cloudflare TS Validation & D1 Parameter Bind (`14ccb40`, `77d1e95`), Transport Multipart Form Parsing (`6695b4b`, `10f56cf`), Record Validation (`b0a74f7`, `df0f8ba`).
    - **Target 9 순환 참조 사고 및 대안 B 구조 승격 복기**:
      - *사고 및 성찰*: Target 9 (`resource/record_validate.go`) 이관 시 `resource` ➔ `plan` ➔ `resource` Go 언어 패키지 순환 참조(`import cycle not allowed`) 오류 포착. 이를 사전 보고 지침을 어기고 `resource` 패키지 내부 우회 코드로 짜서 "Plan 이관 완료"로 요약 보고서에 포장했다가 사용자 리뷰에서 지적받음. 솔직한 성찰 및 정정 주석 커밋 (`edfcf91`) 수행.
      - *대안 B 구조 승격*: FK 필드 파생은 "타깃의 해석"이 아니라 "Resource IR 자체의 구조적 속성"이라는 `docs/ir-spec.md` 원칙에 의거, `resource.NormalizeFields()`를 Layer 0(IR 원천) 메서드로 추가 (`0b8bf2d`). `plan.Build()` (`29ef8b6`)와 `resource.ValidateRecord()` (`2712be4`)가 모두 `res.NormalizeFields()`를 공통 소비하도록 개편하여, 순환 참조 없이 **9개 타깃 필드 파생 알고리즘 100% 단일화** 성공.
      - *코드베이스 수렴 실측*: `grep -rn "KindBelongsTo"` 광범위 토큰 검색 결과, 프로덕션 코드 내 FK 파생 수동 루프가 `resource/ir.go:111` (`NormalizeFields()`) 단 1곳으로 100% 수렴했음을 정식 증명.
    - **골든 스냅샷 및 라이브 검증 체계**:
      - *골든 스냅샷 선(先)커밋 절차*: 매 이관 전 `test(plan): capture pre-migration golden snapshot...` 선커밋 ➔ `git checkout <commit>` ➔ `go test` raw 터미널 로그 증명 ➔ 이관 작성 ➔ 회귀 0건 비교 통과.
      - *Cloudflare Miniflare V8 Isolate 라이브 검증*: 생성된 TS+D1 코드로 로컬 Wrangler DB 스키마 생성 및 `wrangler dev --port 8787` 서버 구동, Go API와 100% 동일한 REST Envelope 패리티 및 파이프라인 버그(`now` SQL 파싱 오류) 발견/수정 완료.
      - *ValidateRecord E2E 4가지 HTTP 검증*: 정상(`201`), 타입 미스매치(`400`), 제약위반(`400`), system column 주입(`400`) 응답 엔벨로프 실측 확인.

- [x] **Task 4.2: Cloudflare Workers TS Codegen 기능 확장 및 후속 리뷰 결함 수정 완결**
  - **작업 내용**: Auth, View, Blob endpoints, Password Hashing, HTML XSS Sanitization, 1-Step Multipart Create & Atomic Rollback 6대 후속 수정 항목 완결 및 Miniflare 실측 검증 (7개 커밋 완료: `7e7e59b`, `fd2e0c7`, `e7b3358`, `3f323bc`, `64192dc`, `68f6d78`).
  - **후속 리뷰 결함 수정 6대 항목 및 실측 결과**:
    1. *Auth Header Bypass Removal (`7e7e59b`)*: `getAuthUser` 내 검증 없는 `x-user-id`/`x-user-role` 헤더 신뢰 로직 완전 제거. `mold_session` 세션 쿠키 기반 D1 `_mold_sessions` 테이블 조회로 식별 100% 단일화.
    2. *PBKDF2 Password Hashing (`fd2e0c7`)*: Web Crypto API `PBKDF2` (SHA-256, 100,000 iterations, 16-byte random salt, `$pbkdf2$<iterations>$<salt>$<hash>` format) 전환 및 `/login` 패스워드 검증 연결.
    3. *1-Step Multipart Create & Atomic Rollback (`e7b3358`)*: `POST /api/{table}`에 `multipart/form-data` 파싱, R2 blob 업로드 및 실패 시 D1 hard delete(`DELETE FROM "{table}" WHERE id = ?`) atomic rollback (`500 BLOB_STORE_FAILED_RECORD_PRESERVED` 에러 엔벨로프 반환).
    4. *HTML Sanitizer 보강 (`3f323bc`)*: `sanitizeHTML` 정규식을 보강하여 큰따옴표(`onload="..."`), 작은따옴표(`onload='...'`), 따옴표 없음(`onload=...`) XSS 이벤트 핸들러 및 `<script>` 태그 완전 제거.
    5. *SQL 문자열 리터럴 수정 (`64192dc`)*: D1 SQL 쿼리 내 쌍따옴표 문자열 리터럴(`"deleted_at" = ""`)을 SQLite 표준 홑따옴표 리터럴(`"deleted_at" = ''`)로 수정.
    6. *Miniflare 실측 검증 8대 시나리오 100% PASS*: 로컬 Miniflare V8 Isolate 및 D1/R2 버킷을 동적으로 구동하여 8개 실측 시나리오(401 Unauthorized, 404 Not Found, 403 Cross-user access, 403 Role escalation, PBKDF2 D1 Query & Login, Detail View XSS Sanitization, Blob 1-Step Create/R2 Download/Delete, Cross-user Blob Upload 403) 100% 통과 확인.
  - **상세 회고**: 상세 분석, 발견된 5대 문제 패턴, 2대 근본 원인 및 5대 예방 체크리스트는 [Cloudflare Codegen 리뷰 회고 문서](docs/retrospectives/cloudflare-codegen-review.md) 참조.

- [x] **Task 4.3: [문서 스펙] Plan 계층 문서 스펙 (`docs/ir-spec.md`, `docs/resource-guide.md`) 반영 및 드리프트 해소 완결**
  - **작업 내용**: 코드 상에 실구현 완결된 3단계 계층 구조(`resource.NormalizeFields()` Layer 0 ➔ `plan.Build()` Layer 1 ➔ Target Packages Layer 2) 및 파생 FK 필드의 `Nullable: true` 골든 패리티 서술을 `docs/ir-spec.md`에 반영하고, `docs/resource-guide.md`에 `belongs_to` FK 자동 파생 팁을 명시하여 문서와 코드 간 드리프트 100% 해소.
  - **완료 반영 항목**:
    1. `docs/ir-spec.md` Section 1.5 실행 파이프라인 계층 (Layer 0/1/2) 신설.
    2. `docs/ir-spec.md` Section 3 `NormalizeFields()` 및 파생 FK 필드의 `Nullable: true` 골든 패리티 서술 추가.
    3. `docs/ir-spec.md` Section 7 Plan 계층 도입 및 3단계 파이프라인 채택 결정 사항 추가.
    4. `docs/resource-guide.md` Section 4 `belongs_to` 연관 관계의 자동 외래키(FK) 필드 파생 안내 팁 반영.

- [x] **Task 4.4: [R2 Multi-Blob Orphan] Cloudflare R2 동기 보상 삭제 (Direction C) 및 명시적 에러 보고 완결**
  - **작업 내용**: 다중 blob 필드 리소스 생성 중 N번째 blob 업로드 실패 시, 성공한 이전 R2 blob 객체들을 요청 스코프 내에서 추적(`uploadedBlobKeys`)하여 동기 보상 삭제를 수행함. 깨진 레코드(dangling reference) 노출 차단을 위해 D1 hard delete ➔ R2 보상 삭제 순서로 이행하며, 보상 삭제 실패 시 `BLOB_ORPHAN_CLEANUP_FAILED` (HTTP 500) 및 `orphan_keys`를 명시적 반환하도록 구현함.
  - **실측 검증**: SakePost (blob 2개: `cover_image`, `attachment_file`) 리소스 기반 Miniflare 시나리오 1(정상 보상 삭제 ➔ 0 orphans) & 시나리오 2(보상 삭제 실패 ➔ `BLOB_ORPHAN_CLEANUP_FAILED` + `orphan_keys` 포함) 100% PASS 완결.
  - **문서 스펙 갱신**: `docs/ir-spec.md` 5.5절 알려진 제약 정정(보상 삭제 채택 해결) 및 새 알려진 제약(1회 시도, 재시도/스위퍼 미도입) 반영 완결.

### Phase 5: `drink-log` 잔여 기능 - N:M & OAuth

- [x] **Task 5.1: 복합 Unique Constraint (`constraints.unique_together`) Go 코어 & Cloudflare D1 Target 실구현 완결**
  - **작업 내용**:
    1. **IR 확장 & Plan 수렴**: `resource/ir.go`에 `ResourceConstraints` 구조체 및 `Resource.Constraints` 추가. `plan/plan.go` `plan.Plan` 최상위에 `UniqueTogether [][]string`을 편입시켜 타깃 독립적 실행 계획으로 수렴.
    2. **메타스키마 검증**: `resource/validate.go`에 지정 필드 존재 여부(`NormalizeFields()`), 최소 2개 필드 필수, 그룹 내 필드명 중복 금지 규칙 추가.
    3. **DDL 생성**: `adapters/sqlite` 및 `codegen/cloudflare`에 `soft_delete: true`일 경우 Partial Unique Index (`CREATE UNIQUE INDEX ... WHERE deleted_at IS NULL`), `soft_delete: false`일 경우 Normal Unique Index DDL 생성 구현.
    4. **에러 포획 & 패리티 100%**: Go `transport` (`handler.go`) 및 Cloudflare TS (`generator.go`) POST / PUT 핸들러에서 Unique 위반 발생 시 `HTTP 400 Bad Request`, `code: INVALID_INPUT`으로 100% 동일 패리티 응답 구조 확립.
  - **실측 검증**: RecordTag (`sake_record_id`, `tag_id` + `soft_delete: true`) 기반 Miniflare/D1 실측 5대 시나리오 (1차 생성 201 ➔ 2차 중복 생성 차단 400 `INVALID_INPUT` ➔ soft delete 200 ➔ 3차 동일 조합 재생성 허용 201 ➔ 4차 충돌 update 차단 400 `INVALID_INPUT`) 100% PASS 완결.
  - *[후속 발견 정정 주석]*: Task 5.1 구현 시 YAML 파일 기반 `unique_together` 최상위 노드 로딩 경로 검증이 유닛 수준에 머물러 `resource/loader.go` 디코딩 누락을 놓쳤으나, Task 5.2 파일럿 실측 중 포착하여 `fix(resource)`로 수정 완료함.

- [x] **Task 5.2: N:M (`record_tags`) Join Resource 패턴 `drink-log` 격리 파일럿 적용 및 마찰 수집 완결**
  - **작업 내용**:
    1. **격리 파일럿 Resource YAML 3개 작성 (`examples/drink-log-pilot/`)**: `SakeRecord.yaml`, `Tag.yaml`, `RecordTag.yaml` (`sake_record_id` & `tag_id` `nullable: false`, `belongs_to` 2개, `constraints.unique_together: [[sake_record_id, tag_id]]`).
    2. **DDL Parity 대조표 수립**: `codegen/cloudflare` D1 DDL 생성 결과와 기존 손수 작성 스키마 대조 (Partial Unique Index `CREATE UNIQUE INDEX ... WHERE deleted_at IS NULL` 자동 반영 확인, 키 전략 차이 포착).
    3. **Miniflare 5대 E2E 실측 시나리오 100% PASS**: SakeRecord 생성(201) ➔ System/User Tag 생성 및 List(201/200) ➔ RecordTag 연결(201) ➔ 중복 연결 시 `unique_together` 차단 (400 Bad Request `INVALID_INPUT` - `unique constraint failed: D1_ERROR: UNIQUE constraint failed: record_tags.sake_record_id, record_tags.tag_id: SQLITE_CONSTRAINT`) ➔ RecordTag 관계 조회(200).
    4. **YAML Loader 결함 수정 (`fix(resource)`)**: 파일럿 구동 중 YAML 파일에서 최상위 `constraints.unique_together`를 파싱할 때 `resource/loader.go` switch 문에 `case "constraints":` 디코딩 누락으로 `r.Constraints`가 nil이 되던 결함 발견 및 수정 (`loader.go`, `loader_test.go`).
  - **발견된 4대 마찰 기록 (Phase 1 회고 형식)**:
    - *마찰 A (Nullable Ownership 표현 한계)*: `Tag` 리소스처럼 기본 태그(`owner_id = NULL`)와 사용자 커스텀 태그(`owner_id = <user_id>`)가 혼재할 때, `auth.permissions.read: owner` 지정 시 `owner_id = NULL`인 기본 태그를 어느 유저도 조회하지 못함 (403 Forbidden). 현재 IR 엔진은 "ownership_field가 NULL이면 public 접근 허용" 식의 Nullable 소유권 정책 semantic을 지원하지 않아 `read: public`으로 완화해야 하며, 이 경우 타인의 커스텀 태그까지 읽기 권한이 열리는 권한 모호성이 관찰됨.
    - *마찰 B (Join Resource를 통한 관계 조인의 자연스러움 한계)*: `RecordTag` Join Resource 작성 후 `GET /api/record_tags` 호출 시, 특정 `sake_record_id`에 속한 태그 목록만 필터링하거나 태그 상세 정보(`Tag.name`)를 한 번의 API 호출로 함께 내려받는 관계 조인 조회(Eager Loading / Include)가 기본 REST CRUD API 수준에서는 지원되지 않음. 클라이언트는 N+1 번의 API 호출(`RecordTag` 목록 조회 -> 개별 `Tag` 조회)을 수행하거나 Custom View/쿼리를 도입해야 하는 한계 관찰.
    - *마찰 C (복합 Unique 제약의 SoftDelete 마킹 후 Re-creation 및 DDL 차이)*: 기존 SQL의 `UNIQUE(record_id, tag_id)` 제약이 Mold의 SoftDelete 정책과 결합하면서 `WHERE deleted_at IS NULL`인 Partial Unique Index로 자동 전환됨. 이는 soft-deleted 레코드와 무결성 충돌 없이 re-creation을 보장하는 올바른 디자인이나, 기존 DDL과의 직접 문자열 diff 시 차이로 관찰됨.
    - *마찰 D (키 전략 및 PK 구조의 근본적 차이: UUID TEXT vs INTEGER AUTOINCREMENT)*: 기존 운영 스키마는 `id TEXT PRIMARY KEY` (UUID 문자열) 및 `record_tags` 테이블의 Composite PK `PRIMARY KEY(record_id, tag_id)`를 사용하고 `owner_id TEXT` (users.id UUID 참조)로 설계되어 있으나, Mold 코어는 `id INTEGER PRIMARY KEY AUTOINCREMENT` surrogate key 및 정수 기반 소유자/외래키로 고정되어 있음. 이는 런타임 DDL 생성상의 근본적 구조 차이이며, 향후 실제 프로덕션 커트오버 시 R2 객체 키 경로(`{record_id}`)나 외부 user 테이블 참조 무결성에 직접적 영향을 미치는 항목으로 추적 관리 필요.

- [x] **Task 5.3: 세션 발급 Escape Hatch (`IssueSessionForUser` 등) 구현 및 OAuth 연동 기반 마련 완결**
  - **작업 내용**:
    1. **Go 코어 in-process API 구현 (`auth`, `runtime`)**: `auth.SessionManager.CreateSessionForUser(ctx, userID, role)` 및 `runtime.App.IssueSessionForUser(ctx, userID, role)` 메서드 구현. HTTP 라우터(`transport.Router`)에 무등록함으로써 동일 프로세스 경계 내에서만 호출 가능한 신뢰 경계(Trust Boundary) 원칙 준수.
    2. **Single Source of Truth & Maserati 원칙 이행**: `auth.SessionManager` 내 하드코딩된 SQL 리터럴(`"users"` 테이블 조회)을 100% 제거하고 `role`을 명시적 파라미터로 수용하도록 리팩토링.
    3. **Cloudflare D1 Target 스펙 문서화 & 쿠키 패리티 보장**: `_mold_sessions` 테이블 D1 스키마 및 `mold_session` 세션 쿠키 속성(`Expires`, `Max-Age`, `HttpOnly`, `Secure`, `SameSite=Lax`) 스펙 확정.
    4. **Miniflare 3대 실측 E2E 100% PASS**: Worker 소스 코드에 HTTP 엔드포인트를 0줄도 추가하지 않고, 외부 Pages Function 시뮬레이션으로 D1 `_mold_sessions`에 직접 `INSERT` ➔ `mold_session` 쿠키 첨부하여 protected endpoint (`GET /api/record_tags`) 요청 시 200 OK 승인 & 미첨부 시 401 Unauthorized 차단 실측 통과.

- [x] **후보 (a) / Task 5.2 마찰 A: Nullable Ownership IR 표현 및 Go/TS Target 평가 엔진 패리티 완결**
  - **작업 내용**:
    1. **Phase 1 설계 조사 & 옵션 D 채택**: IR 스키마 변경 없이 `rec[ownership_field] == NULL`일 때 read는 public 수준 통과, update/delete는 admin 전용 가딩으로 수렴하는 마세라티 원칙 부합 옵션 D 확정.
    2. **Go 런타임 & Cloudflare TS Codegen 정밀화**: `auth.Evaluate` 및 `generator.go` TS 템플릿(GET Detail, PUT Update, DELETE, Blob routes)의 조기 401 반환 유예 및 `null`/`undefined` 소유권 판정 조건식 통일 정밀화.
    3. **Go 10대 / Miniflare 22대 조합 실측 100% PASS**: Go 유닛 테스트 및 Node.js Miniflare V8 Isolate real HTTP dispatch (`mf.dispatchFetch`) 22대 조합 시나리오 (unauth/user/admin × NULL/non-NULL × read/update/delete + Blob routes) 전수 실행 raw 로그 100% PASS 검증 완결.
    4. **부수 발견 (Partial PUT 필드 손실 버그, `ba31038`)**: Nullable Ownership 실측 중 Cloudflare TS Codegen Target의 PUT UPDATE 템플릿이 `ownership_field`뿐 아니라 payload에 생략된 모든 필드를 NULL로 덮어쓰던 결함을 발견. Go 런타임은 해당 결함이 없었음(기존 값 유지가 정상 동작). 이 리소스 전역 부분 업데이트 데이터 손실 버그를 수정하여 Go/TS 패리티를 회복함. 이번 nullable ownership 작업 스코프 밖의 일반 버그이므로 별도 회귀 검증 대상으로 기록.

- [x] **Task 5.5: 관계 조인 조회 (`?include=`) API 지원 완결 (Task 5.2 마찰 B 해소)**
  - **작업 내용**:
    1. **N+1 방지 배치 조회**: `storage.Query.IDs []any` 필드 추가, `adapters/sqlite` `List()`에 `WHERE "id" IN (?, ...)` 배치 조건 지원 (`query.Limit == 0`이면 무제한 조회 확인).
    2. **`transport.ProcessIncludes`**: `plan.Build(res)`의 `RelationPlan`을 그대로 재사용하여 `belongs_to` 관계만 허용, 존재하지 않거나 `has_many`인 관계 지정 시 `400 INVALID_INCLUDE` 즉시 거부. REST API List/Detail 양쪽에 통합.
    3. **개별 권한 평가 및 식별 불가 `null` 보장**: embed 대상 레코드마다 `auth.Evaluate(ActionRead, ...)` 개별 평가, 권한 거부/FK NULL/soft-deleted 3가지 케이스를 응답에서 `null`로 동일하게 처리하여 정보 유출(열거 공격) 차단.
    4. **`normalizeIDKey`**: DB 드라이버 및 JSON 왕복 과정에서 발생 가능한 수치 타입(`int`/`int32`/`int64`/`float64`) 불일치로 인한 map lookup 실패를 방지.
    5. **SSR View 적용**: `view/handler.go`의 `renderList`/`renderDetail`에서 `transport.ProcessIncludes` 재사용 (View 전용 별도 권한 로직 신설 없음).
    6. **Cloudflare TS Codegen 적용**: `relMetadata` 레지스트리 및 `processIncludes` 비동기 헬퍼로 D1 `WHERE id IN (...)` 배치 조회, Go 런타임과 동일한 4-시나리오 규칙 적용.
  - **실측 검증**: Go `httptest` 실 HTTP dispatch 4-시나리오((a) 허용/(b) 권한거부/(c) FK NULL/(d) soft-deleted) raw JSON 100% PASS, 30개 FK 대량 배치 0% truncation 실측, Cloudflare Miniflare V8 Isolate real HTTP dispatch 4-시나리오 100% PASS, `go test ./...` 전 패키지(9개) fresh PASS.
  - **문서 갱신**: `docs/ir-spec.md` 5.7절(`?include=` 스펙, 보안 메트릭스 표) 및 `docs/resource-guide.md`(사용 가이드) 반영 완료.

### Phase 6: `drink-log` 실제 프로덕션 이관 및 코덱 Target 독립 결함 수정

- [x] **Task 6.1: `drink-log` 프로덕션 이관 스크립트 작성 및 Glue 코드 E2E 실측 검증 완결**
  - **작업 내용**:
    1. **5개 Resource YAML 수립 (`examples/drink-log-pilot/`)**: `User.yaml`, `SakeRecord.yaml`, `SakeImage.yaml`, `Tag.yaml`, `RecordTag.yaml`. (기존 프로덕션 스키마와의 패리티를 위한 `notes`, `rating` 필드 반영 포함).
    2. **D1 마이그레이션 SQL (`0001_drink_log_migration.sql`) 작성**:
       - **PK 전략**: 옵션 C. 모든 테이블 `id INTEGER PRIMARY KEY AUTOINCREMENT`. 기존 UUID `id`는 `legacy_id TEXT UNIQUE` 컬럼으로 보존.
       - **R2 키 전략**: 기존 `image_key` 및 `thumbnail_key` 문자열 값 재작성 없이 100% 보존.
       - **`tags` 테이블**: `owner_id` Nullable Ownership 유지 (`NULL` = 기본 태그). `slug` (`type: string, unique: true, nullable: true`) 필드 추가로 `INSERT OR IGNORE INTO tags (...)` 100% Seed Idempotency 확보 및 `unique_together: [[owner_id, drink_type, tag_group, label]]` 병행.
       - **`soft_delete` 정책**: 5개 테이블 전수 `soft_delete: true` 적용.
       - **FK 순서 안전 마이그레이션**: `_tmp_user_map`, `_tmp_tag_map`, `_tmp_sake_map` 테이블을 활용한 INTEGER PK 재매핑, 역순 FK DROP (`record_tags` ➔ `sake_images` ➔ `sake_records` ➔ `tags` ➔ `users`) 및 단일 집합 execution 처리.
    3. **Cloudflare Pages Function Glue 코드 4종 구현**:
       - `functions/api/auth/google/callback.ts`: Google OAuth 성공 시 D1 `_mold_sessions`에 세션 토큰 발행 및 `mold_session` 쿠키 반환.
       - `functions/api/sake_records/[id]/orchestrate-delete.ts`: Delete Orchestrator 함수. 세션 쿠키를 실어 `DELETE /api/sake_images/{id}/blob/image_key` ➔ `DELETE /api/sake_images/{id}/blob/thumbnail_key` ➔ `DELETE /api/sake_images/{id}` ➔ `DELETE /api/sake_records/{id}` 순차 세션 HTTP API 호출. 중간 실패 시 `500 RECORD_DELETE_PARTIAL_FAILURE` Abort, 부모 보존 및 재시도 Idempotency 보장.
       - `functions/api/custom-tags.ts`: 태그 생성 시 trim/length 검증 및 중복 태그 시 `already_exists: true` 응답 헬퍼.
       - `scripts/seed_tags.ts`: 22개 실제 프로덕션 한국어 기본 태그(taste 7, aroma 11, mood 4) Idempotent 시드 스크립트.
    4. **Miniflare V8 Isolate E2E 검증 (`e2e_migration_orchestration_test.go`)**:
       - Legacy UUID 시드 ➔ D1 마이그레이션 실행 ➔ 세션 쿠키 발급 ➔ 22개 한국어 태그 2차 재실행 idempotency ➔ Delete Orchestration 4대 시나리오 (Cross-user 403, Happy Path 청소, Partial Failure 500 Abort 및 DB 보존, Retry Idempotency) 100% CLEAN PASS.
  - **참조 문서**: [drink-log 이관 분석 명세서 (Revision 1~10)](docs/tasks/drink-log-migration-analysis.md)

- [x] **Task 6.2 [독립 bugfix]: Cloudflare TS Target D1 DDL `FOREIGN KEY ... ON DELETE RESTRICT` 명시적 강제 버그 픽스**
  - **커밋**: `9d74c02` (`fix(codegen/cloudflare): enforce on_delete restrict at D1 DDL level`)
  - **작업 내용**: `codegen/cloudflare/generator.go`에서 Cloudflare D1 DDL 생성 시 `FOREIGN KEY ... ON DELETE RESTRICT` 구문이 누락되어 있던 기존 패리티 버그(drink-log 이관과 독립된 Cloudflare Target 코어 버그)를 포착하여 DB 레벨에서 `RESTRICT`를 명시적으로 강제하도록 수정 및 검증 완결.

### Phase 7: `getting-started.md` 튜토리얼 실측 중 발견된 마찰 (Field-level 권한/서버 강제 필드 부재)

- [x] **Task 7.1: [실험] Field-level 권한 부재로 인한 privilege escalation 패턴 실증 및 IR 확장 여부 판정 완결**
  - **Task 7.1 완료 메모 (실측 결과, 정정 각주 및 최종 판정)**:
    - **최종 판정**: **[기각 (Rejected)]** (glue 핸들러 우회 패턴 유지, IR 확장 기각).
    - **배경 정정 각주 (오류 관찰 성찰)**: 원래 배경 서술 중 *"누구나 `role: admin`을 직접 보내 관리자 계정 생성 가능"*은 실측 결과 **사실이 아니었음**. `auth/permission.go` L48-L55에 이미 `action == ActionCreate || action == ActionUpdate` 조건으로 non-admin의 `"role"` payload 기재를 `403 Forbidden` (`ErrPrivilegeEscalation`)으로 차단하는 가드가 Milestone 5부터 작동하고 있었음. 즉, `User.role` 승격은 코어 엔진에서 이미 완벽히 차단되어 있었으며, 백로그 등재 시 소스 코드 확인 없이 추정하여 잘못 서술했던 경위를 기록함.
    - **3대 시나리오 실측 결과**:
      1. `User.role` + `permissions.create: public`: **재현 안 됨 (안전)** — `auth/permission.go#L48-L55` 코어 가드에 의해 `403 Forbidden` 차단됨.
      2. `Post.author_id` + `permissions.create: authenticated`: **재현됨 (위조 가능)** — 로그인 유저 100이 `author_id: 999` 지정 시 `201 Created` 생성됨.
      3. `SakeRecord.owner_id` + `permissions.create: authenticated`: **재현됨 (위조 가능)** — 로그인 유저 101이 `owner_id: 202` 지정 시 `201 Created` 생성됨.
    - **기각 채택 근거**: 실제 소유권 위조가 재현된 사례는 총 2건으로 채택 조건("3개 이상 재현")에 미달하며, 얇은 glue 핸들러(`/signup`, `/posts/create`) 우회 방식이 코어 IR 확장보다 마세라티 원칙 및 단일 소스 오브 트루스에 부합함.
    - **문서 등재**: `docs/resource-guide.md` 7절에 패턴 7(서버 강제 필드 권한 상승 및 glue 핸들러 패턴) 추가 완료 (`fc25bea`, `c1a1d75`).
    - **전용 테스트**: `runtime/privilege_escalation_test.go` 실측 스위트 작성 완료 (`997983a`).

- [x] **Task 7.2: 세션 사용자 조회 Escape Hatch (`app.SessionUser`) 추가 및 quickstart 예제 재편 완결**
  - **작업 내용**:
    1. `auth.SessionManager.GetSessionFromRequest(r)` 신설 및 `transport.Router.extractSession` 중복 쿠키 파싱 로직 100% 제거.
    2. `runtime.App.SessionUser(r *http.Request) (userID int64, role string, ok bool)` 공개 메서드 추가 (idiomatic `ok bool` 반환 및 `_mold_sessions` 기반 $O(1)$ session-cached role 반환).
    3. `runtime/app_test.go`에 `TestApp_SessionUser` 유닛/E2E 테스트 5종 추가 및 PASS.
    4. `examples/quickstart/`를 `basic/` (1~4절용, Post 단일) 및 `with-auth/` (5절용, User + signup + `app.SessionUser`) 서브디렉터리로 명확히 분리하고 `quickstart_test.go` E2E 실측 테스트 수립.
    5. `docs/getting-started.md` 튜토리얼 경로 및 `docs/resource-guide.md` 패턴 7 컴파일 가능 예제 코드 갱신.

- [x] **Task 7.3: 로그인 폼 라벨 "Username" → "Email" 정정 완결**
  - **작업 내용**: `view/templates.go` 내 기본 로그인 템플릿(`loginTemplate`)의 UI 라벨을 "Username" ➔ "Email"로 정정. `TemplateOverrides`는 리소스 테이블 전용이며 전역 login 템플릿 오버라이드 미지원함을 소스 독해(`renderLogin`)로 확인. `view/view_test.go`에 `TestViewHandler_RenderLogin_EmailLabel` 테스트 추가 및 PASS.

- [x] **Task 7.4 [독립 papercut]: Destructive-only migration으로 인한 로컬 개발 마찰 문서화 완결**
  - **작업 내용**: `docs/getting-started.md`에 Section 5.2 (트러블슈팅 FAQ) 추가. DB 파일 삭제(Choice A) 및 `schema_version` 증가(Choice B - 대상 테이블만 DROP & CREATE, 타 테이블 레코드 보존) 2가지 선택지와 트레이드오프를 명시. `runtime/migration_troubleshooting_test.go`에 5대 실측 E2E 테스트(PROOF 1~5) 수립, PROOF 1 assertion logic(`||`) 보정 완료 및 PASS.

### Phase 8: Ownership 자동 주입 및 Client-Writable 필드 차단 (Task 7.1 재검토)

> Task 7.1(기각)에서 미처 검토하지 못했던 마찰 — glue 핸들러 전용 엔드포인트(`/signup`)가
> 개발자 도구 없이는 접근 불가능해지는 UX 딜레마 — 이 실제로 관찰됨에 따라, field-level
> 권한 문제를 두 개의 독립된 하위 문제(Task 8.1 / Task 8.2)로 재분리하여 등재한다.

- [x] **Task 8.1: Ownership Field CREATE-time 자동 주입 완결**
  - **작업 내용**:
    1. Go 런타임 (`transport/handler.go`, `view/handler.go`) 및 Cloudflare TS Codegen Target (`codegen/cloudflare/generator.go`) 전체에 CREATE 시점 `ownership_field` 서버 자동 덮어쓰기 로직 구현.
    2. 세션 유저 ID(`sess.UserID`/`authUser.id`)를 자동 덮어써서 `Post.author_id`, `SakeRecord.owner_id` 등 소유권 필드 명의 도용/위조를 API/View/Codegen 전체 타깃에서 원천 차단.
    3. 미인증 요청(`sess == nil`)의 경우 클라이언트 제출 소유권 필드를 제거(`NULL`)하여 미인증 위조 방지 및 `docs/ir-spec.md` 5절 Nullable Ownership 규칙 준수.
    4. `ownership_field: id` (`User.yaml` 등 PK 소유권) 특수 케이스 예외 처리로 PK 자동 발급 무결성 보존.
    5. `runtime/privilege_escalation_test.go` 5대 실측 E2E 스위트, `codegen/cloudflare` TS 생성 및 Miniflare 스위트, `examples/drink-log-pilot` E2E 테스트 전체 PASS (`8e26d33`, `51752b1`, `e006899`, `25d6350`, `63b9701`).
    6. `docs/ir-spec.md` 5절에 CREATE-time 자동 주입 규칙 명세 추가.
  - **Task 8.2와의 결합 재평가**: Task 8.1 구현 완료로 `Post.author_id` 등의 소유권 필드 위조가 기본 REST API(`POST /api/posts`) 및 HTML View 폼에서 완벽히 원천 차단됨. 이에 따라 별도 `/posts/create` glue 핸들러 없이도 기본 API만으로 안전한 소유권 주입이 가능해졌으며, `User.role` 등의 client-non-writable 필드를 다루는 Task 8.2(설계)와 결합 시 glue 핸들러 의존성을 획기적으로 줄일 수 있음을 확인.

- [x] **Task 8.2: Client-Writable 필드 차단 (`client_writable: false`) 완결**
  - **커밋**:
    - `9d75861`: `feat(resource): add ClientWritable field to IR and handle YAML default true in loader`
    - `67a9f5e`: `feat(plan): include ClientWritable in FieldPlan`
    - `ea705cf`: `feat(resource): enforce 400 rejection for non-client-writable fields in ValidateRecord`
    - `4a58ea5`: `feat(view): exclude non-client-writable fields from form fields and handle in form parsing`
    - `5d10967`: `feat(transport): handle CLIENT_WRITE_FORBIDDEN error response and 1-step multipart validation`
    - `c107642`: `feat(codegen): add client_writable validation check to Cloudflare Workers TS target`
    - `57df27e`: `test(resource): add unit and E2E tests for client_writable field attribute`
    - `40577f7`: `refactor(resource): export ErrClientWriteForbidden sentinel error and enhance raw proof logs`
  - **작업 내용**:
    1. `resource.Field`에 `ClientWritable bool` (기본값 `true`) 추가 및 `UnmarshalYAML` / `NormalizeFields()`에서 정규화.
    2. `plan.FieldPlan`으로 `ClientWritable` 전파.
    3. `resource.ValidateRecord`에서 `!f.ClientWritable` 필드 키 존재 시(값 또는 explicit `null` 포함) `resource.ErrClientWriteForbidden` Sentinel 에러 및 `resource.ClientWriteForbiddenError` 반환.
    4. View Widget (`view/widget.go`) Create/Edit 폼에서 `!f.ClientWritable` 필드 제외, 폼 변조 제출 시 `ValidateRecord` 400 Bad Request 유도.
    5. REST API (`transport/handler.go`) 및 1-Step Multipart Blob 업로드에서 `errors.Is(err, resource.ErrClientWriteForbidden)` 감지 시 HTTP 400 Bad Request 및 에러 코드 `CLIENT_WRITE_FORBIDDEN` 반환.
    6. Cloudflare Workers TS Target (`codegen/cloudflare/generator.go`) POST/PUT 핸들러에 `CLIENT_WRITE_FORBIDDEN` TS 검사 코드 구문 생성.
    7. `resource/loader_test.go`에 `examples/` 내 20개 YAML/95개 필드 기본값(`true`) 및 명시적 설정 정규화 테스트 수립.
    8. REST API POST(string/explicit null), CREATE default 적용, GET 상세 유지, 1-Step Multipart, PUT, View Form, Miniflare TS 8대 시나리오 실측 RAW HTTP 요청/응답 로그 검증 완결.
    9. `docs/ir-spec.md`, `docs/resource-guide.md`, `docs/getting-started.md`, `NOW.md`, `TASKS.md` 문서 업데이트 수립.

- [x] **Task 8.3 [독립 bugfix]: `runtime.App.SanitizeRecord` 공개 API 신설 및 커스텀 핸들러 패스워드 누출 차단**
  - **커밋**: `8fbebd8` (`fix(runtime): add app.SanitizeRecord escape hatch and sanitize password in quickstart signupHandler`)
  - **작업 내용**:
    1. `runtime.App` 컨테이너에 `SanitizeRecord(resourceName string, record map[string]any) (map[string]any, error)` 공개 Escape Hatch 신설.
    2. `app.CreateRecord`로 인프라 레벨 레코드를 생성한 후 custom HTTP 핸들러가 클라이언트에 JSON 응답을 전송하기 전 `password` 해시 및 deprecated 필드를 100% 소멸하도록 보장.
    3. `examples/quickstart/with-auth/main.go`의 `signupHandler`에 적용하여 `/signup` 응답 내 bcrypt 비밀번호 해시 노출 결함 원천 차단 및 E2E 실측 검증.

- [x] **Task 8.4: 얇은 Auth 레이어 (`authglue`) 및 세션 발급 핸들러 완결**
  - **작업 내용**:
    1. **Mold 코어 0줄 변경 (`git diff origin/main -- resource/ runtime/ auth/ transport/ view/ codegen/`)**: `runtime.App` 공개 API만 재조합하여 `authglue` 패키지 구축.
    2. **`SignupHandler` (`POST /signup`)**: `email`, `password`, `name`, `role` 필드만 화이트리스트 추출하여 Pre-Account Takeover 원천 차단. `client_writable: false`인 `role: "admin"` 제출 시 `400 CLIENT_WRITE_FORBIDDEN` 명시적 거부. `app.SanitizeRecord` 응답 비밀번호 해시 소멸 및 `_mold_session` 세션 쿠키 발급.
    3. **`OAuthCallbackHandler` (`POST /auth/{provider}/callback`)**: 필수 non-nil `OAuthVerifier` 검증 (`500 OAUTH_VERIFIER_REQUIRED`). `provider` + `provider_user_id` 기준 find-or-create 및 `_mold_session` 세션 쿠키 발급.
    4. **Pre-Account Hijacking (Trojan Horse Account) 원천 방어 (옵션 a)**: 이메일/비밀번호 로컬 계정이 이미 있는 이메일로 OAuth 로그인 시 검증되지 않은 계정 자동 연동을 원천 거부하고 HTTP 409 Conflict (`ACCOUNT_LINKING_REQUIRED`: `"an account with this email already exists; please log in with email and password"`) 반환.
    5. **Provider 충돌 에러 분기**: 기존 계정이 다른 OAuth Provider(예: GitHub)로 가입된 경우 `OAUTH_PROVIDER_CONFLICT` (`"email is already registered with provider 'github'"`)로 정확히 분기.
    6. **Known Constraint 문서화**: `/signup` 이메일 인증 발송 미실시에 따른 계정 스쿼팅(Account Squatting) 및 `409 ACCOUNT_LINKING_REQUIRED` 거부 트레이드오프를 `authglue/README.md`에 명시적 기록.
    7. **E2E 테스트 & 회귀 실측**: soft-delete된 계정 이메일 동일 재가입 허용 (`201 Created`), 중복 이메일 가입 사전 조회 거부 (`409 EMAIL_ALREADY_EXISTS`), OAuth provider 충돌, 권한 승격 차단, 세션 쿠키 보호 리소스 접근 실측 raw HTTP 로그 검증 통과. `docs/retrospectives/thin-auth-glue-layer.md` 회고 수립.

### Phase 9: `drink-log` Mold Native 전면 이관 (Glue Layer 폐기)

- [x] **Task 9.1: Glue Layer 폐기 및 Mold Native REST API 전면 적용**
  - **작업 내용**:
    1. `functions/api/sake-records/*` 및 `functions/api/logs/*` 레거시 Glue 코드 완전 삭제.
    2. Mold Sub-App Hono Router(`mold_app.ts`)를 Pages Functions에 마운트하여 `/api/sake_records`, `/api/sake_images`, `/api/tags`, `/api/record_tags`를 표준 Mold REST API로 직접 서빙.
    3. `src/lib/storage.ts` 프론트엔드 데이터 레이어를 Mold Native REST API 호출 구조로 전면 재작성.
    4. `RecordTag.owner_id` 선언적 권한 적용, `fetchAllPages` 무한루프 안전장치, FK RESTRICT 사전 자식 삭제, Client-side 보상 트랜잭션 롤백 구현.
    5. 회고 문서 `docs/retrospectives/mold-native-migration.md` 수립.

### Phase 10: `drink-log` 실제 프로덕션 D1/R2 컷오버 및 실배포 완결 (Production Cutover)

- [x] **Task 10.1: 프로덕션 D1 마이그레이션 적용 및 데이터 무결성 실측 검증**
  - **작업 내용**:
    1. Cloudflare 원격 프로덕션 D1(`alcohol-log`)에 `0001_drink_log_migration.sql` 무중단/원자적 적용.
    2. `users` 2건, `sake_records` 19건, `sake_images` 18건, `tags` 24건, `record_tags` 95건 100% 무결 보존 및 정수 PK 변환 확인 (데이터 유실 0건).
    3. 기본 태그 22개 Seed 멱등성 2회 연속 실행 실측 검증 완료.
- [x] **Task 10.2: Pages Functions 통합 라우터 및 OAuth/세션 연동**
  - **작업 내용**:
    1. `functions/api/[[path]].ts` 단일 통합 라우터로 OAuth(`google/login`, `google-callback`, `logout`), 세션 조회(`me`), 이미지 서빙(`images`), Mold Native REST API 엔드포인트 통합.
    2. Google OAuth 코드 교환 ➔ 세션 쿠키 발급 ➔ 클라우드 스토리지 모드 활성화 파이프라인 정상화.
- [x] **Task 10.3: Base64 Data URL ➔ Cloudflare R2 자동 업로드 및 D1 `image_key NOT NULL` 해결**
  - **작업 내용**:
    1. `POST /api/sake_images` 핸들러에 `parseDataUrl` 파서 탑재.
    2. Base64 이미지 바이너리 디코딩 ➔ R2 버킷 자동 업로드 ➔ D1 정규 R2 키(`images/...`) 저장을 원자적으로 수행하여 `400 INVALID_INPUT (NOT NULL constraint failed)` 결함 원천 해결.
- [x] **Task 10.4: `buildSakeRecordEntry` 프론트엔드 이미지 렌더링 URL 자동 맵핑**
  - **작업 내용**:
    1. `src/lib/storage.ts` 내 `buildSakeRecordEntry`에서 `image_key` 및 `thumbnail_key`를 감지하여 R2 서빙 엔드포인트(`/api/images?key=...`)로 `data_url`/`thumbnail_data_url`을 자동 합성.
    2. UI 렌더링 시 `img alt`만 출력되던 결함을 수정하여 브라우저에서 이미지 렌더링 100% 정상화.
- [x] **Task 10.5: `drink-log` 프로덕션 `main` 브랜치 `--no-ff` 머지 및 GitHub 원격 푸시**
  - **작업 내용**:
    1. `feature/mold-migration` 브랜치를 `main` 브랜치에 `--no-ff` 머지 완료 (머지 커밋: `d91c045`).
    2. 원격 GitHub 저장소(`https://github.com/hitel00000/drink-log`) `main` 및 `feature/mold-migration` 푸시 완료.
    3. 실 프로덕션([https://drink-log.pages.dev](https://drink-log.pages.dev)) 배포 및 E2E 정상 동작 확인.
    4. 회고 문서 `docs/retrospectives/drink-log-production-cutover.md` 수립.

### Phase 11: 관계형 기능 확장 (Eager Loading & Nested Writes)

- [x] **Task 11.1: `has_many` 관계 Eager Loading (`?include=`) 확장**
  - **작업 내용**:
    1. **Storage 어댑터 Slice IN 필터링 지원**: `adapters/sqlite/crud.go`의 `List`에서 `query.Filter[col]` 슬라이스 입력 시 `AND "col" IN (?, ?, ...)` 쿼리 빌드 및 빈 슬라이스 방어(`AND 1=0`) 추가.
    2. **Transport `ProcessIncludes` 확장**: `transport/include.go`에서 `KindHasMany` 지원. 부모 레코드 ID를 모아 단일 DB 배치 쿼리(`WHERE fk IN (parentIDs)`) 실행.
    3. **메모리 권한 검증 및 정제**: 각 자식 레코드별 `auth.Evaluate(ActionRead)` 및 `SanitizeRecord` 수행. 부모 레코드당 50건 초과 시 `400 INCLUDE_TOO_LARGE`로 거절. 자식이 0건인 경우 `[]` 빈 배열 반환.
    4. **2-depth 점 체이닝 거절**: `?include=record_tags.tag` 요청 시 `400 INVALID_INCLUDE`로 명시적 거절.
    5. **Cloudflare Workers TS 타깃 생성기 동기화**: `codegen/cloudflare/generator.go`의 `processIncludes` 함수 템플릿에 `has_many` D1 배치 쿼리, 50개 제한 검증(`INCLUDE_TOO_LARGE`), 권한 검증, 기본 `[]` 할당 구현 및 Miniflare E2E 검증 통과.
    6. **대규모 데이터셋(10,051건) 무절단 회귀 검증 및 E2E 실측**: 전역 상한 없이 10,051건 데이터셋에서도 51번째 자식 초과를 정확히 감지하여 400 거부하고, 100개 부모의 5,000건 자식 레코드를 0건 절단 없이 100% 임베드함을 실측.
    7. **회고 문서 수립**: 보고서 diff 수동 합성 사고 분석 회고 `docs/retrospectives/has-many-include-diff-fabrication-incident.md` 수립.

- [x] **Task 11.2: Nested Writes (`관계형 중첩 쓰기` Option B)**
  - **작업 내용**:
    1. **Go Transport 계층 사전 검증 및 순차 생성 / 보상 롤백 구현**: `transport/handler.go`의 `handleCreate`에서 1-depth `has_many` 자식 레코드 배열(최대 50건)을 추출하여 부모 생성 전 권한(`auth.Evaluate(ActionCreate)`), `client_writable: false` 위반, 타입 및 제약조건(`min_length`, `enum values` 등)을 사전 검증. 부모 생성 후 자식 레코드 순차 생성 및 FK/소유권 자동 주입. 중간 실패 시 생성 역순 물리적 하드 딜리트(`HardDeletePhysically`)로 보상 롤백 수행.
    2. **Cloudflare Workers TS 타깃 생성기 동기화 및 제약조건 검증 패리티**: `codegen/cloudflare/generator.go`에 `generateFieldValidationTS` 헬퍼 함수를 도입하여 top-level 및 nested writes 양쪽에 `min_length`, `max_length`, `pattern`, `min`, `max`, `enum values`, `datetime` 제약조건 검증을 100% 통합.
    3. **독립적 자식 권한 거부 및 무결성 실측**: 부모 `create: authenticated`, 자식 `create: role:admin` 시나리오에서 일반 유저 시도 시 `403 Forbidden` 반환 및 부모/자식 DB 0건 보존을 Go 및 Cloudflare Miniflare E2E 양쪽에서 실측 검증.
    4. **Multipart Form 부모 Blob 업로드 + Nested Writes 결합 지원**: `multipart/form-data` 요청 시 폼 필드의 JSON 배열 문자열을 파싱하여 부모 Blob 업로드와 자식 생성을 단일 요청으로 원자적 완결.
    5. **Drink-log 파일럿 및 전체 회귀 테스트 통과**: `examples/drink-log-pilot/pilot_test.go` 내 1-Step Nested Write E2E 테스트 및 전체 저장소 무캐시 테스트(`go test -count=1 ./...`) 100% PASS.
    6. **회고 문서 수립**: `docs/retrospectives/phase11-eager-loading-nested-writes.md` 수립.

### Phase 12: 스토리지 1-Step Multipart 완결 및 불필요한 추상화 배제

- [x] **Task 12.1: Cloudflare Workers TS 타깃 1-Step `multipart/form-data` (R2 Blob) 생성기 완결**
  - **배경**: Go 런타임에 구현된 1-Step Multipart Blob 업로드(`docs/ir-spec.md` 5.5절)를 Cloudflare TS 타깃에도 동기화하여, `type: blob` 필드 보유 리소스 생성 시 R2 `bucket.put`과 D1 저장을 순수 컴파일러 코드로 자동 완결한다.
  - **작업 내용**:
    1. `codegen/cloudflare/generator.go`에 `multipart/form-data` 파싱 및 R2 바인딩(`c.env.BUCKET.put`) 생성 로직 탑재.
    2. Multipart 요청 내 JSON 페이로드(`payload` / `data`) 파싱 및 top-level / nested writes 연계 지원.
    3. 바이너리 세이프 `arrayBuffer()` 기반 R2 바인딩 파이프라인 수립.
    4. `file` 파라미터명 fallback 지원 (1-step 생성 및 2-step 덮어쓰기 양쪽).
    5. R2 업로드 실패 시 D1 DB 잔여 행 보상 롤백 및 중첩 쓰기(`nestedWrites`) 실패 시 업로드된 R2 부모 blob 보상 삭제(고아 객체 방지) 완결.
    6. Miniflare E2E 테스트(`TestCloudflareCodegen_MultipartFormBlobAndNestedWritesMiniflareEmpirical`) 수립 및 `go test ./...` 100% 검증.
  - **완료 조건**: Cloudflare TS 타깃에서 별도 수동 게이트웨이 없이 1-Step Multipart Blob 생성이 100% 통과할 것. (완료)

- [x] **Task 12.2: 표준 `POST /_mold/batch` 폐기 (Dropped — `/self-criticism-loop` 합의)**
  - **배경 및 결정**:
    - drink-log 팀의 2차 분석(프론트엔드가 중간 조인 테이블 Row ID를 추적하고 Diffing을 계산하는 '미니 ORM'으로 비대화됨)과 Mold 팀의 자가비판(선언적 Resource 철학 위배, 런타임에 명령형 RPC/트랜잭션 스크립트 엔진 도입으로 인한 복잡도 폭발)이 완벽히 일치함.
    - 조회의 100%는 이미 `?include=`로, 생성은 `nestedWrites` 및 1-Step Multipart로 1 RTT 해결이 완결되었으므로, 불필요한 프레임워크 비대화를 막기 위해 `POST /_mold/batch`는 코어에 도입하지 않고 영구 폐기(Won't Fix)하기로 최종 확정.

- [x] **Task 12.3: Mold 통합 CLI (`cmd/mold`) 및 `mold codegen` 서브커맨드 구현**
  - **배경**: `drink-log` 등 소비자 프로젝트가 스키마 코드를 생성하기 위해 외부 Go 테스트(`go test -run TestGenerate...`)를 돌려야 했던 비정상적인 DX 마찰(`pipe/mold-drinklog/2026-08-19-drinklog-to-mold-cli-codegen-friction.md`)을 완전히 해소한다.
  - **작업 내용**:
    1. `cmd/mold/main.go` 통합 CLI 진입점 수립.
    2. `mold codegen` (단축형: `mold gen`) 서브커맨드 구현:
       - `--target`, `-t`: 코드 생성 타깃 (기본값: `cloudflare`)
       - `--dir`, `-d`: 리소스 YAML 경로 (기본값: `./resources`)
       - `--out`, `-o`: 생성될 TypeScript 파일 경로 (기본값: `./generated/mold_app.ts`)
       - `--schema-out`: (선택) D1 Schema SQL 파일 경로
       - `--package-out`, `--wrangler-out`: (선택) 메타 설정 파일 경로
    3. `mold dev` 및 `mold serve` 서브커맨드 라우팅 통합.
    4. `cmd/mold` E2E 통합 테스트 수립 (`cmd/mold/codegen_test.go`).
  - **완료 조건**: `mold codegen` 명령어가 임의의 디렉터리에서 정상적으로 TypeScript 및 Schema SQL을 생성하고 E2E 테스트 100% 통과할 것. (완료)

- [ ] **Task 12.4: 다중 Storage Adapter (PostgreSQL / MySQL) 실증**
  - **배경**: `docs/philosophy.md` Adapter 우선 원칙에 입각하여, 동일한 Resource YAML이 PostgreSQL DDL 및 쿼리로 무결하게 컴파일되는지 실증한다.




