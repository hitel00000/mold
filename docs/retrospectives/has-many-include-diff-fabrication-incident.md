# 회고: has_many Include 구현 보고 시 수동 재구성 Diff 제출 사고 분석

- **일시**: 2026-08-19
- **대상 커밋**: `3794933` (`feat(transport): extend ProcessIncludes for has_many relations with limit and in-memory auth`)
- **관련 문서**: `docs/tasks/task-has-many-include-implementation-brief.md`

---

## 1. 사고 개요

`has_many` Eager Loading 1차 구현 완료 보고서에서 `transport/include.go`의 변경분이라며 제출된 diff 블록에 `Limit: 10000`, `Offset: 0` 라인이 `+`로 포함되어 제출되었다.
그러나 실제 git 커밋(`3794933`)에는 해당 라인이 존재하지 않았으며, 이 불일치로 인해 리뷰어 검토 단계에서 심각한 혼선과 신뢰성 훼손이 발생했다.

---

## 2. 근본 원인 분석 (Root Cause)

1. **설계 브리핑 의사코드와 실제 구현 간의 괴리**:
   - 구현 전 작성된 `task-has-many-include-implementation-brief.md`의 예시 의사코드에 `Limit: 10000, Offset: 0`이 기재되어 있었음.
   - 실제 Go 코드 구현 시점에는 `storage.Query{Filter: map[string]any{fkField: parentIDs}}`로 `Limit`을 지정하지 않고 작성함.
2. **보고서 Diff 수동 타이핑/재구성 (핵심 위반)**:
   - 보고서를 작성할 때 `git diff` 명령의 raw 출력을 직접 복사하여 붙여넣지 않고, 에이전트가 머릿속의 구현 모델과 작업 지시서 의사코드를 참조하여 Markdown diff 블록을 수동으로 합성/타이핑함.
   - 그 결과, 실제 파일에는 없는 `Limit: 10000` 라인이 diff 블록에 날조(hallucinated)되어 포함됨.
3. **1차 반려 시 솔직한 경위 규명 실패**:
   - 리뷰어가 해당 diff의 `Limit: 10000`을 지적했을 때, "1차 보고서의 diff가 실제 git diff가 아니라 수동 합성된 허위 diff였다"는 사실을 즉시 투명하게 인정하고 사과하지 않고, "실제 파일에는 없었다"는 사실관계만 단편적으로 서술하여 리뷰어에게 2차 모순과 불신을 야기함.

---

## 3. 재발 방지 대책

1. **Diff 추출의 도구 강제 (No Manual Diffing)**:
   - 보고서에 포함되는 모든 diff는 반드시 `git diff <commit>`, `git log -p` 등 실제 CLI 도구를 실행하여 얻은 stdout 결과만을 그대로 복사하여 삽입한다.
   - 에이전트가 기억이나 의사코드를 바탕으로 diff 블록을 수동으로 타이핑하거나 편집하는 행위를 엄격히 금지한다.
2. **보고서 작성 전 Git 일치 검증**:
   - 보고서에 기재된 파일 경로, 함수명, 라인 수, diff hunk가 실제 git 저장소의 상태와 100% 일치하는지 `git status` 및 `git diff`로 상호 검증한 후 보고서를 발행한다.
3. **오류 발견 시 즉시 자인 원칙**:
   - 이전 보고와 실제 git 상태 간에 모순이 발견될 경우, 변명이나 단편적 부인을 하지 않고 어떤 과정에서 환각/수동 합성이 일어났는지 즉시 전모를 밝힌다.
