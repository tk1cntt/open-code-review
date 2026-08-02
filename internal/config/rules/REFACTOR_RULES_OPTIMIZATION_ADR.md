# ADR / Architecture Review: Tối ưu Refactor Rules cho AI Agent

| Field | Value |
|---|---|
| Status | **Phase 1 + Phase 2 implemented** — compose, family, catalog IDs, profiles, disable hooks, telemetry |
| Date | 2026-08-03 |
| Scope | `internal/config/rules/rule_docs/refactoring/*`, `system_rules.go` `ResolveRefactor`, prompt payload refactor agent |
| Related code | `system_rules.go` (`ResolveRefactor`, `LoadDefault`, `mergeWithSystemRefactorRule`), `system_rules.json`, `internal/refactor/agent.go` |
| Author role | Independent system architecture review (4-step process) |

---

## Vấn đề (Problem Statement)

Hệ thống OCR gửi refactor rules tới AI Agent theo path file:

1. `default_refactor_rule` = `refactoring/common.md`
2. `refactor_rule_map` map glob → file ngôn ngữ / config riêng
3. `ResolveRefactor(path)` hiện tại: **first match wins, path rule thay thế (replace) common**, không compose

Hệ quả quan sát được:

- ~40 file markdown, ~125 KB nội dung embed
- Language files lặp lại hàng loạt rule chung (80 LOC, >4 params, naming `is/has/can/should`, dead code, footer severity)
- Các section chỉ có ở `common.md` (Immutability CRITICAL, Sealed Types, Input Validation, DI) **không được gửi** khi path match language file
- Rule prose thuần, không ID, không tier, không confidence gate có cấu trúc
- Agent dễ gợi ý design-level refactor với evidence yếu; prompt có nguy cơ chạm token threshold

**Mục tiêu tối ưu:** tăng chất lượng finding, giảm trùng lặp / drift, giữ hành vi resolve ổn định, có thể kiểm thử và triển khai theo giai đoạn.

---

## Bước 1 — Liệt kê và Phân rã (Neutral Listing)

Chỉ đặc điểm kỹ thuật; **không** đánh giá tốt/xấu.

### Phương án A — Status Quo (Replace semantics)

- Resolve: path rule XOR default common
- Nội dung: mỗi language file tự chứa full categories (Complexity, Naming, …)
- Format: Markdown prose
- Không family layer; không rule ID; không threshold profile
- Compose chỉ tồn tại ở user/project layer (`merge_system_rule`)

### Phương án B — Compose `common ⊕ language` (string concat tại resolve)

- Resolve: nếu match path rule → `DefaultRefactorRule + separator + pathRule`; config-like path có thể skip common
- Language file giữ nguyên hoặc được rút gọn dần
- Format: vẫn Markdown
- Không thay đổi schema JSON map (vẫn 1 path → 1 file)
- Cần test: kích thước prompt, thứ tự section, hash `CanonicalConfig` nếu áp dụng refactor layer

### Phương án C — Delta-only language packs + Family inheritance

- Layers: `common` → `family/{jvm,systems,scripting,web,config}` → `lang/{java,go,…}`
- Resolve: ghép 2–3 layer theo map mở rộng trong `system_rules.json`
- Language file chỉ chứa idiom đặc thù
- Format: Markdown (hoặc Markdown sinh từ layer)
- Cần schema map mới (multi-file per path) hoặc convention include

### Phương án D — Structured rules (YAML/JSON catalog) + Markdown renderer

- Source of truth: catalog có `id`, `category`, `severity_default`, `confidence_min`, `applies`, `trigger`, `action`, `anti_patterns`
- Runtime: filter theo path/lang/tier → render markdown (hoặc inject JSON checklist) vào prompt
- Có thể disable/enable rule theo project config
- Migration: dual-write hoặc convert từ `.md` hiện tại

### Phương án E — Priority tiers + token budget (prompt policy)

- Rules gắn tier P0/P1/P2/P3 (safety → style)
- Agent instruction: ưu tiên P0→P1; cap số finding; deep tier khi còn budget
- Có thể implement bằng annotation trong Markdown hoặc metadata structured
- Không bắt buộc đổi resolve compose

### Phương án F — Threshold profiles + evidence gates + local vs architectural split

- Profile số (max_fn_lines, max_params, max_nesting) theo lang/project
- Evidence gate: chỉ report khi có bằng chứng trong file/context scan
- Phân loại: local-safe-refactor vs architectural-smell (comment-only / low auto-apply)
- Wire profile vào template render hoặc rule text substitution

### Phương án G — Hybrid staged (B + E + F content trước; C/D sau)

- Phase 1: đổi semantics compose + chỉnh nội dung common (tier, evidence) + slim lang dần
- Phase 2: family packs và/hoặc structured catalog
- Giữ Markdown làm delivery format tới LLM trong phase 1
- Test gates theo từng phase

### Phương án H — Tool/linter-first, AI rules tối giản

- Phần lớn dead code / complexity / unused import giao static analysis
- AI rules chỉ còn semantic/design residual
- Cần pipeline tool integration ngoài rule markdown

---

## Bước 2 — Phân tích theo Tiêu chí (Attribute Mapping)

### Tiêu chí đánh giá

| ID | Tiêu chí | Ý nghĩa trong OCR |
|---|---|---|
| T1 | Correctness of rule delivery | Agent có nhận đúng union policy chung + idiom không |
| T2 | Prompt efficiency (tokens) | Bytes rule / tín hiệu hữu ích |
| T3 | Maintainability | Drift giữa 40 file, chi phí thêm ngôn ngữ |
| T4 | Implementation risk & blast radius | Đụng `ResolveRefactor`, embed FS, tests, prompt behavior |
| T5 | Measurability / tunability | Rule ID, disable, severity budget, A/B |
| T6 | False-positive control | Evidence gate, architectural overreach |
| T7 | Time-to-value | Bao lâu có cải thiện observable |

### Ai mạnh nhất theo tiêu chí

| Tiêu chí | Mạnh nhất | Ghi chú kỹ thuật |
|---|---|---|
| T1 Correctness delivery | **B** (compose) / **C** (compose đa tầng) | A fail vì common orphan khi match path |
| T2 Prompt efficiency | **C** (delta) / **H** (ít rule AI) / **E** (gửi ít tier) | D filter tier cũng mạnh nếu catalog gọn |
| T3 Maintainability | **C** + **D** | Single source + inheritance |
| T4 Low implementation risk | **E** (content-only) rồi **B** (1 chỗ resolve) | H/D/C risk cao hơn |
| T5 Measurability | **D** | ID + metadata native |
| T6 FP control | **F** (+ **E**) | Gate + local/arch split |
| T7 Time-to-value | **E/F content in common** → **B** | Không cần schema mới |

### Trade-off matrix (chọn A thay vì B, …)

| Nếu chọn… thay vì… | Mất đi (cụ thể) |
|---|---|
| **A** thay **B** | Mất union common+lang; phải duplicate hoặc chấp nhận mất Immutability/DI/Validation |
| **B** thay **C** | Mất DRY đa ngôn ngữ cùng family (JVM null-safety lặp java/kotlin); vẫn O(N) file lang |
| **B** thay **D** | Mất rule ID ổn định, disable từng rule, filter severity máy được |
| **C** thay **B** | Mất đơn giản resolve 1-file; tăng schema, test tổ hợp layer, rủi ro prompt phình nếu layer chưa delta |
| **D** thay **B/E** | Mất time-to-value ngắn; migration catalog + renderer; dual format risk |
| **E** thay **B** | Mất fix “common orphan”; tier chỉ tối ưu *cách dùng* rule, không fix *rule không được gửi* |
| **F** thay **B** | Tương tự E: chất lượng finding tốt hơn nhưng delivery semantics vẫn sai nếu vẫn replace |
| **H** thay **B–F** | Mất phủ semantic refactor AI; phụ thuộc linter đa ngôn ngữ; effort tích hợp tool lớn |
| **G** thay pure **D** | Mất “big bang” structured ngay; chấp nhận nợ dual-model tạm thời |
| **G** thay pure **A** | Mất zero-code-change; chấp nhận churn test + slim content |

### Trade-off sâu: B vs C vs D (ba ứng viên “architecture”)

```
         Correctness (common∪lang)
                    ▲
                    │     C●
                    │    ╱
                    │  B●
                    │ ╱
              A●────┼────────► Measurability / ID
                    │           D●
                    │
              Low risk / fast
```

- **B:** sửa correctness delivery với blast radius nhỏ (1 function + tests + content slim).
- **C:** tối ưu maintainability dài hạn; cần design map/layer trước.
- **D:** tối ưu vận hành/tuning; chi phí migration cao nhất.

---

## Bước 3 — Đề xuất dựa trên Context (Contextual Recommendation)

### Context thực tế của repo / sản phẩm

| # | Context |
|---|---|
| C1 | Codebase Go; rules embed qua `//go:embed`; đã có test orphan/referenced (`system_rules_test.go`) |
| C2 | ~40 refactor rule files; common ~7KB; lang 1.5–6KB; total ~125KB |
| C3 | Runtime semantics **replace**, trong khi vận hành nội dung **giả định** common + override — lệch kiến trúc/triển khai |
| C4 | Consumer là LLM refactor agent (`internal/refactor/agent.go`); đã có token threshold warning |
| C5 | User/project layer đã có `merge_system_rule` — pattern compose **đã tồn tại** một phần |
| C6 | Config files (json/yaml/pom/…) không phải “function code”; common code-oriented không phải lúc nào cũng áp |
| C7 | Cần cơ sở thực thi + kiểm thử sau này (tài liệu này); ưu tiên đường đi có thể ship theo phase, đo được |
| C8 | Không có bằng chứng team đang xây static-analysis mesh đa ngôn ngữ trong OCR core → H không khớp ngắn hạn |

### Phương án tối ưu theo context: **G (Hybrid staged) với Phase 1 = B + E + F(content)**

**Không chọn A:** mâu thuẫn C3 (correctness delivery).  
**Không chọn pure C ngay:** C7 + C1 — schema multi-layer tăng risk trước khi proof compose 2-layer.  
**Không chọn pure D ngay:** C7 time-to-value; migration catalog trước khi nội dung ổn → rewrite 2 lần.  
**Không chọn H:** C8.

**Chọn G vì:**

1. **Phase 1 sửa lỗi semantics (B)** — đúng root cause “common orphan”, reuse pattern compose đã có ở user merge.
2. **Phase 1 tăng chất lượng signal (E+F trong common)** — tier + evidence + local/arch — không cần schema mới, giảm FP và token waste ngay.
3. **Slim language files** — biến side-effect của B (prompt dài hơn) thành win (prompt ngắn hơn status quo full-dup).
4. **Phase 2 optional C/D** — chỉ khi Phase 1 metrics cho thấy still need (drift family, need rule disable).

### Target architecture (Phase 1)

```
ResolveRefactor(path):
  matched = first path rule or empty
  if matched == empty:
    return common
  if isConfigOrDataRule(matched):   // json, yaml, pom, package.json, ...
    return matched                  // no common code rules
  return common + "\n\n---\n\n" + matched   // code languages

common.md:
  - P0/P1/P2 annotations (or section order = priority)
  - evidence gates + local vs architectural policy
  - single output footer (severity/confidence/rule ref)

lang/*.md:
  - delta only: idioms, anti-patterns, lang thresholds
  - no duplicate Complexity/Naming boilerplate
  - no footer
```

### Acceptance criteria (kiểm thử sau này)

| ID | Criterion | Cách verify |
|---|---|---|
| AC1 | File `.java` nhận **cả** Immutability/DI từ common **và** Optional/try-with-resources từ java | Unit test `ResolveRefactor("src/Foo.java")` contains markers |
| AC2 | File `package.json` / `pom.xml` **không** nhận function-oriented common | Unit test negative contains |
| AC3 | Prompt size median code-lang ≤ baseline replace-full-lang (sau slim) | Benchmark fixture tokens hoặc byte length |
| AC4 | Không regress `LoadDefault` embed; mọi path map file tồn tại | Existing orphan tests + new compose tests |
| AC5 | common có section Evidence / Priority; lang files không lặp footer | Content lint test hoặc checklist review |
| AC6 | Canonical/hash behavior documented nếu refactor rules vào manifest | Test hoặc ADR note |

### Implementation checklist (Phase 1)

- [x] `ResolveRefactor`: compose + allowlist/skiplist config patterns
- [x] Tests: java/go/ts compose; json/yaml/pom no-common; unmatched → common only
- [x] Rewrite `common.md`: tiers P0–P2, evidence gate, local vs arch, one footer
- [x] Slim top languages: `java`, `go`, `python`, `ts_js_tsx_jsx`, `kotlin`, `rust`, `csharp` (delta)
- [x] Slim remaining languages + confirm config files stay standalone
- [ ] Optional: log/telemetry rule bytes per file type
- [ ] Manual golden: 2–3 sample refactor runs before/after (finding quality)
- [x] Refactor template: priority budget instructions (MAIN_TASK + PLAN_TASK)
- [x] Soft AC3: composed payload size guard in unit tests
- [x] Telemetry: `RefactorPayloadStats` + agent event `refactor.rule.payload`

### Phase 2 (implemented)

| Item | Status | Artifact |
|---|---|---|
| Family packs (C) | Done | `rule_docs/refactoring/family/{jvm,systems,scripting,web,dotnet,mobile}.md` |
| Family resolve | Done | `refactor_families` + `refactor_family_map` in `system_rules.json`; compose layer order |
| Structured catalog (D) | Done | `rule_docs/refactoring/catalog.json` + Rule ID Index appended to common |
| Disable by ID | Done | `disabled_refactor_rules` on project `rule.json`; `filterDisabledRefactorRules` |
| Threshold profiles (F) | Done | `profiles.json` + `refactor_profile_map`; `## Threshold Profile` section |
| Forced profile | Done | `refactor_profile` on project `rule.json` |
| Tests | Done | family/profile/disable/stats unit tests |

#### Compose order (code languages)

```
common.md (+ catalog index)
  → ## Threshold Profile
  → ## Family-Specific Refactoring Rules
  → ## Language-Specific Refactoring Rules
```

Config/data paths remain **standalone** (language/config rule only).

#### Project knobs (`.opencodereview/rule.json`)

```json
{
  "disabled_refactor_rules": ["REF-DI-001", "REF-NAME-001"],
  "refactor_profile": "go_idiomatic"
}
```

Highest non-empty layer wins (custom → project → enterprise → global).

---

## Bước 4 — Adversarial Mode (Devil’s Advocate)

Giả sử quyết định ship **G Phase 1 (B + E + F content)**. Các kịch bản **FAIL**:

### Fail scenario 1 — Prompt bloat regression (compose trước khi slim)

- **Điều kiện:** Merge compose vào `ResolveRefactor` nhưng language files vẫn full duplicate.
- **Hậu quả:** Mỗi subtask gửi ~common+lang ≈ 10–13KB rule text; nhiều file chạm token threshold; agent skip hoặc quality drop.
- **Tấn công:** “Bạn vừa làm prompt *dài hơn* status quo.”
- **Mitigation bắt buộc:** Compose **cùng PR** với slim ít nhất top-N languages; gate AC3; feature flag `refactor_compose_common=true` nếu cần rollback.

### Fail scenario 2 — Config/data files bị “function rules” làm bẩn

- **Điều kiện:** Compose mù cho mọi path match, kể cả `**/*.json`, `**/pom.xml`, workflows.
- **Hậu quả:** Agent suggest “extract function”, “DI”, “80-line function” trên JSON/YAML; noise + mất uy tín.
- **Mitigation:** Skiplist / rule class `code` vs `config` (AC2); default: config path rules **standalone**.

### Fail scenario 3 — Semantic conflict common vs lang không có precedence

- **Điều kiện:** common nói “>4 params → struct”; Go delta nói “>5 acceptable với options pattern”; agent nhận cả hai, output mâu thuẫn.
- **Hậu quả:** Finding không nhất quán; user không biết rule nào thắng.
- **Mitigation:** Document precedence: **language delta overrides common on same topic**; optional heading `##### Overrides`; sau này D có `overrides: REF-X`.

### Fail scenario 4 — “Tier labels” bị LLM ignore

- **Điều kiện:** Chỉ ghi `P0/P1` trong markdown, không đổi template instruction.
- **Hậu quả:** Agent vẫn dump 20 minor style findings; E không có effect.
- **Mitigation:** Cập nhật refactor template/prompt system: “Emit at most N findings; sort by tier; suppress P2 unless confidence VERY_HIGH”.

### Fail scenario 5 — CanonicalConfig / reproducibility drift

- **Điều kiện:** Compose runtime nhưng hash/canonical chỉ lưu path rule text hoặc ngược lại; resume/session so khớp rule config sai.
- **Hậu quả:** Cache invalidation sai, khó reproduce run.
- **Mitigation:** Quyết định rõ: canonical string = **resolved composed text** hoặc **(commonSHA, langSHA, mode)**; thêm test.

### Fail scenario 6 — Slim lang xóa mất idiom quý

- **Điều kiện:** Bulk delete “Duplication/Dead Code” sections; vô tình xóa bullet lang-specific nằm chung heading.
- **Hậu quả:** Mất tín hiệu tốt (ví dụ Go `errgroup`, Rust `let else`).
- **Mitigation:** Slim theo checklist category; review diff per file; giữ file “before” trong git history; golden content tests per lang marker strings.

### Rủi ro kiến trúc thường bị bỏ qua

1. **Resolve vs Content coupling:** Đổi semantics resolve mà không version behavior → enterprise user phụ thuộc output cũ bị “silent change”.
2. **Two sources of merge policy:** User `merge_system_rule` compose user⊕system; system compose common⊕lang — hai trục compose khác nhau, dễ confuse docs.
3. **First-match glob order:** Compose không sửa việc `**/*.{ts,js}` vs `**/*.vue` order; sai map vẫn sai rule.
4. **Overfitting common to OOP:** Common giàu DI/sealed/immutability có thể bias agent trên Python scripts / C — cần `applies` hoặc soft language later.
5. **False sense of structure:** Tier trong prose ≠ policy engine; tưởng đã “govern” nhưng chưa enforce.
6. **Test theater:** Chỉ test string contains “Immutability” không chứng minh agent *tuân* rule — cần sample-run quality bar.

---

## Kết luận: Lựa chọn tốt nhất (Final Recommendation)

### Quyết định

| Item | Decision |
|---|---|
| **Selected** | **Phương án G — Hybrid staged** |
| **Phase 1 (ship first)** | **B** compose common⊕code-lang + **E** priority tiers + **F** evidence/local-arch **trong content** + slim deltas |
| **Phase 1 non-goals** | Structured YAML catalog (D), full family tree (C), linter mesh (H) |
| **Phase 2** | C và/hoặc D **khi trigger** (drift / need disable IDs) |
| **Explicitly rejected for now** | A (status quo), pure C/D/H as first move |

### Vì sao đây là “tốt nhất” sau phản biện

Adversarial cho thấy failure mode chính của B là **bloat** và **config pollution** — cả hai **mitigate được trong cùng phase** (slim + skiplist). Failure của E (LLM ignore tier) mitigate bằng **template instruction**, effort nhỏ. Structured catalog (D) giải quyết measurability nhưng **không** fix orphan common nếu vẫn replace; làm D trước = chi phí cao, root cause còn. Family (C) optimize maintainability *sau* khi 2-layer compose đã ổn.

→ **G Phase 1 là điểm Pareto** cho context OCR hiện tại: sửa correctness, kiểm soát FP, giữ blast radius, có AC testable.

### Execution order (bắt buộc)

1. Spec skiplist config patterns + compose algorithm (this ADR)
2. Tests first (AC1–AC2) — red
3. Implement `ResolveRefactor` compose
4. Update `common.md` (tiers, evidence, footer)
5. Slim languages (top-N then rest)
6. Template instruction for tier budget
7. AC3 size check + manual golden runs
8. Only then consider Phase 2 design spike

### Rollback

- Flag hoặc revert single commit resolve + keep slim content (slim alone vẫn positive under replace, dù orphan common trở lại)
- Ưu tiên revert resolve nếu production prompt timeout; content improvements có thể giữ

---

## Phụ lục A — Baseline facts (từ khảo sát code)

```
ResolveRefactor: first match → return path rule only; else DefaultRefactorRule
default_refactor_rule: refactoring/common.md
refactor_rule_map: ~40 globs (languages + config manifests)
compose exists only for user/project merge_system_rule path
```

## Phụ lục B — Config-like patterns (đề xuất skiplist common)

Áp dụng **standalone** (không ghép common) cho các rule file / pattern thuộc nhóm:

- `properties`, `mapper_dao_xml`, `pom_xml`, `build_gradle`
- `package_json`, `cargo_toml`, `composer_json`, `json`
- `github_workflows`, `github_config`, `yaml`
- `po`, `pot`, `protobuf`, `graphql`, `prisma`
- `terraform`, `bicep`
- (rà soát lại `freemarker`, `web` css/html — có thể partial)

Nhóm **compose với common**: java, go, python, ts/js, kotlin, rust, c/cpp, csharp, dart, swift, ruby, php, perl, julia, fsharp, arkts, vue, astro, …

## Phụ lục C — Traceability

| Artifact | Path |
|---|---|
| This ADR | `internal/config/rules/REFACTOR_RULES_OPTIMIZATION_ADR.md` |
| Resolver | `internal/config/rules/system_rules.go` |
| Map | `internal/config/rules/system_rules.json` |
| Rules | `internal/config/rules/rule_docs/refactoring/` |
| Consumer | `internal/refactor/agent.go` |

---

## Change log

| Date | Change |
|---|---|
| 2026-08-03 | Initial independent 4-step architecture review; select G Phase 1; define AC + fail scenarios + execution order |
| 2026-08-03 | **Phase 1 implementation:** `composeRefactorRules` + `isStandaloneRefactorPath` in `system_rules.go`; rewrite `common.md` (P0–P2, evidence); delta-only language rules; template priority budget; unit tests AC1–AC3 |
| 2026-08-03 | **Phase 2 implementation:** family packs + map; `catalog.json` IDs; `profiles.json` + profile map; project `disabled_refactor_rules` / `refactor_profile`; `refactor_resolve.go`; agent payload telemetry; expanded unit tests |
