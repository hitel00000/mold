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

> 실제로 손으로 따라 해보고 싶다면 → [docs/getting-started.md](docs/getting-started.md)

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

## 3. 설치 및 CLI 사용법 (Installation & Usage)

### 1) Mold CLI 설치
Go가 설치된 환경에서 아래 명령어로 전역 설치합니다:

```bash
go install github.com/hitel00000/mold/cmd/mold@latest
```

---

### 2) 사용 시나리오 A: Cloudflare Workers / TypeScript 프로젝트 (Codegen)
`resources/` 폴더의 YAML을 읽어 Hono + D1 + R2 TypeScript 코드를 생성합니다:

```bash
# TypeScript 및 Schema SQL 생성
mold codegen --dir ./resources --out ./src/mold_app.ts --schema-out ./schema.sql
```

**💡 `package.json` 스크립트 연동 (Zero-Install / npx 스타일)**:  
바이너리를 전역 설치하지 않고도 프로젝트 내부에서 원클릭으로 실행할 수 있습니다:
```json
{
  "scripts": {
    "codegen": "go run github.com/hitel00000/mold/cmd/mold codegen -d ./resources -o ./functions/_shared/generated/mold_app.ts",
    "build": "npm run codegen && tsc -b && vite build"
  }
}
```

---

### 3) 사용 시나리오 B: 독립 Go 백엔드 런타임 실행 (Standalone Server)
별도의 백엔드 코드 작성 없이 YAML과 SQLite DB 파일만으로 즉시 REST API 및 SSR HTML 관리 화면을 실행합니다:

```bash
mold serve --dir ./resources --db ./app.db --port 8080
```

---

## 4. 핵심 문서 안내 (Repository Navigation)

Mold는 문서를 최소한으로 유지하며, 각 문서의 역할이 엄격하게 분리되어 있습니다.

* **[docs/philosophy.md](docs/philosophy.md)**: Mold가 존재하는 이유, 비전, 그리고 오랫동안 변하지 않을 핵심 철학 및 원칙
* **[TASKS.md](TASKS.md)**: 현재 진행 중인 상태, 검증해야 할 가설(Hypotheses), 및 Post-MVP 개발 백로그
* **[AGENTS.md](AGENTS.md)**: AI 에이전트와 사람이 함께 일할 때 준수해야 할 작업 규약
* **[docs/ir-spec.md](docs/ir-spec.md)**: Resource IR의 강타입 구조체 명세 및 검증 규칙
* **[docs/resource-guide.md](docs/resource-guide.md)**: Resource YAML 작성 가이드 및 Good/Bad 패턴 대조표
* **[docs/getting-started.md](docs/getting-started.md)**: 처음 써보는 사람을 위한 5분 튜토리얼
* **[docs/targets/cloudflare.md](docs/targets/cloudflare.md)**: 이미 Cloudflare에 배포된 서비스를 이관하는 방법
