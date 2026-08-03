# ADR: Multi-file / Cross-file Refactor Suggestions

| Field | Value |
|---|---|
| Status | **Implemented (F0–F3 skeleton)** — code shipped; apply repair LLM loop is best-effort |
| Date | 2026-08-03 |
| Depends on | `REFACTOR_RULES_OPTIMIZATION_ADR.md` (Phase 1–2 single-file rules) |
| Scope | `internal/refactor`, templates, comment model, context engine, verification loop |
| Problem | Single-file refactor is optimized; **cross-file** duplication, extract-shared, package-level smells are out of scope today |
| Core insight | Bottleneck is **noise + dependency reasoning**, not raw context window (“Lost in the Middle” if dumping many full files) |

---

## 1. Current state (facts)

| Aspect | Behavior today |
|---|---|
| Dispatch | Enumerate files → batch for concurrency → **1 LLM subtask / file** (`refactor/agent.go`) |
| Prompt | Full file in `<current_file_content>`; rule = `ResolveRefactor(path)` |
| Explicit ban | Template: *“Do not comment on code in other files — context tools are reference only.”* |
| Evidence policy | Common rules: no cross-module claims without evidence **in current file** |
| Tools available | `file_read`, `file_find`, `code_search`, `code_comment`, `task_done` (no `file_read_diff` on refactor) |
| Batch strategy | `by-language` groups **scheduling only**, not joint analysis |
| Output model | `code_comment` anchored to **one** file path + line range |

**Gap:** Agent never sees two full files as co-primary subjects; even with `code_search`, policy forbids multi-file findings. Extract-to-shared-module, clone detection, API consolidation across package cannot be first-class products.

---

## 2. Problem classes to solve

| Class | Example | Needs |
|---|---|---|
| **C1 Clone / near-duplicate** | Same validation block in `a.go` and `b.go` | Similarity + multi-anchor finding |
| **C2 Extract shared** | 3 handlers share error-wrap pattern → `pkg/errorsx` | Placement proposal + call-site previews |
| **C3 Package smell** | God package, cyclic imports, layering violation | Graph/import evidence + multi-file list |
| **C4 Consistent rename / API** | Same concept named differently across files | Symbol index + coordinated suggestion |
| **C5 Dead cross-file** | Exported helper unused outside defining file | Call graph / search (static or tool) |

Single-file pipeline handles local subset of C1–C2 **within one file only**.

---

## 3. Neutral option listing

### Option A — Tool-augmented single-file (relax policy only)

- Keep 1 task / file.
- Soften prompt: allow findings that **cite** other files via `code_search` / `file_read`.
- `code_comment` still primary-anchored on current file; related paths in comment body.

### Option B — Two-phase pipeline: Local then Cluster

- **Phase L (local):** existing per-file refactor (unchanged).
- **Phase X (cross):** cluster files → one multi-file LLM task per cluster → cross-file findings.
- Clusters from directory / package / language / import neighborhood / similarity.

### Option C — Similarity-first clusters (static prefilter + LLM)

- Cheap index: function fingerprints (token shingles, AST hash, or minhash of normalized bodies).
- Only clusters with similarity ≥ threshold go to multi-file LLM.
- Highest precision for C1/C2; less for pure architecture (C3).

### Option D — Package / batch joint analysis (no similarity)

- One LLM call over N files in same package (truncated contents).
- Simple; token-heavy; risk of shallow analysis.

### Option E — Graph-aware agent (call graph / imports)

- Build import graph (and optionally approximate call edges via search).
- Agent tasks are **subgraph-scoped** (node + neighbors).
- Strong for C3/C5; heavier infra.

### Option F — Hybrid: C prefilter + B phases + multi-anchor comments

- Local pass unchanged.
- Cross pass only on similarity/import clusters.
- New finding shape: primary path + `related_locations[]`.
- Cross-file rule pack + categories (`cross_duplication`, `extract_shared`, …).

### Option G — External linter / clone tool first (jscpd, PMD CPD, …)

- Run clone detector → feed clone reports into LLM for refactor suggestion only.
- High recall for C1; integration + multi-language support cost.

---

## 4. Criteria mapping

| Criterion | Strongest |
|---|---|
| Precision on clones (C1) | **C**, **G** |
| Extract-shared quality (C2) | **F**, **C+B** |
| Architecture smells (C3) | **E**, **D** |
| Token / cost control | **C**, **G**, then **B** |
| Fit OCR architecture (tools, comments, session) | **B/F** (extends existing agent) |
| Implementation risk | **A** lowest, then **B**, then **C/F** |
| Time-to-value | **A** (days) → **B** (1–2 weeks) → **F** (full product) |

### Trade-offs

| Choose … over … | You give up |
|---|---|
| A over B | Real multi-file rewrites; still biased to one anchor file |
| B over C | Many low-signal clusters (whole package) unless clustering is smart |
| C over D | Setup for fingerprints; miss non-clone package smells |
| D over C | Token budget; weaker clone recall at scale |
| E over B | Build/maintain graph; language coverage |
| G over C | Tooling matrix per language; less control inside OCR |
| F over pure B | Schema + UX complexity for multi-anchor comments |

---

## 5. Contextual recommendation

### Context (OCR)

1. Already has per-file agent, tools, sessions, batch dispatch, refactor rules Phase 1–2.
2. Comment model is path+lines; viewers/skills expect file-local comments.
3. Token thresholds already a concern (compose common+family+lang).
4. Multi-language product — pure AST clone engines hard to own for all langs.
5. Policy currently **forbids** multi-file comments — product decision, not accident.

### Recommended path: **Option F + 3-Layer Framework (Context → Detect/Plan/Transform → Verify)**

Staged delivery maps to both **product modes** and **framework layers**:

| Stage | Product surface | Framework layer focus | Solves |
|---|---|---|---|
| **F0** | `--cross-file=hints` | Layer 2 lite (policy + tools only) | Weak C1/C2 via search |
| **F1** | `--mode=cross` / `full` | Layer 1 **lite** (skeleton+import graph) + Layer 2 **Detect+Architect** (suggest only) + multi-anchor comments | C1/C2/C3 lite |
| **F2** | Similarity clusters | Layer 1 **similarity** + richer Detect input | C1/C2 at scale |
| **F3** | `--apply` + verify | Layer 2 **Transform** + Layer 3 **Verification Loop** | Safe multi-file edit |

**Not first:** full SCIP/LSIF platform for all langs, unconstrained multi-file dump, auto-apply without verify.

### Why this merge

| From original Option F | From 3-Layer Framework (external) |
|---|---|
| Fits OCR agent/session/comments | Names the real bottleneck: **noise + graph**, not window size |
| Opt-in, non-breaking default | Skeleton/topology instead of raw dump |
| Multi-anchor findings | **Split Detect ≠ Plan ≠ Transform** |
| Similarity prefilter | **Deterministic verification** before accept |

---

## 6. Target architecture (v2 — 3-Layer)

```
[ Codebase / path set ]
         │
         ▼
┌────────────────────────────────────────────────────────────┐
│ LAYER 1 — Structural Context Engine                        │
│  • Import/topology graph (JSON)                            │
│  • File/function skeletons (signatures, types, exports)    │
│  • Optional: similarity / symbol hits                      │
│  • Optional later: Tree-sitter, SCIP/LSIF                   │
│  Output: ProjectTopology + SkeletonMap + CodeSlices        │
└────────────────────────────┬───────────────────────────────┘
                             │
         ┌───────────────────┼───────────────────┐
         ▼                   ▼                   ▼
   Phase L (existing)   LAYER 2 — Agent Skill Engine
   per-file local       ┌─────────────────────────┐
                        │ X1 SmellDetector (RO)   │──► SmellReport[]
                        │ X2 RefactorArchitect    │──► RefactorPlan
                        │ X3 CodeTransformer*     │──► diffs / patches
                        └───────────┬─────────────┘
                                    │
                                    ▼
┌────────────────────────────────────────────────────────────┐
│ LAYER 3 — Automated Verification Loop (* F3 / apply mode)  │
│  • Syntax/AST parse  • typecheck/lint  • tests             │
│  • Fail → feedback to Transformer (max retries)            │
│  • Pass → accept session result / optional commit          │
└────────────────────────────────────────────────────────────┘
```

\* OCR **today** stops at suggestions (`code_comment`). X3+Layer3 are the apply path; F1 ships Detect+Architect → comments/plan only.

### 6.1 Layer 1 — Structural Context Engine

**Principle:** Agent must not read 50 full files. Compress first.

| Artifact | Content | Agent use |
|---|---|---|
| **Dependency graph** | File/package nodes; import edges; optional cycle flags | Coupling, cluster seed, topo edit order |
| **Skeletons** | Per-file: imports, type/func signatures, exported symbols — **bodies omitted** | API shape, data clumps, envy signals |
| **Semantic slices** | Full bodies only for high-similarity windows / detector hits | Clone evidence without whole-file dump |
| **Cluster budget** | max files, max skeleton bytes, max slice bytes | Anti “Lost in the Middle” |

#### OCR-realistic Layer 1 ladder (do not block F1 on SCIP)

| Step | Tech | When |
|---|---|---|
| L1a | Path/dir grouping + regex/line import edges (Go/TS/Python first) | F1 |
| L1b | Signature extraction via language-light parsers or `go/parser`, `ts-morph`-like, tree-sitter **optional** | F1–F2 |
| L1c | Text-normalized function shingles / minhash | F2 |
| L1d | SCIP/LSIF or external index when available | F3+ / enterprise |
| L1e | Vector search for semantic near-dup | Optional, not required for v1 |

**Visual payload to LLM (F1 default):**

```text
<project_graph>{ ... imports JSON or mermaid ... }</project_graph>
<file_skeletons>
  path: a.go
  - func ValidateToken(ctx, tok string) error
  - func HandleLogin(...)
</file_skeletons>
<code_slices>  <!-- only hot windows -->
  <slice path="a.go" lines="40-62">...</slice>
  <slice path="b.go" lines="55-78">...</slice>
</code_slices>
```

Full files only when cluster is small enough **or** Architect phase needs them for a shortlisted smell.

### 6.2 Layer 2 — Agent Skill Protocol (3 specialized phases)

Do **not** use one vague skill “find code smells”. Split:

#### X1 — `CrossFileSmellDetector` (read-only)

| | |
|---|---|
| **Input** | `project_graph`, `file_skeletons`, optional `code_slices` |
| **Task** | Patterns only: duplicated flow, data clumps, feature envy / tight coupling, inconsistent APIs, dead exports (if graph allows) |
| **Forbidden** | Local single-file nits (guard clause, rename local) — those stay Phase L |
| **Output** | Strict JSON `SmellReport[]` |

```json
{
  "smell_type": "DUPLICATED_FLOW",
  "severity": "major",
  "confidence": "HIGH",
  "affected_files": ["src/a.ts", "src/b.ts"],
  "evidence": [
    {"path": "src/a.ts", "start_line": 40, "end_line": 62, "note": "validate+normalize"},
    {"path": "src/b.ts", "start_line": 55, "end_line": 78, "note": "same steps"}
  ],
  "extracted_candidate": "ProcessPaymentHandler",
  "target_abstraction": "shared utility / strategy",
  "rule_id": "REF-XDUP-001"
}
```

Smell types (catalog):

| smell_type | Maps to problem class |
|---|---|
| `DUPLICATED_FLOW` | C1 |
| `NEAR_CLONE` | C1 |
| `DATA_CLUMP` | C2 |
| `FEATURE_ENVY` / `TIGHT_COUPLING` | C3 |
| `INCONSISTENT_API` | C4 |
| `DEAD_EXPORT` | C5 |
| `EXTRACT_CANDIDATE` | C2 |

#### X2 — `RefactoringArchitect` (planning)

| | |
|---|---|
| **Input** | SmellReport[] + full code of **affected files only** (+ skeletons of neighbors if needed) |
| **Task** | Non-breaking plan: new module?, interface changes?, **edit order (topo: leaves/base first, callers later)** |
| **Output** | `RefactorPlan` JSON |

```json
{
  "plan_id": "plan-auth-validate",
  "summary": "Extract shared token validation into pkg/auth/validate.go",
  "breaking_change": false,
  "steps": [
    {
      "order": 1,
      "action": "create_file",
      "path": "pkg/auth/validate.go",
      "symbol": "ValidateToken",
      "notes": "unexported helpers stay package-private"
    },
    {
      "order": 2,
      "action": "rewrite_callsite",
      "path": "pkg/auth/a.go",
      "related_paths": ["pkg/auth/validate.go"]
    },
    {
      "order": 3,
      "action": "rewrite_callsite",
      "path": "pkg/auth/b.go"
    }
  ],
  "risks": ["export surface if ValidateToken is public"],
  "test_focus": ["pkg/auth", "TestValidate*"]
}
```

**OCR F1 product mapping:** Architect output is rendered as:

1. Multi-anchor `code_comment` findings (user-visible suggestions), and/or  
2. Session artifact `refactor_plan.json` for later apply.

#### X3 — `CodeTransformerAgent` (execution) — F3

| | |
|---|---|
| **Input** | One plan step + file contents |
| **Task** | Produce unified diff or tool `apply_patch` / write |
| **Constraint** | Only files in the plan step; no drive-by edits |
| **Loop** | On verify fail → re-prompt with tool stderr (max 3) |

### 6.3 Layer 3 — Automated Verification Loop (Safety Net)

**Golden rule:** *Agent proposes → Framework verifies deterministically → fail = self-correct → pass = accept.*

| Check | Tool examples | On failure |
|---|---|---|
| Syntax / parse | language parser, `gofmt` parse | return parse error to X3 |
| Type / lint | `go test` compile, `tsc --noEmit`, `ruff`, `mypy` | structured diagnostics in feedback |
| Tests | unit/integration subset from plan `test_focus` | test output truncated into feedback |
| Diff policy | no files outside plan; optional size cap | reject patch |
| Rollback | git worktree / temp branch | restore on hard fail |

**OCR note:** Review/refactor today is **suggestion-first**. Layer 3 is mandatory for `--apply` / auto-edit modes, optional “verify suggestion compile” later.

### 6.4 Skill Spec (normalized)

```yaml
# cross_file_refactor_evaluator.yaml (conceptual)
name: cross_file_refactor_evaluator
description: >
  Evaluates multi-file clusters for cross-file smells and proposes abstractions.
  Does not perform single-file local refactors.
inputs:
  - name: project_graph
    type: json
  - name: file_skeletons
    type: map
  - name: code_slices
    type: list
    required: false
  - name: cross_file_rules
    type: markdown
phases:
  - id: detect
    role: CrossFileSmellDetector
    tools: [file_read, code_search]   # read-only
    output_schema: SmellReport
  - id: architect
    role: RefactoringArchitect
    tools: [file_read]
    output_schema: RefactorPlan
  - id: transform
    role: CodeTransformer
    tools: [apply_patch, file_read]
    output_schema: PatchSet
    enabled_when: apply_mode
  - id: verify
    role: system
    tools: [syntax_check, typecheck, test_runner]
instructions: |
  1. Use project_graph + skeletons first; load full bodies only for evidence.
  2. Focus ONLY on cross-file boundaries (duplication, extract, coupling, API consistency).
  3. Do NOT emit single-file local optimizations.
  4. Prefer unexported same-package helpers before new public APIs.
  5. One smell group → one finding/plan (no N duplicate comments).
  6. Edit order must be topological when plan has dependencies.
```

### 6.5 Cluster constraints (hard budgets)

| Knob | Suggested default |
|---|---|
| `max_files_per_cluster` | 6–8 |
| `max_skeleton_bytes` | 40–60 KB |
| `max_slice_bytes` | 40–80 KB |
| `max_full_files_in_architect` | 4–6 |
| `max_clusters` | budget-based |
| `min_similarity` (F2) | 0.75–0.85 |
| Skip | generated, vendor; tests optional |

### 6.6 Output model extension (comments + plan)

```json
{
  "path": "pkg/auth/a.go",
  "start_line": 40,
  "end_line": 62,
  "category": "cross_duplication",
  "severity": "major",
  "confidence": "HIGH",
  "message": "Duplicate validation in a.go and b.go; extract ValidateToken",
  "suggestion_code": "... shared helper preview ...",
  "related_locations": [
    {"path": "pkg/auth/b.go", "start_line": 55, "end_line": 78}
  ],
  "refactor_kind": "extract_shared",
  "proposed_symbol": "pkg/auth/validate.go::ValidateToken",
  "plan_id": "plan-auth-validate",
  "smell_type": "DUPLICATED_FLOW"
}
```

Backward compatible: `related_locations` / `plan_id` optional.

### 6.7 Rules pack (new)

`rule_docs/refactoring/cross_file/common.md` + catalog IDs:

| ID | Focus |
|---|---|
| `REF-XDUP-001` | Cross-file duplicated flow / near-clone |
| `REF-XEXTRACT-001` | Extract shared helper/module |
| `REF-XCLUMP-001` | Data clump across APIs |
| `REF-XCOUPLE-001` | Feature envy / tight coupling |
| `REF-XAPI-001` | Inconsistent cross-file API |
| `REF-XDEAD-001` | Dead export (graph-backed) |

### 6.8 CLI surface

```bash
ocr refactor --mode=local              # default: Phase L only
ocr refactor --mode=cross              # Layer1 + X1/X2 → multi-file suggestions
ocr refactor --mode=full               # local then cross
ocr refactor --cross-file=hints        # F0
ocr refactor --mode=cross --apply      # F3: X3 + verify (future)
```

### 6.9 Mapping: external 3-Layer design ↔ OCR building blocks

| Framework piece | Existing OCR | Gap to build |
|---|---|---|
| Context Engine | path enum, `file_read`, `code_search` | skeleton/graph builder, slice selector |
| Smell detector | per-file MAIN_TASK | `CROSS_DETECT_TASK` + strict JSON |
| Architect | per-file `PLAN_TASK` | multi-file `CROSS_ARCHITECT_TASK` + RefactorPlan |
| Transformer | none (comments only) | apply_patch path + worktree |
| Verification | none in refactor agent | pluggable runners by language |
| Rules | single-file compose Phase 1–2 | `cross_file/*` pack |
| Comments | single path | `related_locations`, `plan_id` |

### 6.10 What the external design adds vs what we must not copy blindly

| Keep / adopt | Adapt for OCR | Defer |
|---|---|---|
| Noise-first principle; skeleton payload | Import graph without full SCIP at F1 | Universal Tree-sitter for every language day 1 |
| Detect → Architect → Transform split | Transform only under `--apply` | Force Strategy Pattern as default abstraction |
| Strict JSON schemas | Align with `code_comment` + session artifacts | Replace comments entirely with plan-only UX |
| Verify loop golden rule | Language-pluggable, not hardcode `tsc` | Full monorepo typecheck always |
| Topological edit order | Encode in RefactorPlan.steps | Auto-commit without user opt-in |
| High-sim full bodies only | Shingles first; vectors optional | Require vector DB |

**OCR-specific additions the external note underweighted:**

1. **Coexistence with Phase L** — local and cross must not double-count.  
2. **Multi-language product** — Context Engine must degrade gracefully (skeleton quality ≠ same for Go vs free-form).  
3. **Comment/PR UX** — multi-anchor fan-out for GitHub/skills.  
4. **Rule system** — reuse catalog IDs, priority P0–P2, disabled_refactor_rules.  
5. **Session resume / budget** — cluster ids in session; token telemetry already exists.  
6. **Evidence policy alignment** — cross phase *explicitly* allows multi-file evidence *inside cluster only*.

---

## 7. Adversarial: failure modes (updated)

1. **Token explosion / Lost in the Middle** — dump full package.  
   *Mitigation:* Layer 1 skeleton-first; hard slice budgets; Architect loads full files only for shortlisted smells.

2. **False clone → bad extract** — structural similarity, different domain.  
   *Mitigation:* Detector confidence; Architect must state domain invariant; human preview; verify tests.

3. **Graph incompleteness** — weak import edges → miss C3/C5.  
   *Mitigation:* degrade to dir clusters; label confidence MEDIUM; don’t claim cycles without edges.

4. **Multi-anchor UX break** — tools show only primary path.  
   *Mitigation:* fan-out comments or markdown “Related files”; `plan_id` grouping.

5. **Double billing L + X** — same smell twice.  
   *Mitigation:* X prompt forbids pure-local; dedupe fingerprints; categories split.

6. **Transform without verify** — silent break.  
   *Mitigation:* no default apply; Layer 3 required for apply; max 3 repair loops; worktree rollback.

7. **SCIP/AST big-bang** — never ships.  
   *Mitigation:* L1a/L1b ladder; ship Detect+Architect on light graph first.

8. **Over-abstraction (Strategy everywhere)** — Architect cargo-cult.  
   *Mitigation:* rules prefer simplest extract (same-package helper) before patterns; P2 for design patterns.

---

## 8. Implementation plan (when approved)

### Sprint F0 — Hints only

- [x] Flag `--cross-file=hints` + `--mode`
- [x] Prompt: allow citing other paths; single anchor (`{{cross_file_hints}}`)
- [x] Short cross-file rule appendix (`cross_file/common.md`)
- [x] Tests: mode parse + flag validation

### Sprint F1 — Context lite + Detect + Architect (suggest)

- [x] **Layer 1a:** `ProjectTopology` (dir + import edges)
- [x] **Layer 1b lite:** skeleton extractor
- [x] Cluster builder with skeleton/slice budgets
- [x] `CROSS_DETECT_TASK` → `SmellReport[]` (strict JSON)
- [x] `CROSS_ARCHITECT_TASK` → `RefactorPlan` + multi-anchor comments
- [x] Cross-file rules + catalog IDs in rule pack
- [x] Extend comment model: `related_locations`, `plan_id`, `smell_type`
- [x] Telemetry: `refactor.cross_file.*`
- [x] Unit tests: topology, cluster, schema parse

### Sprint F2 — Similarity Context

- [x] Function-window normalize + shingles / Jaccard
- [x] Cluster edges from similarity; merge with import neighbors
- [x] Golden-style unit test for co-clustering near-duplicates

### Sprint F3 — Transform + Verification

- [x] Apply plan steps with `suggestion_code` + backup/rollback
- [x] Verify: `go/parser` for Go; brace heuristic for others; optional `go test`
- [x] Repair attempts constant (full X3 rewrite LLM deferred — same plan cannot self-heal)
- [x] CLI `--apply` / `--apply-run-tests` gated; default off

### Non-goals (near term)

- Replacing single-file mode
- Mandatory SCIP/LSIF for all languages
- Auto-commit without explicit user action
- Vector DB as hard dependency
- Full X3 LLM patch rewriter after verify fail

---

## 9. Decision

| Item | Choice |
|---|---|
| **Selected** | **Option F + 3-Layer Framework**, staged F0→F1→F2→F3 |
| **Default UX** | `local` only; cross/apply opt-in |
| **Layer 1 v1** | Skeleton + import/dir topology + optional slices — **not** full SCIP |
| **Layer 2 v1** | Detect + Architect → suggestions/plans; Transform later |
| **Layer 3** | Required for apply; optional later for “compile-check suggestion” |
| **Primary anti-noise tactic** | Skeleton/graph first; full code only for evidence |
| **Rejected as v1** | Raw multi-file dump; graph platform big-bang; apply without verify |

---

## 10. Change log

| Date | Change |
|---|---|
| 2026-08-03 | Initial multi-file refactor ADR; recommend staged Option F |
| 2026-08-03 | **v2:** Merge external 3-Layer Framework (Context Engine, Detect/Architect/Transform, Verification Loop); OCR ladder L1a–L1e; skill specs; F3 apply path; adversarial + mapping tables |
| 2026-08-03 | **Implementation:** package `internal/refactor/crossfile`; agent cross phase; CLI `--mode/--cross-file/--apply`; templates CROSS_*; model related_locations; unit tests |
