# Phase 1 종합 회고 (독립 프로젝트 적용 및 마찰 실증)

> 이 문서는 Mold Phase 1 (Task 1.1 ~ Task 1.3)의 완료 후, 수집된 마찰 데이터와 실측 코드를 바탕으로 가설 1, 2, 3을 최종 판정한 종합 회고 기록이다.

---

## 1. 개요

Phase 1은 Mold MVP (Milestone 1~6) 완료 이후, **"실제 외부 서비스 도메인(`drink-log`)에 Mold를 적용할 때 발생하는 마찰을 실증적으로 수집하고 판정한다"**는 목적하에 진행되었다.

사전 추측으로 코드를 미리 짜지 않는 **마세라티 원칙**과 **"실험 ➔ 관찰 ➔ 마찰 제거"** 방법론에 따라, Task 1.1(외부 모듈 적용), Task 1.2(Auth/Permissions 실측), Task 1.2.5(Blob Storage R2 갭 분석), Task 1.3(Custom UI Template Override)을 순차적으로 진행하였으며, 각 단계에서 수집된 8개의 마찰 현상을 종합 분석하여 핵심 아키텍처 결정을 내렸다.

---

## 2. 마찰 재분류: 8개 표면 마찰 ➔ 3개 근본 원인

Phase 1 진행 동안 관찰된 8개의 표면 마찰은 분석 결과 **3개의 근본 원인**으로 압축되었다.

| 구분 | 표면 마찰 현상 | 근본 원인 |
| :--- | :--- | :--- |
| **마찰 1** | 외부 프로젝트에서 `resource`, `storage`, `transport`, `auth`, `adapters/sqlite` 5개 패키지 개별 임포트 | **근본 원인 A**: 명시적이고 통합된 공개 런타임 API 표면(Public Runtime Surface)의 부재 |
| **마찰 5** | 에러 응답 디코딩을 위해 `transport.ErrorEnvelope` 구조체 직접 참조 | **근본 원인 A**: 명시적이고 통합된 공개 런타임 API 표면(Public Runtime Surface)의 부재 |
| **마찰 8** | Custom UI 작성 시 주입되는 `PageData` 구조체 및 define 규칙 미문서화 (소스 코드 소독 파악) | **근본 원인 A**: 명시적이고 통합된 공개 런타임 API 표면(Public Runtime Surface)의 부재 |
| **마찰 2** | 메인 조립 보일러플레이트 코드 유입 (~50줄) | **근본 원인 B**: 런타임 부트스트래핑 및 컨테이너(App Container)의 부재 |
| **마찰 3** | DB/Resource/Blob 경로 등을 한번에 받는 통합 Config 구조체 부재 | **근본 원인 B**: 런타임 부트스트래핑 및 컨테이너(App Container)의 부재 |
| **마찰 4** | 로컬 개발 시 `go.mod` replace 및 sum 수동 동기화 필요 | **근본 원인 B**: 런타임 부트스트래핑 및 컨테이너(App Container)의 부재 |
| **마찰 6** | Blob Initial upload vs Overwrite 권한 분리 미비 (권한 우회 구멍) | **근본 원인 C**: Blob 확장 시 권한/트랜잭션 엣지 케이스 *(※ Task 1.2.5에서 해결 완료)* |
| **마찰 7** | 1-Step create 롤백 시 SoftDelete 사용 시 무결성 파괴 위험 | **근본 원인 C**: Blob 확장 시 권한/트랜잭션 엣지 케이스 *(※ Task 1.2.5에서 해결 완료)* |

> [!NOTE]
> **근본 원인 C에 대한 처리**: 근본 원인 C는 Task 1.2.5에서 `docs/ir-spec.md` 5.5절 스펙 정립과 `adapters/sqlite` 내 unexported internal helper `HardDeletePhysically` 구현 및 `BLOB_STORE_FAILED_RECORD_PRESERVED` 에러 응답 강제로 이미 완벽히 해결 및 반영되었다.

---

## 3. 가설별 최종 판정 및 실측 데이터

### [가설 1] 외부 모듈 제품성 (External Consumer) 가설

- 📌 **최종 판정**: **[기각 (Rejected)]** *(현 형태 기준)*
- **판정 기준**:
  - *채택 조건*: 외부 프로젝트에서 Mold 패키지 1개만 임포트하고 `resources/` 경로만 넘겨주면, 아무 보일러플레이트 없이 부팅 및 서빙될 때.
  - *기각 조건*: 외부 프로젝트 연동 시 내부 상태 강결합이나 불필요한 인프라 코드가 요구될 경우.
- **판정 근거 및 실측 데이터**:
  - **무엇이 문제였는가**: `drink-log`에서 Mold를 구동하기 위해 단일 진입점 없이 5개 패키지를 수동으로 임포트해야 했고(근본 원인 A), `main.go`에 DB 개설, Router/ViewHandler 결합, 리로드 콜백 연결 등 ~50줄의 보일러플레이트 코드가 유입되었다(근본 원인 B).
  - **왜 기각인가**: 비록 Resource 수 증가 시 보일러플레이트 증가는 $O(1)$ 상수임이 입증되었으나, 초기 조건인 "단 1개 패키지 임포트 및 보일러플레이트 0줄 부팅"을 충족하지 못하므로 기각한다.
  - **다음에 적용할 원칙**: 단일 `runtime` 패키지 및 `runtime.App` 부트스트래핑 컨테이너 도입이 필수적임을 실측으로 확정함.

---

### [가설 2] Invisible Infrastructure DX (`mold dev`) 가설

- 📌 **최종 판정**: **[판정 불가 (Pending)]**
- **사유**: Phase 2 (파일 저장 시 백그라운드 핫컴파일/리로드) 실험이 아직 진행되지 않아 실측 데이터가 없으므로 Phase 2 완결 시 판정한다.

---

### [가설 3] Feature & Plan 계층 가설

- 📌 **최종 판정**: **[채택 (Adopted)]** *(※ 구현 착수는 마세라티 원칙에 따라 보류)*
- **판정 기준**:
  - *채택 조건*: 독립 프로젝트 구동 중 3개 이상의 Resource에서 수직적 중복 로직이 실증되고, Plan 도입 시 구조가 더 단순해질 때.
  - *기각 조건*: 중복이 미미하거나 Plan 도입 시 단순 변환 코드만 늘어날 경우.
- **판정 근거 및 실측 데이터**:
  - **무엇이 문제였는가 (Mold 본체 실측)**: Mold 본체 5개 패키지(`resource`, `storage`, `transport`, `view`, `auth`)를 직접 파악한 결과:
    1. `switch f.Type` 분기가 6개 이상의 서로 다른 파일(`schema.go`, `validate.go`, `record_validate.go`, `handler.go`, `widget.go` 등)에 파편화되어 중복 작성됨.
    2. `for _, f := range res.Fields` 루프가 11개 지점에서 반복됨.
    3. `res.SoftDelete` 및 `deleted_at` 시스템 컬럼 가딩이 4개 레이어 전반에 분산 구현됨.
  - **왜 채택인가**: Milestone 2 회고에서 겪었던 "SoftDelete / ID 가드가 엔드포인트별로 흩어져 일부 누락되었던 사고"가 바로 이 수직적 파편화 패턴에서 비롯되었음이 실측 확인되었다. 즉, DDL/API/View/Auth 전반에 걸친 수직적 중복 패턴이 실증되었으므로 채택 조건을 충족한다.
  - **다음에 적용할 원칙 (마세라티 원칙에 따른 구현 보류)**:
    - 필요성은 실증되어 채택되었으나, 당장 Plan/Feature 계층을 만들어 불필요한 추상화를 앞당기지 않는다.
    - **Philosophy ⑦ (마세라티 원칙)**에 따라, **Phase 4 (Cloudflare Workers TypeScript/D1 codegen Target)처럼 두 번째 타깃이 실제로 등장하여 IR 다형성 매핑 추상화가 불가피해지는 시점에 Plan 계층을 도입 및 구현**한다.

---

## 4. 다음 단계 제안: `runtime` 패키지 신설 (근본 원인 A/B 해결)

가설 1 기각에 따른 구조 개선을 위해 `github.com/hitel00000/mold/runtime` 패키지 신설을 제안한다.

### 최소 스케치 (Concept)

```go
package runtime

type Config struct {
    ResourceDir string
    DBPath      string
    BlobDir     string
}

type App struct {
    router *transport.Router
    vh     *view.ViewHandler
}

func New(cfg Config) (*App, error)
func (a *App) Listen(addr string) error
```

### 마세라티 원칙 기준 구분
- **지금 할 것**: `runtime.App` 및 `runtime.Config` 최소 추상화를 작성하여 `drink-log` `main.go`를 10줄 이내로 단축하고, 공개 타입(`ErrorEnvelope`, `PageData`) 정리.
- **나중에 할 것**: 미들웨어 훅, 플러그인 로더, 다중 백엔드 런타임 스위처 등 엔터프라이즈급 추상화.

---

## 5. 회고 과정 자체에 대한 메타 메모 (Meta-Retrospective)

Phase 1 회고 과정에서 최초 가설 3 판정 시 **"관측 대상의 오류"**가 발생했으나, 리뷰 사이클을 통해 바로정정되었다:

1. **초기 오류**: 외부 앱(`drink-log`)의 `main.go` 보일러플레이트 관점(사용자 관점)만 보고 "Mold 본체에 중복이 없다"고 주관적 기각 결론을 냈음.
2. **교정 과정**: 가설 3의 본질 질문이 **Mold 본체 코드 내부의 수직적 중복**임을 재인식하고, `grep` 및 소스 파악을 통해 6개 지점 `switch f.Type`, 11개 지점 필드 루프, 4개 레이어 가딩 분산을 실측함.
3. **메타 교훈**: *"가설을 판정할 때는 주관적 추측이나 외곽 관찰에 의존하지 않고, 반드시 검증 대상의 내부 실측 데이터와 대조해야 한다."* 이 재검토 사이클은 Mold 회고 프로세스의 신뢰성을 크게 높인 주요 자산이다.
