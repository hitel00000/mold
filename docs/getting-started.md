# Getting Started

> 이 문서는 Mold를 처음 써보는 사람이 **손으로 따라 하며** Resource YAML 하나로
> SQLite 스키마, REST API, HTML 화면이 자동으로 생기는 것을 직접 확인하기 위한
> 튜토리얼입니다. "왜 이렇게 설계했는가"는 [docs/philosophy.md](philosophy.md),
> "Resource YAML을 어떻게 써야 하는가"는 [docs/resource-guide.md](resource-guide.md),
> "정확한 필드/구조 스펙"은 [docs/ir-spec.md](ir-spec.md)를 참고하세요.
>
> 이 문서는 **Go 런타임(`runtime.App`)** 경로를 다룹니다. 이미 Cloudflare
> Pages/D1/R2에 서비스가 있어서 정적 코드 생성(Codegen) 경로가 필요하다면
> [docs/targets/cloudflare.md](targets/cloudflare.md)로 가세요.

---

## 0. 사전 준비

* Go 1.22+ (Mold는 SQLite 어댑터가 내장되어 있어 별도 DB 서버 설치가 필요 없습니다)
* 그 외 의존성 없음 — `go run` 하나로 끝납니다

Mold 코어 소스를 아직 checkout하지 않았다면 먼저 clone합니다.

```bash
git clone https://github.com/hitel00000/mold.git
cd mold
```

이 튜토리얼의 완성된 예제 코드는 기본 버전의 경우 `examples/quickstart/basic/`, 인증 및 회원가입/소유권 필드가 추가된 버전의 경우 `examples/quickstart/with-auth/`에 있습니다. 아래 단계는
그 디렉터리를 처음부터 직접 만들어보는 과정입니다.

---

## 1. Resource 하나 작성하기

새 디렉터리를 만들고 (Mold 레포와 별도 프로젝트로 진행해도 됩니다):

```bash
mkdir -p myapp/resources
cd myapp
```

`resources/Post.yaml`을 작성합니다.

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
      max_length: 200

  - name: body
    type: markdown
    nullable: false
```

이 YAML 하나로 다음이 전부 자동 생성됩니다: SQLite `posts` 테이블, CRUD REST API
(`/api/posts`), 검증 로직, 기본 HTML 목록/상세/폼 화면 (`/view/posts`).

---

## 2. 진입점 작성하기

`main.go`:

```go
package main

import (
	"log"

	"github.com/hitel00000/mold/runtime"
)

func main() {
	app, err := runtime.New(runtime.Config{
		ResourceDir: "./resources",
		DBPath:      "./mold-quickstart.db",
	})
	if err != nil {
		log.Fatalf("failed to start Mold: %v", err)
	}
	defer app.Close()

	log.Println("listening on http://localhost:8080")
	if err := app.Listen(":8080"); err != nil {
		log.Fatal(err)
	}
}
```

이게 조립부 전체입니다. DB 개설, 세션 매니저, IR 로드, DDL 생성,
Router/ViewHandler 조립, reload 콜백 연결까지 `runtime.New` 안에 캡슐화되어
있습니다.

`go.mod`를 아직 안 만들었다면:

```bash
go mod init myapp
go get github.com/hitel00000/mold
```

로컬에서 아직 배포 전인 mold 소스를 참조해야 한다면 `go.mod`에
`replace github.com/hitel00000/mold => ../mold` 한 줄을 추가하세요.

실행:

```bash
go run .
```

`http://localhost:8080/view/posts` 를 브라우저로 열면 빈 목록 화면이 보입니다.

---

## 3. REST API로 CRUD 해보기

```bash
# 생성
curl -X POST http://localhost:8080/api/posts \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello Mold", "body": "# 첫 글\n반갑습니다."}'

# 목록 조회
curl http://localhost:8080/api/posts

# 단건 조회 (id는 위 응답의 data.id)
curl http://localhost:8080/api/posts/1

# 수정
curl -X PUT http://localhost:8080/api/posts/1 \
  -H "Content-Type: application/json" \
  -d '{"title": "Hello Mold (edited)"}'

# 삭제 (soft_delete: true이므로 deleted_at 마킹)
curl -X DELETE http://localhost:8080/api/posts/1
```

응답은 항상 `{"data": ...}` 또는 `{"error": {"code": ..., "message": ...}}`
형태의 envelope로 감싸져 옵니다.

---

## 4. 자동 생성된 HTML 화면 확인하기

`http://localhost:8080/view/posts`에 다시 접속하면 방금 만든 글이 목록에
보이고, 상세 화면(`/view/posts/1`)에서는 `body`의 markdown이 렌더링된
`<h1>` 태그를 확인할 수 있습니다. Create/Edit 폼도 같은 화면에서 바로
동작합니다.

기본 View를 쓰지 않고 직접 만든 프론트엔드로 교체하고 싶다면 언제든
버릴 수 있습니다 — REST API만 그대로 쓰면 됩니다.

---

## 5. 권한(Auth) 추가해보기

지금까지의 `Post`는 완전히 공개 상태입니다. 로그인한 사용자만 글을 쓰고,
작성자 본인만 수정/삭제할 수 있도록 만들어 보겠습니다.

먼저 `resources/User.yaml`을 추가합니다.

```yaml
resource:
  name: User
  timestamps: true
  soft_delete: true

fields:
  - name: email
    type: email
    nullable: false
    constraints:
      unique: true
  - name: password
    type: password
    nullable: false
    constraints:
      min_length: 8
  - name: name
    type: string
    nullable: false
  - name: role
    type: enum
    nullable: false
    default: "user"
    client_writable: false
    constraints:
      values: ["admin", "user"]

auth:
  permissions:
    create: public
    read: authenticated
    update: owner
    delete: role:admin
```

그리고 `resources/Post.yaml`에 소유권 필드와 권한을 추가합니다.

```yaml
fields:
  # ...기존 title, body에 이어서...
  - name: author_id
    type: int
    nullable: true

auth:
  ownership_field: author_id
  permissions:
    create: authenticated
    read: public
    update: owner
    delete: owner
```

> [!TIP]
> `update`/`delete`를 `public`으로 두면 안 됩니다 — 다른 사람이 남의 글을
> 고치거나 지울 수 있게 됩니다. 자세한 Good/Bad 패턴은
> [docs/resource-guide.md](resource-guide.md) 7절을 참고하세요.

프로세스를 재시작하지 않고 반영하려면 admin 세션으로 reload API를 호출합니다.

```bash
curl -X POST http://localhost:8080/_mold/reload \
  -H "Cookie: mold_session=<admin 세션 쿠키>"
```

개발 중에는 파일 저장할 때마다 매번 curl로 reload를 부르는 대신
`mold dev` CLI를 쓰면 파일 저장만으로 자동 반영됩니다 (`cmd/mold-dev`).

> [!TIP]
> **안전한 회원가입 (`client_writable: false`)**:
> `User.yaml`의 `role` 필드에 `client_writable: false`와 `default: "user"`를
> 함께 지정하면, `permissions.create: public`으로 가입을 공개해두어도
> 클라이언트가 `"role": "admin"`을 실어 보내는 공격 시도가 400 Bad Request
> (`CLIENT_WRITE_FORBIDDEN`)로 자동 차단되며, 기본값 `"user"`로 안전하게 가입 처리됩니다.

### 5.1 회원가입(Signup)과 커스텀 핸들러 (Glue Handler)

`client_writable: false` 덕분에 단순한 회원가입은 별도 백엔드 코드 작성 없이
Mold 기본 CRUD 화면 및 REST API (`POST /api/users`)만으로 안전하게 수용할 수 있습니다.

그러나 튜토리얼 예제인 `examples/quickstart/with-auth/main.go`에서는 회원가입 성공 시 **즉시 세션 쿠키를 발급(`app.IssueSessionForUser`)하여 자동 로그인시키는 흐름**을 구현하기 위해 커스텀 `/signup` 핸들러 방식을 유지하고 있습니다.

이처럼 다음과 같은 부가적인 유저 경험 및 서비스 로직이 필요한 경우 얇은 커스텀 핸들러(Glue Handler)를 작성하는 편이 권장됩니다:

1. **가입 즉시 세션 쿠키 자동 발급**: `app.CreateRecord` 생성 후 `app.IssueSessionForUser`로 세션 쿠키를 바로 구워주는 경우 (`examples/quickstart/with-auth/main.go` 참조)
2. **이메일 인증 토큰 발급 및 발송**: 회원가입 직후 이메일 검증 링크/토큰을 발급해야 하는 경우
3. **소셜 로그인 (OAuth Callback)**: Google, GitHub 등 외부 Provider 인증 완료 후 세션을 잇는 경우 (`examples/drink-log-pilot/functions/api/auth/google/callback.ts` 참조)
4. **복합 온보딩 트랜잭션**: 회원가입과 동시에 기본 워크스페이스나 초기 설정 레코드를 함께 생성해야 하는 경우

단순 레코드 가입은 Mold 코어 기능(`client_writable: false`)으로 해결하고, 세션 자동 발급 및 비즈니스 부가 로직이 필요할 때만 얇은 커스텀 핸들러를 얹어 확장하면 됩니다.

### 5.2 트러블슈팅: 필드를 추가했는데 `no column named ...` 에러가 발생하는 경우

`Post.yaml`에 `author_id` 필드를 추가한 뒤 기존 SQLite DB 파일이 존재하는 상태에서 레코드를 생성하면 다음과 같은 에러가 발생할 수 있습니다:

```json
{"error":{"code":"INVALID_INPUT","message":"failed to insert record into posts: SQL logic error: table posts has no column named author_id (1)"}}
```

**원인**: Mold는 빠른 프로토타이핑을 위해 **파괴적 마이그레이션(Destructive-only Migration)** 정책을 따릅니다 (`AGENTS.md` 참조). Resource YAML의 `schema_version`이 이전과 동일하면 기존 SQLite 테이블 DDL 변경 시도를 건너뛰므로, 새 필드(`author_id`) 컬럼이 테이블에 자동으로 추가되지 않습니다.

이 문제를 해소하는 두 가지 선택지와 트레이드오프는 다음과 같습니다:

1. **선택지 1: DB 파일 삭제 후 재기동 (가장 간단함)**
   * **방법**: 개발용 DB 파일(`mold-quickstart.db`)을 삭제하고 앱을 재기동합니다.
   * **트레이드오프**: 최신 스키마로 DB가 새로 개설되지만, 기존 로컬 테스트 데이터는 **전부 삭제**됩니다.
2. **선택지 2: Resource YAML의 `schema_version` 증가 (`schema_version: 2`)**
   * **방법**: `resources/Post.yaml` 최상위에 `schema_version: 2` (기존 1 ➔ 2)를 명시하고 앱을 재기동합니다.
   * **트레이드오프**: Mold가 스키마 버전 변경을 감지하고 해당 리소스 테이블(`posts`)에 대해 `DROP TABLE IF EXISTS "posts"` 후 새 스키마로 재생성합니다. 다른 테이블(`users` 등)의 데이터는 보존되지만, 해당 리소스(`posts`) 테이블 내의 기존 데이터는 **파괴적으로 삭제**됩니다 (비파괴적 `ALTER TABLE` 미지원).

---

## 6. 다음으로 볼 문서

* 전체 필드 타입 / 제약조건 / Relation / N:M 패턴: [docs/resource-guide.md](resource-guide.md)
* Resource IR의 정확한 스펙과 파이프라인 구조: [docs/ir-spec.md](ir-spec.md)
* 왜 이런 설계를 했는가 (마세라티 원칙 등): [docs/philosophy.md](philosophy.md)
* 이미 Cloudflare에 배포된 서비스를 이관하려면: [docs/targets/cloudflare.md](targets/cloudflare.md)
* AI 에이전트와 함께 이 프로젝트에 기여하려면: [AGENTS.md](../AGENTS.md)
