# Mold

> AI와 함께 온라인 서비스를 빠르게 만드는 Opinionated Resource Runtime

---

# 목표

Mold는 **Resource 정의 하나로 온라인 서비스의 기본 구조를 실행하는 Runtime**이다.

Mold의 목표는 모든 서비스를 지원하는 것이 아니다.

대부분의 온라인 서비스가 공통적으로 가지는 구조를 자동화하여, 개발자는 비즈니스와 사용자 경험에만 집중하도록 돕는다.

---

# 핵심 철학

## Resource가 유일한 Source of Truth이다.

모든 것은 Resource에서 시작한다.

Resource를 정의하면 다음은 결정적으로 생성되어야 한다.

* Storage
* Validation
* CRUD API
* Authentication 연동
* Authorization
* 기본 View

Resource 외의 모든 것은 파생물이다.

---

## 설정보다 생성

가능한 한 설정(Configuration)을 만들지 않는다.

사용자가 옵션을 추가하도록 만들기보다 Resource 정의만으로 추론할 수 있도록 설계한다.

설정은 기본 기능이 아니라 Escape Hatch이다.

---

## Opinionated Framework

Mold는 의도적으로 선택지를 줄인다.

유연함보다 일관성을 우선한다.

Escape Hatch가 존재하더라도 기본 사용 흐름은 항상 하나여야 한다.

---

## 결정적인 Backend

동일한 Resource 정의는 항상 동일한 결과를 만들어야 한다.

동일한 입력은 항상 동일한

* Database Schema
* CRUD API
* Validation
* Permission
* 기본 View

를 생성해야 한다.

AI는 Backend를 수정하는 것이 아니라 Resource를 수정해야 한다.

생성된 코드는 언제든 다시 만들어질 수 있어야 한다.

Backend는 "런타임 컴파일러" 방식으로 동작한다. 부팅 또는 명시적 reload
시점에 YAML을 검증한 뒤 강타입 IR(중간 표현)로 한 번 컴파일하고, 이후
모든 레이어(Storage/Transport/View)는 이 IR만 참조한다. 요청마다
YAML을 다시 파싱하지 않는다. 이 원자적 컴파일이 곧 컴파일 언어의
fail-fast 안전성을 인터프리터형 구조 안에서 재현하는 방법이다.

---

## 자유로운 Frontend

기본 View는 제공하지만 소유하지 않는다.

생성된 View는 언제든 삭제하거나 교체할 수 있어야 한다.

제품의 경쟁력은 Frontend와 사용자 경험에서 나온다.

Mold는 그것을 방해하지 않는다.

---

## Adapter 우선

비즈니스는 특정 구현체를 알아서는 안 된다.

Storage는 Adapter이다.

지금은 SQLite 하나만 구현한다. PostgreSQL, MySQL, IndexedDB, Remote
REST Backend 등은 실제로 필요해졌을 때 추가한다 (마세라티 원칙 참고).
Adapter 인터페이스를 처음부터 여러 백엔드의 최소공배수로 설계하지 않는다.

Resource 정의는 어떤 Storage를 사용하더라도 변경되지 않아야 한다.

---

## 마세라티 원칙

아직 발생하지 않은 문제를 미리 해결하지 않는다.

우선순위는

1. 단순함
2. 빠른 개발
3. AI 친화성

이다.

확장성이 실제 문제로 등장했을 때 비로소 복잡성을 받아들인다.

---

# 하지 않는 것

Mold는 다음을 목표로 하지 않는다.

* 거대한 Enterprise Framework
* 분산 시스템 플랫폼
* Microservice Framework
* Workflow Engine
* Frontend Framework
* 범용 ORM

---

# MVP

다음 Resource 하나를 작성한다.

```yaml
resource:
  name: Post

fields:
  - name: title
    type: string

  - name: body
    type: markdown
```

그리고 아무 Backend 코드도 작성하지 않고

* Database Schema
* CRUD API
* Authentication
* Authorization
* 기본 HTML View

가 생성되어야 한다.

---

# 확정된 핵심 결정

아래는 이미 결정되어 재논의가 필요 없는 사항이다. 새 세션이나 AI
에이전트가 이 결정을 뒤집어야 한다고 판단하면, 코드를 먼저 짜지 말고
왜 필요한지부터 설명할 것.

* **언어**: Go
* **Transport**: HTTPS 고정
* **Storage**: SQLite 고정 (Adapter 구조는 유지하되 구현체는 하나만)
* **Migration**: destructive만 구현 (diff 기반은 실제 필요해지면 추가)
* **삭제 정책**: append-only + soft_delete 기본값. 실제 DELETE 대신
  `deleted_at` 마킹
* **YAML 문법**: 정식 배열 형태만 지원 (`fields: - name: ... type: ...`).
  축약 형태는 명시적으로 거부한다
* **Auth**: 세션 쿠키 기반
* **Resource reload**: 파일 워처가 아니라 명시적 API
  (`POST /_mold/reload`, admin 세션 필요)로만 트리거. 결정성을 위해
  비결정적인 트리거(파일 워처)는 쓰지 않는다
* **프로젝트 포지셔닝**: 복잡한 프로덕션 서비스가 아니라, 빠른
  프로토타이핑과 작은 프로덕트를 위한 도구. 이 포지셔닝이 append-only,
  destructive migration 같은 다른 결정들의 근거가 된다

---

# AI 작업 원칙

AI는 항상 다음 원칙을 따른다.

1. Resource를 Source of Truth로 간주한다.
2. 추론 가능한 것은 설정으로 만들지 않는다.
3. 런타임의 개념을 늘리지 않는다.
4. 특정 벤더에 종속되는 설계를 피한다.
5. Adapter를 우선한다.
6. 새로운 추상화보다 기존 개념으로 해결할 수 있는지 먼저 검토한다.
7. 생성된 코드는 언제든 버릴 수 있어야 한다.
8. 모든 기능은 존재 이유를 설명할 수 있어야 한다.
9. IR 구조체(resource/ir.go)나 스펙 문서(docs/ir-spec.md)를 변경해야
   한다고 판단되면, 코드를 먼저 짜지 말고 먼저 질문한다. 이 둘은
   Storage/Transport/View가 전부 참조하는 단일 계약이므로 임의로
   바꾸면 드리프트가 생긴다.

---

# 작업 및 리뷰 워크플로우

이 섹션은 Milestone 1~2를 진행하며 실제로 검증된 방식이다. 이후
마일스톤도 이 워크플로우를 기본값으로 따른다.

## 단위 커밋

* 요청받은 작업을 여러 단계로 쪼갤 수 있다면, 단계마다 별도 커밋으로
  나눈다. 여러 단계를 하나의 커밋으로 합치지 않는다.
* 커밋 메시지는 `type(scope): 내용` 형식을 따른다
  (예: `feat(resource): define IR struct types`).
* 문제를 발견해서 이전 단계로 돌아가 수정해야 하면, 기존 커밋을
  amend/rebase로 덮어쓰지 말고 새 커밋으로 추가한다. append-only
  철학을 커밋 히스토리에도 동일하게 적용한다.

## 보고 형식

작업 완료 후 보고할 때는 다음을 반드시 포함한다.

* 커밋별 요약 (커밋 메시지 + 주요 변경 내용)
* **실제 diff** (요약이 아니라 실제 코드 변경분). 자연어 요약만으로는
  "확인됨"이라고 말한 것이 실제로 확인되지 않은 경우를 놓칠 수 있다.
* 새로 추가되거나 수정된 테스트 목록
* 애매하거나 임의로 판단을 내린 지점이 있다면, 무엇을 근거로
  그렇게 판단했는지 명시. 판단이 필요한 상황에서 조용히 넘어가지 않는다.
* 확인이 필요한데 diff만으로 판단이 안 서는 사항(예: "이 함수가
  실제로 다른 함수에서 호출되고 있는가")은 직접 실행해서 검증한 결과를
  보고한다. "구현되어 있다"와 "실제로 연결되어 동작을 확인했다"를
  구분해서 보고할 것.

## 경계 조건 점검

하나의 조건(예: `soft_delete` 여부, `create`/`update` 구분, 권한 등)이
여러 함수나 엔드포인트에 영향을 준다면, 영향받는 대상을 전부 나열해서
빠짐없이 처리했는지 확인한다. 일부 함수에서만 조건이 반영되고 다른
함수에서는 누락되는 패턴이 실제로 반복해서 발생했다
(`docs/retrospectives/` 참고).

두 개 이상의 IR 속성이 동시에 관여하는 지점(예: `unique` + `soft_delete`,
`permissions` + `ownership_field`)은 각각 따로 구현하고 끝내지 말고,
조합됐을 때의 실제 동작을 별도로 검토하고 전용 테스트로 검증한다.

## 검증 레이어 구분

"정의(스키마) 자체가 유효한가"를 검증하는 것과 "지금 들어온 실제 데이터
값이 유효한가"를 검증하는 것은 서로 다른 책임이다. 함수 이름과 주석에
어느 쪽을 검증하는지 항상 명시한다. 동적 타입 데이터를 다루는 경계
(HTTP 파싱, DB 쓰기 등)에서는 타입 검증을 constraints 검증보다
먼저 수행한다.

## 회고와 핸드오프

* 마일스톤이 끝나면 `docs/retrospectives/milestone-N.md`에 회고를
  남긴다. 리뷰 과정에서 반복적으로 발견된 문제 패턴과, 다음
  마일스톤에 적용할 체크리스트를 포함한다.
* `NOW.md`는 세션 간 핸드오프를 위한 문서로, 마일스톤이 끝날 때마다
  갱신한다. 새 세션(사람이든 AI든)은 작업을 시작하기 전에 이
  문서부터 읽는다. `NOW.md` 갱신은 해당 마일스톤의 마지막 커밋에
  포함시킨다.

---

# 설계 원칙

아키텍처를 결정할 때는 항상 다음 순서를 따른다.

1. 단순함
2. 결정성
3. AI 친화성
4. 개발 생산성
5. 확장성

미래의 가능성을 위해 현재의 단순함을 희생하지 않는다.

---

# 디렉터리 구조

```text
mold/
├── cmd/
├── runtime/
├── resource/
├── storage/
├── transport/
├── auth/
├── view/
├── adapters/
├── examples/
├── docs/
│   └── retrospectives/
├── NOW.md
├── AGENTS.md
├── TASKS.md
└── README.md
```

---

# 첫 번째 목표

다음 Resource를 작성한다.

```yaml
resource:
  name: Post

fields:
  - name: title
    type: string

  - name: body
    type: markdown
```

그리고 자동으로

* SQLite Schema
* CRUD API
* HTML CRUD 화면

이 생성되어 브라우저에서 바로 사용할 수 있어야 한다.

여기까지가 첫 번째 마일스톤이다.
