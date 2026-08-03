# Multi-file Refactor — Implementation Plan (executable)

## Goal

Ship F0→F3 per `MULTI_FILE_REFACTOR_ADR.md` v2 inside OCR.

## Phase breakdown

### F0 — Hints
- CLI: `--mode=local|cross|full` (default local), `--cross-file=hints|off`
- Template MAIN_TASK optional appendix when hints/cross
- Rule: `refactoring/cross_file/common.md` short pack
- Wire flags into `refactor.Args`

### F1 — Context + Detect + Architect
- Package `internal/refactor/crossfile`:
  - Topology (import edges, dir groups)
  - Skeleton (signatures)
  - Cluster (budgeted)
  - Render prompt payloads
  - Parse SmellReport / RefactorPlan JSON
- Template: CROSS_DETECT_TASK, CROSS_ARCHITECT_TASK
- Agent: after local (full) or alone (cross), run detect→architect per cluster
- Convert plan/smells → multi-anchor `LlmComment` (`RelatedLocations`)
- Model + code_comment parse support for related_locations

### F2 — Similarity
- Shingle/Jaccard function windows
- Merge clusters by similarity edges
- Tests with synthetic duplicate pairs

### F3 — Apply + Verify
- `--apply` flag
- Worktree/temp apply of plan steps (suggestion_code or LLM transform)
- Verify: parse (go/parser + generic non-empty), optional `go test` when Go
- Repair loop max 3; rollback on fail
- Default apply=false

## Done criteria
- [x] Unit tests green for crossfile package + flags validation
- [x] `go build ./cmd/opencodereview`
- [x] ADR checklists marked

## Shipped map

| Phase | Code |
|---|---|
| F0 | `shared_flags.go` `--mode/--cross-file`; template `{{cross_file_hints}}`; `agent.renderMessages` |
| F1 | `crossfile/{topology,skeleton,cluster,render,parse}`; `cross_phase.go`; `CROSS_*` tasks; `cross_file/common.md` |
| F2 | `crossfile/similarity.go` + cluster union |
| F3 | `crossfile/{apply,verify}.go`; `--apply` / `--apply-run-tests` |

## Follow-ups (optional)
- X3 LLM rewrite on verify fail (true repair loop)
- Session persistence of RefactorPlan artifacts
- Viewer UI for `related_locations`
