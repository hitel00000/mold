# Milestone 4 (Default View) 회고

> 이 문서는 Mold Milestone 4 (Default View / 자동 생성 HTML CRUD UI) 구현 과정에서 반영된 UI 렌더링 설계, XSS 방어, Foreign Key Form 통합 및 회고 피드백 반영 사항을 기록하고, 향후 마일스톤(Identity / Auth)에 적용할 지침을 정리한다.

---

## 1. 개요

Milestone 4에서는 Resource IR 및 Transport 레이어를 기반으로 웹 브라우저에서 직접 데이터 조작이 가능한 자동 생성 관리 화면(`view`)을 구축하였다.
별도의 프론트엔드 빌드 도구 없이 Go 표준 `html/template` 서버사이드 렌더링(SSR)과 Vanilla CSS/JS로 동작하며, 단일 프로세스 `Router`의 `atomic.Pointer[Registry]` 스냅샷을 상시 참조하여 `POST /_mold/reload` 발생 시 브라우저 새로고침 하나만으로 최신 스키마의 View와 Navigation이 즉시 렌더링되도록 작성되었다.

---

## 2. 주요 구현 및 반영 사항

### 1) 중앙 집적형 Widget Builder (`view/widget.go`)
- `FieldType`별 UI 위젯 매핑(`string`, `text`, `markdown`, `int`, `float`, `bool`, `datetime`, `enum`, `email`, `url`)을 단일 중앙 모듈에 통합하여, List/Detail/Form 3개 화면 전체에서 공통 재사용하도록 구현함.
- `deprecated: true` 필드는 렌더링 대상에서 완전히 배제함.

### 2) `Fields` + `Relations` (belongs_to FK) 이중 순회 Form 통합
- `res.Fields`뿐만 아니라 `res.Relations` 중 `KindBelongsTo` 관계의 `ForeignKey` (예: `post_id`) 컬럼도 Form 필드 순회 대상에 포함함.
- `<input type="number">` 입력 필드로 렌더링되어, Create/Edit Form 제출 시 FK 값이 누락되지 않고 DB 레코드 생성까지 정상 전달됨을 E2E 테스트(`TestView_FKField_FormSubmission_E2E`)로 보장함.

### 3) Markdown XSS Sanitization 방어 (`view/markdown.go`)
- Detail 화면 렌더링 시 `goldmark`로 파싱된 HTML을 `bluemonday.UGCPolicy().SanitizeBytes()`로 소독하여 `template.HTML`로 출력.
- `<script>` 태그 삽입, `onerror` 속성 유입, `javascript:` URI scheme 전파 3가지 공격 경로를 차단함을 단위 테스트(`TestMarkdown_XSSSanitization`)로 검증 완료.

### 4) Navigation & Pagination UI
- 등록된 모든 Resource 탭을 상단 헤더에 동적 렌더링하는 Navigation 구성.
- Transport의 `meta.total`, `meta.limit`, `meta.offset` 메타데이터와 연동되는 `[Prev]` / `[Next]` 페이지네이션 UI 지원.

---

## 3. 알려진 제약 및 다음 마일스톤(Milestone 5) 적용 체크리스트

1. **[FK Form Select UX 확장 고려]**
   - 현재 `belongs_to` FK 필드는 마세라티 원칙에 맞춰 직접 ID 숫자를 입력받는 `<input type="number">` 형태로 동작한다. 추후 UX 개선을 위해 대상 리소스의 Title/Name 목록을 가져오는 `<select>` 방식은 필요성이 명확해질 때 확장한다.
2. **[Auth 연동 UI 요소 준비]**
   - Milestone 5(Identity / Auth) 완료 시 로그인/로그아웃 버튼 및 권한(`read`, `create`, `update`, `delete`)에 따른 Form/버튼 비활성화 및 접근 제어가 UI에 결합되어야 한다.

---

## 4. 커밋 요약

- `feat(view): implement auto-generated HTML default CRUD views with XSS sanitization & FK form fields`
- `docs(retrospectives): add milestone-4 retrospective and update NOW.md & TASKS.md`
