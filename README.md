# Mold

> Resource 정의 하나로 온라인 서비스의 기본 구조를 완벽하게 자동 실행하는 Opinionated Runtime

---

## 1. 소개 (What is Mold?)

Mold는 **Resource 정의(YAML) 하나로 온라인 서비스의 기본 구조를 자동 생성 및 실행**하는 백엔드 런타임 컴파일러입니다.

개발자가 Resource를 작성하면 Mold는 다음을 자동으로 제공합니다.

* **Database Schema** (SQLite DDL & Automatic Soft-delete)
* **CRUD REST API** (`/api/{table}` & Pagination)
* **Strict Validation** (Primitive & Semantic Constraints)
* **Authentication / Authorization** (Session Cookie & 3-Tier ACL Guard)
* **Default HTML View** (List/Detail & Form SSR Engine)

---

## 2. 빠른 예시 (Quick Example)

아래의 `Post.yaml` 하나만 작성하면, 백엔드 코드 수정 0줄로 모든 CRUD API, 데이터베이스 테이블, HTML 관리 UI가 즉시 생성되어 동작합니다.

```yaml
resource:
  name: Post
  timestamps: true
  soft_delete: true

fields:
  - name: title
    type: string
    nullable: false
    constraints:
      min_length: 1

  - name: body
    type: markdown
    nullable: false
```

---

## 3. 핵심 문서 안내 (Repository Navigation)

Mold는 문서를 최소한으로 유지하며, 각 문서의 역할이 엄격하게 분리되어 있습니다.

* **[docs/philosophy.md](docs/philosophy.md)**: Mold가 존재하는 이유, 비전, 그리고 오랫동안 변하지 않을 핵심 철학 및 원칙
* **[TASKS.md](TASKS.md)**: 현재 진행 중인 상태, 검증해야 할 가설(Hypotheses), 및 Post-MVP 개발 백로그
* **[AGENTS.md](AGENTS.md)**: AI 에이전트와 사람이 함께 일할 때 준수해야 할 작업 규약
* **[docs/ir-spec.md](docs/ir-spec.md)**: Resource IR의 강타입 구조체 명세 및 검증 규칙
* **[docs/resource-guide.md](docs/resource-guide.md)**: Resource YAML 작성 가이드 및 Good/Bad 패턴 대조표
