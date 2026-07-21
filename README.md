# Mold

> Resource 하나로 온라인 서비스의 기본 구조를 실행하는 Opinionated Runtime

## 소개

Mold는 AI와 함께 온라인 서비스를 빠르게 개발하기 위한 Runtime이다.

개발자는 Resource만 정의한다.

Mold는 Resource를 기반으로 다음을 자동으로 제공한다.

* Database Schema
* CRUD API
* Validation
* Authentication / Authorization
* 기본 HTML View

개발자는 생성된 기본 View를 자유롭게 수정하거나 완전히 교체할 수 있다.

---

## 철학

온라인 서비스는 생각보다 공통점이 많다.

대부분의 서비스는 다음 구조를 가진다.

```text
Resource
    ↓
Storage
    ↓
Transport
    ↓
View
```

Mold는 이 공통 구조를 자동화한다.

반복적인 Backend 코드를 작성하는 대신, 개발자는 비즈니스와 사용자 경험에 집중한다.

---

## 특징

* Resource First
* Opinionated by Default
* Deterministic Backend
* Flexible Frontend
* AI Friendly
* Adapter Based Storage

---

## 예제

```yaml
resource:
  name: Post

fields:
  - name: title
    type: string

  - name: body
    type: markdown
```

위 Resource 하나만 정의하면

* CRUD API
* Database
* 기본 HTML 화면

이 자동으로 생성된다.

---

## 목표

Mold는 모든 문제를 해결하려는 Framework가 아니다.

대부분의 온라인 서비스를 가장 빠르게 시작하기 위한 Runtime을 목표로 한다.

복잡한 문제는 실제로 필요해졌을 때 해결한다.
