# Cloudflare Workers Target (Codegen)

> 이 문서는 [docs/getting-started.md](../getting-started.md)의 **Go 런타임 경로**와는
> 다른 경로를 다룹니다. 새 프로젝트를 시작하는 경우라면 대부분 getting-started.md가
> 맞는 문서입니다. 이 문서는 **이미 Cloudflare Pages + D1 + R2 위에 서비스가 있어서
> Go 런타임을 띄울 수 없는 경우**를 위한 것입니다.

---

## 1. 언제 이 경로를 쓰는가

Mold의 기본 실행 방식(`runtime.App`)은 "런타임 컴파일러"입니다 — 부팅 또는
`POST /_mold/reload` 시점에 YAML을 IR로 컴파일하고, 프로세스가 계속 떠 있으면서
그 IR을 참조합니다. 이건 Go 프로세스가 상시 떠 있을 수 있는 환경을 전제로 합니다.

Cloudflare Workers(V8 isolate)는 Go 런타임 자체를 띄울 수 없는 stateless 환경입니다.
이 경우 Mold는 런타임을 이식하는 대신, **빌드 타임에 Resource IR을 읽어 TS+Hono+D1
코드를 정적으로 생성(codegen)**하는 별도 Target으로 동작합니다.

이 Target을 실제로 프로덕션 서비스에 적용한 사례는 `examples/drink-log-pilot/`에
있습니다. 아래 내용과 함께 참고하면 도움이 됩니다.

> [!NOTE]
> `AGENTS.md`의 "확정된 핵심 결정"에 명시된 대로, 이 Target은 Go/SQLite 고정
> 결정을 뒤집는 것이 아니라 별도 산출물입니다. Resource IR 자체는 두 Target 모두에서
> 동일하게 Target 독립적입니다 (`docs/philosophy.md` ④).

---

## 2. 사전 요구사항

* Node.js (`npm`, `npx`)
* Cloudflare 계정 및 `wrangler` CLI (`npm i -g wrangler`, 또는 `npx wrangler`)
* Mold 코어 소스 checkout (`codegen/cloudflare` 패키지를 Go로 실행해야 하므로 Go 툴체인도 필요)

---

## 3. 코드 생성하기

> [!IMPORTANT]
> 이 문서를 작성하는 시점 기준으로, `codegen/cloudflare` 패키지는 Go API
> (`cloudflare.NewGenerator()` / `gen.Generate(reg)`)로만 문서화/테스트되어 있고,
> 별도의 `mold codegen` 같은 CLI 래퍼는 현재 문서상 확인되지 않습니다. 아래는
> `codegen/cloudflare/generator_test.go`에 나온 실제 사용 패턴을 그대로 옮긴 것입니다.
> CLI 래퍼가 필요하다면 별도 작업으로 제안하는 것을 추천합니다.

Resource YAML을 로드하고 Generator를 호출하는 작은 Go 프로그램을 작성합니다.

```go
// cmd/gen-cloudflare/main.go
package main

import (
	"log"
	"os"
	"path/filepath"

	"github.com/hitel00000/mold/codegen/cloudflare"
	"github.com/hitel00000/mold/resource"
)

func main() {
	reg, err := resource.LoadAll("./resources")
	if err != nil {
		log.Fatalf("failed to load resources: %v", err)
	}

	gen := cloudflare.NewGenerator()
	output, err := gen.Generate(reg)
	if err != nil {
		log.Fatalf("failed to generate: %v", err)
	}

	outDir := "./cf-worker"
	os.MkdirAll(outDir, 0755)
	writeFile(outDir, "package.json", output.PackageJSON)
	writeFile(outDir, "wrangler.jsonc", output.WranglerConfig)
	writeFile(outDir, "schema.sql", output.SchemaSQL)
	writeFile(outDir, "index.ts", output.IndexTS)
}

func writeFile(dir, name, content string) {
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
		log.Fatalf("failed writing %s: %v", name, err)
	}
}
```

실행하면 `./cf-worker`에 4개 파일이 생성됩니다: `package.json`, `wrangler.jsonc`,
`schema.sql` (D1 DDL), `index.ts` (Hono 기반 라우트 전체).

---

## 4. 로컬에서 검증하기

```bash
cd cf-worker
npm install
npx wrangler d1 execute <YOUR_DB_NAME> --local --file=./schema.sql
npx wrangler dev
```

`wrangler dev`가 띄운 로컬 서버에 Go 런타임과 동일한 REST API
(`/api/{table}`)로 요청을 보내 패리티를 확인할 수 있습니다.

배포:

```bash
npx wrangler deploy
```

---

## 5. Go 런타임과의 기능 패리티 / 알려진 제약

핵심 기능(CRUD, 검증, 세션 인증, 3단계 권한, `unique_together`, `?include=`
관계 조인, Blob 업로드)은 Go 런타임과 동일하게 동작하도록 실측 검증되어
있습니다. 다만 이 Target 고유의 알려진 제약이 있습니다.

* **다중 Blob 필드 실패 시 보상 삭제는 1회 시도만** — 재시도/비동기 스위퍼 미도입
  (`docs/ir-spec.md` 5.5절)
* **Hard delete cascade는 코어가 대신 해주지 않음** — 부모-자식 관계 삭제 시
  R2 바이너리 정리 등은 애플리케이션 레벨(Pages Function)에서 세션 기반 HTTP
  호출로 오케스트레이션해야 함 (`examples/drink-log-pilot/functions/api/sake_records/[id]/orchestrate-delete.ts` 참고)
* **정적 codegen이므로 `POST /_mold/reload` 개념이 없음** — Resource를 바꾸면
  다시 generate → deploy 해야 함

더 자세한 배경과 실측 데이터는 다음 문서에 있습니다.

* [docs/tasks/drink-log-migration-analysis.md](../tasks/drink-log-migration-analysis.md) — PK 전략, R2 키 보존, 마이그레이션 순서 등 실제 이관 의사결정 전 과정
* [docs/retrospectives/drink-log-migration.md](../retrospectives/drink-log-migration.md) — 이관 중 반복 관찰된 4대 문제 패턴
* [docs/retrospectives/cloudflare-codegen-review.md](../retrospectives/cloudflare-codegen-review.md) — Codegen 기능 확장 시 발견된 보안/스펙 결함과 체크리스트

---

## 6. 실제 사례로 보기

`examples/drink-log-pilot/`은 실제 프로덕션 서비스(사케 시음 기록 앱)를 이 Target으로
이관한 전체 사례입니다. Resource YAML 5개, D1 마이그레이션 SQL, Pages Function
Glue 코드(OAuth 콜백, Delete Orchestration, 커스텀 태그 API, Seed 스크립트)까지
전부 참고할 수 있습니다.
