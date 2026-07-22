# TASKS

## Milestone 0. 철학 고정

* [x] 프로젝트 이름 결정
* [x] 핵심 철학(Manifesto) 작성
* [x] Non Goals 작성
* [x] MVP 범위 정의

---

## Milestone 1. Resource

목표: Resource 하나를 메모리에서 표현할 수 있다.

* [x] Resource Schema 설계
* [x] Primitive Type 정의
* [x] Relation 정의
* [x] Resource Loader 구현
* [x] Resource Registry 구현

완료 기준

* YAML 하나를 읽으면 Resource 목록이 생성된다.

---

## Milestone 2. Storage

목표: Resource를 실제 데이터베이스에 저장할 수 있다.

* [x] Storage Interface 정의
* [x] SQLite Adapter 구현
* [x] Schema → CREATE TABLE 생성
* [x] Migration 전략 결정
* [x] CRUD 구현

완료 기준

* Resource 하나만으로 CRUD가 동작한다.

* 회고: [Milestone 2 회고](docs/retrospectives/milestone-2.md)

---

## Milestone 3. Transport

목표: REST API를 자동 생성한다.

* [x] HTTP Router
* [x] List
* [x] Detail
* [x] Create
* [x] Update
* [x] Delete
* [x] Pagination

완료 기준

* 새로운 Resource를 추가하면 API가 자동으로 생긴다.

* 회고: [Milestone 3 회고](docs/retrospectives/milestone-3.md)

---

## Milestone 4. Default View

목표: 관리 화면이 자동으로 생성된다.

* [x] List View
* [x] Detail View
* [x] Create Form
* [x] Edit Form
* [x] Navigation

완료 기준

* 브라우저에서 CRUD가 가능하다.

* 회고: [Milestone 4 회고](docs/retrospectives/milestone-4.md)

---

## Milestone 5. Identity

목표: 온라인 서비스의 최소 조건을 만족한다.

* [x] User Resource
* [x] Session
* [x] Authentication
* [x] Authorization
* [x] Resource별 Permission

완료 기준

* 로그인 후 권한에 따라 Resource 접근이 제어된다.

* 회고: [Milestone 5 회고](docs/retrospectives/milestone-5.md)

---

## Milestone 6. AI Workflow

목표: 사람이 Resource만 작성한다.

* [x] Resource 작성 가이드 ([resource-guide.md](docs/resource-guide.md))
* [x] AI용 프로젝트 규칙 (AGENTS.md)
* [x] AI가 Resource 추가 (`examples/` 및 pure YAML reload 워크플로우)
* [x] Runtime 자동 반영 (`POST /_mold/reload` 원자적 반영 및 오류 시 보존)
* [x] View 재생성 (`atomic.Pointer[Registry]` 스왑 기반 View 재생성)

완료 기준

* AI가 Resource만 수정해도 서비스가 확장된다.

* 회고: [Milestone 6 회고](docs/retrospectives/milestone-6.md)

---

# 검증 프로젝트

## Blog

* [x] User
* [x] Post
* [x] Comment

---

## Todo

* [x] User
* [x] Project
* [x] Task

---

## CRM

* [x] Customer
* [x] Contact
* [x] Company

---

## 성공 기준

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

그리고 아무 코드도 작성하지 않고 다음이 자동으로 생성된다.

* Storage
* CRUD
* REST API
* Default View
* Authentication 연동
* Authorization 연동

여기까지가 MVP이다.
