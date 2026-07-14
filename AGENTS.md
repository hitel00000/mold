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

예시

* SQLite
* PostgreSQL
* MySQL
* IndexedDB
* Remote REST Backend

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
resource: Post

fields:
  title:
    type: string

  body:
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
└── docs/
```

---

# 첫 번째 목표

다음 Resource를 작성한다.

```yaml
resource: Post

fields:
  title:
    type: string

  body:
    type: markdown
```

그리고 자동으로

* SQLite Schema
* CRUD API
* HTML CRUD 화면

이 생성되어 브라우저에서 바로 사용할 수 있어야 한다.

여기까지가 첫 번째 마일스톤이다.
