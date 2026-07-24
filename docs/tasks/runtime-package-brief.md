# 작업 지시서: `runtime` 패키지 신설

> 이 문서는 소스 코드에 접근 가능한 구현 에이전트에게 전달하기 위한 작업 브리핑이다.
> 작업 착수 전 `NOW.md`의 "읽는 순서"에 따라 프로젝트 문서를 먼저 읽을 것.
> 특히 `docs/retrospectives/phase1-retrospective.md`(가설 1 기각 근거)와
> `cmd/mvp_e2e_test.go`(현재 보일러플레이트 실물)를 반드시 확인할 것.

---

## 1. 배경 (왜 이 작업을 하는가)

Phase 1 실험(Task 1.1~1.3)에서 `drink-log`라는 외부 프로젝트에 Mold를 적용해본 결과,
가설 1(외부 모듈 제품성)이 **기각**되었다. 관찰된 8개 표면 마찰은 2개 근본 원인으로 압축된다.

- **근본 원인 A**: 통합된 공개 런타임 API 표면 부재
  - `resource`, `storage`, `transport`, `auth`, `adapters/sqlite` 5개 패키지를 개별 임포트해야 함
  - 에러 디코딩을 위해 `transport.ErrorEnvelope`를 외부에서 직접 참조
  - Custom View 작성 시 `view.PageData` 구조체 계약이 문서화되어 있지 않아 소스를 직접 읽어야 함
- **근본 원인 B**: 부트스트래핑 컨테이너(App Container) 부재
  - `main.go`에 DB 개설, 세션 매니저, IR 로드, DDL 생성, Router/ViewHandler 조립,
    reload 콜백 작성까지 ~50줄의 보일러플레이트가 요구됨
  - DB 경로/Resource 경로/포트 등을 한 번에 받는 통합 Config가 없음

이 작업은 근본 원인 A/B를 해결하기 위해 `runtime` 패키지를 신설하는 것이다.
**근본 원인 C(Blob 권한/롤백)는 Task 1.2.5에서 이미 해결 완료되었으므로 이 작업 범위가 아니다.**

---

## 2. 스코프

### 반드시 할 것
- `runtime/` 디렉터리에 신규 패키지 작성 (디렉터리는 이미 `.gitkeep`으로 존재)
- 기존 5개 패키지(`resource`, `storage`, `transport`, `auth`, `view`, `adapters/*`)를
  **내부적으로 조립하는 얇은 조립 레이어**로 동작
- 공개 타입 재노출(`transport.ErrorEnvelope`, `view.PageData` 등 외부에서 직접
  임포트해야 했던 타입들을 `runtime` 패키지 표면에서도 접근 가능하게)

### 절대 하지 말 것
- `resource/ir.go`, `docs/ir-spec.md`에 정의된 IR 구조체 변경 — AGENTS.md 원칙 9번에 따라
  이 둘은 변경이 필요하다고 판단되면 코드를 먼저 짜지 말고 먼저 질문할 것
- Storage/Transport/View의 기존 결정론적 컴파일러 로직 수정
- 미들웨어 훅, 플러그인 로더, 다중 백엔드 스위처 등 확장 — 마세라티 원칙에 따라
  `phase1-retrospective.md` 5절에 "나중에 할 것"으로 명시되어 있으므로 이번 스코프 아님

---

## 3. 설계 스펙

### 3.1 `runtime.Config`

`cmd/mvp_e2e_test.go`의 `buildRuntime` 클로저가 실제로 필요로 하는 입력을 기준으로 구성한다.

```go
package runtime

type Config struct {
    ResourceDir string // Resource YAML 디렉터리 (필수)
    DBPath      string // SQLite DB 경로 (필수)
    BlobDir     string // fsblob 루트 디렉터리 (선택, 비어있으면 BlobStore 미설정)
}
```

- 필수 필드 검증(빈 문자열 등)은 `New()` 진입 시점에 명확한 에러로 즉시 거부한다.
  (검증 레이어 구분 원칙: 이건 "Config 자체의 유효성" 검증이지 레코드 데이터 검증이 아님을
  주석에 명시할 것)

### 3.2 `runtime.App`

```go
type App struct {
    // 내부 필드는 unexported. router, viewHandler, store, sessionMgr 등을 보유.
}

func New(cfg Config) (*App, error)
func (a *App) Listen(addr string) error
```

**`New(cfg)`가 내부적으로 수행해야 할 일** (기존 `main.go`/`cmd/mvp_e2e_test.go`의
`buildRuntime` 패턴을 그대로 캡슐화):

1. `sqlite.Open(cfg.DBPath + "?_pragma=foreign_keys(1)")` 로 Store 개설
2. `auth.NewSessionManager(store.DB())` 로 세션 매니저 생성
3. `resource.LoadAll(cfg.ResourceDir)` 로 Resource Registry 로드
4. 로드된 모든 Resource에 대해 `store.EnsureSchema(ctx, r)` 실행
5. `transport.NewRegistry()` 생성 후 각 Resource를 `Register(r, store)`
6. `transport.NewRouter(transReg)` 생성, `SetSessionManager(sm)` 연결
7. `cfg.BlobDir`가 비어있지 않으면 `fsblob.New(cfg.BlobDir)`로 BlobStore를 생성해
   `router.SetBlobStore(bs)`로 연결
8. `router.SetReloadFunc(...)` 에 1~7 중 IR 로드~Registry 조립 부분을 재수행하는
   클로저를 등록 (원자적 리로드 유지 — `docs/ir-spec.md` 6절의 "기존 IR 유지" 보장이
   깨지지 않아야 함)
9. `view.NewViewHandler(router, nil)` 로 ViewHandler 생성
   (`view.TemplateOverrides`를 나중에 주입할 수 있는 여지는 남겨둘 것 — 예: `Config`에
   `Overrides *view.TemplateOverrides` 선택 필드 추가 검토. 이 부분은 판단이 갈리면
   구현 에이전트가 임의로 결정하지 말고 보고 시 "애매했던 지점"으로 명시할 것)
10. `/api`, `/_mold` prefix는 router로, 나머지는 viewHandler로 라우팅하는
    단일 `http.Handler`를 조립 (`cmd/mvp_e2e_test.go`의 `mainHandler` 패턴과 동일)

**`Listen(addr)`**: 위에서 조립한 핸들러로 `http.ListenAndServe(addr, handler)` 실행.

### 3.3 공개 타입 재노출

`runtime` 패키지에서 아래와 같이 타입 별칭을 제공해 외부 프로젝트가
`transport`/`view` 패키지를 직접 임포트하지 않아도 되게 한다.

```go
type ErrorEnvelope = transport.ErrorEnvelope
type SuccessEnvelope = transport.SuccessEnvelope
type ListSuccessEnvelope = transport.ListSuccessEnvelope
type PageData = view.PageData
```

이 목록이 충분한지는 실제 `drink-log`에서 어떤 타입을 참조했는지
(`phase1-retrospective.md` 마찰 #5, #8) 다시 확인하고 빠진 것이 있으면 추가할 것.

---

## 4. 검증 조건 (완료 기준)

- [ ] `runtime` 패키지 자체의 유닛 테스트: `New()`의 정상 경로 / Config 누락 시 에러 경로
- [ ] `cmd/mvp_e2e_test.go`가 검증하던 시나리오(REST CRUD, HTML View, AI Workflow
      Reload)를 `runtime.New()` + `Listen()` 기반으로 재현하는 e2e 테스트 최소 1개 추가
      (기존 `mvp_e2e_test.go`를 대체하는 게 아니라, `runtime` 레이어를 통해서도
      동일하게 동작함을 별도로 증명)
- [ ] 새 e2e 테스트에서 "이 테스트의 `main` 조립부에 해당하는 코드가 10줄 이내인가"를
      코드 라인 수로 직접 확인하고 보고서에 실측치 포함
      (가설 1의 "단 1개 패키지 임포트, 보일러플레이트 0줄"이라는 채택 조건에
      얼마나 근접했는지 정량적으로 보고할 것 — 완전한 0줄은 목표가 아니고,
      10줄 이내가 이번 작업의 성공 기준)
- [ ] `go build ./...` 및 전체 테스트 스위트(`go test ./...`) 통과

---

## 5. 작업 방식 (AGENTS.md 워크플로우 준수)

- 단위 커밋으로 쪼갤 것 (예: Config/New 골격 → EnsureSchema+Registry 조립 →
  Reload 콜백 → ViewHandler 조립 → 타입 재노출 → 테스트, 각각 별도 커밋)
- 커밋 메시지: `type(scope): 내용` 형식, 예) `feat(runtime): add Config and App bootstrap skeleton`
- 문제 발견 시 기존 커밋 amend 금지, 새 커밋으로 추가 (append-only)
- 완료 후 보고에 반드시 포함:
  - 커밋별 요약 + **실제 diff**
  - 새로 추가/수정된 테스트 목록
  - 애매하거나 임의로 판단한 지점과 그 근거 (특히 3.2의 9번 항목 — TemplateOverrides
    주입 방식)
  - "구현되어 있다"와 "실제로 실행해서 확인했다"를 구분해서 보고
- 두 개 이상의 속성이 동시에 관여하는 지점(예: `BlobDir` 비어있음 + Reload,
  세션 매니저 부재 + 로그인 라우팅)은 조합 케이스를 별도로 검토·테스트할 것

---

## 6. 이 작업 완료 후 갱신할 문서

- `NOW.md`: "다음 할 일"을 이번 작업 결과 기준으로 갱신 (runtime 패키지 완료,
  가설 1 재판정 여부 — 완전 채택까지는 아니어도 마찰 감소 실측치 기록)
- `TASKS.md`: Phase 1 관련 후속 작업 상태 갱신. 근본 원인 A/B가 해소되었는지
  실측 기반으로 표시
- 새 회고 문서(`docs/retrospectives/runtime-package.md` 등) 작성 여부는
  마일스톤 규모의 변경인지 판단해서 결정 (AGENTS.md "회고와 핸드오프" 섹션 참고)
