# Báo Cáo Phân Tích: Lỗi `--apply` Cho Refactor Đa File

**Ngày:** 2026-08-09  
**Trạng thái:** Phân tích — Chưa triển khai  
**Mức độ nghiêm trọng:** 🔴 Cao — `--apply` không hoạt động với refactor cross-file tạo file mới

---

## 1. Tổng Quan Vấn Đề

### 1.1 Mô tả

Khi chạy `ocr refactor --mode=cross --apply`, các comment refactor liên quan đến nhiều file (tạo file mới, gộp chung function, extract shared code) không được áp dụng đúng cách.

| Loại refactor | Expected | Actual |
|---|---|---|
| Đổi tên biến trong 1 file | ✅ Sửa đúng | ✅ Sửa đúng |
| Extract function → file mới | Tạo file mới + sửa file cũ | ❌ Chỉ xóa code file cũ, không tạo file mới |
| Gộp 2 file trùng lặp | Tạo file chung + xóa file cũ | ❌ Xóa code nhưng thiếu file chung |
| Di chuyển code giữa 2 file | Sửa cả 2 file | ❌ Chỉ sửa 1 file, file kia thiếu |

---

## 2. Phân Tích Kiến Trúc Hiện Tại

### 2.1 Pipeline Tổng Thể

```
RefactorAgent.Run()
├── Phase L: Per-file analysis (local)
│   └── executeSubtask() → LLM tool-use loop → CommentCollector
│
└── Phase X: Cross-file analysis (cross)
    ├── BuildClusters()          — Nhóm file theo similarity
    ├── runCrossDetect()         — X1: Phát hiện smell (SmellReport[])
    ├── runCrossArchitect()      — X2: Tạo plan (RefactorPlan[])
    │   └── PlanStep với Action: create_file | rewrite_callsite | modify | delete
    └── runCrossApply()          — F3: Áp dụng plan (chỉ khi --apply)
        └── ApplyPlanSteps()     — Ghi suggestion_code vào file
            └── VerifyFiles()    — Verify + rollback nếu fail
```

### 2.2 Flow `--apply` hiện tại

File `internal/refactor/cross_phase.go` → `runCrossApply()`:

1. **Filter plans có `suggestion_code`** — plans không có code bị bỏ qua
2. **Loop retry** (max `crossfile.MaxRepairAttempts = 3`)
3. **`ApplyPlanSteps()`** (`internal/refactor/crossfile/apply.go`):
   - Backup file cũ (nếu tồn tại)
   - Xóa file (`action=delete`)
   - Ghi file mới (các action khác) — **dùng `suggestion_code` làm toàn bộ nội dung file**
   - `VerifyFiles()` — parse AST cho Go files, balanced braces cho non-Go
   - Nếu verify fail → rollback toàn bộ (khôi phục backup)

**Quan trọng:** Trong `apply.go` dòng 49-51:
```go
if st.SuggestionCode == "" && st.Action != "delete" {
    res.Skipped = append(res.Skipped, st.Path+": no suggestion_code")
    continue  // ← BỎ QUA, không áp dụng
}
```

### 2.3 So sánh hai cơ chế apply

| | Review `--apply` | Refactor `--apply` |
|---|---|---|
| **Cơ chế** | LLM-as-editor (tool-use loop) | Apply trực tiếp từ PlanStep |
| **Tools** | file_edit, file_write, shell_run | Không dùng LLM tools |
| **Tạo file mới** | ✅ file_write tool | ⚠️ Qua suggestion_code (thường rỗng!) |
| **Scope** | Từng file riêng lẻ | Multi-file (có thứ tự) |
| **Đã implement?** | ✅ Đầy đủ trong `agent.go` | ❌ Thiếu Phase X3 Transform |

---

## 3. Root Cause Analysis

### 3.1 Nguyên nhân chính: Thiếu Phase X3 (Transformer)

**ADR F3** mô tả kiến trúc 3-layer: Detect (X1) → Architect (X2) → **Transform (X3)** → Apply → Verify

Tuy nhiên, triển khai hiện tại trong `cross_phase.go` chỉ có:
- `runCrossDetect()` → X1 ✅
- `runCrossArchitect()` → X2 ✅
- `runCrossApply()` → F3 (gọi thẳng sau architect) ⚠️

**Không có `runCrossTransform()` (X3)** — phase chịu trách nhiệm gọi LLM để sinh code thực tế (`suggestion_code`) cho từng `PlanStep`.

**Hậu quả:**
- Architect chỉ tạo plan ở mức kiến trúc (high-level): "cần extract function X sang file Y"
- Architect **không sinh `suggestion_code` đầy đủ** cho từng step
- Step `create_file` cần TOÀN BỘ nội dung file mới — architect prompt hiện tại không được thiết kế để làm việc này
- `ApplyPlanSteps()` bỏ qua tất cả step không có `suggestion_code` → kết quả: `"nothing applied"`

### 3.2 Minh họa lỗi

**Scenario:** Extract function `ValidateEmail` từ `user.go` và `admin.go` sang `validator.go` mới.

Architect plan output:
```json
{
  "plan_id": "P1",
  "summary": "Extract duplicated ValidateEmail to shared validator package",
  "steps": [
    {"order": 1, "action": "create_file", "path": "pkg/validator/validator.go", "suggestion_code": ""},
    {"order": 2, "action": "modify",       "path": "pkg/user/user.go",        "suggestion_code": ""},
    {"order": 3, "action": "modify",       "path": "pkg/admin/admin.go",      "suggestion_code": ""}
  ]
}
```

**Kết quả `--apply`:**
1. Step 1 (`create_file`): `suggestion_code` rỗng → **SKIP** (không tạo `validator.go`)
2. Step 2 (`modify`): `suggestion_code` rỗng → **SKIP** (không sửa `user.go`)
3. Step 3 (`modify`): `suggestion_code` rỗng → **SKIP** (không sửa `admin.go`)

→ **Kết quả:** `"nothing applied (no suggestion_code on plan steps)"` — không file nào được tạo hay sửa.

### 3.3 Góc nhìn khác: `PlansToComments` trong `parse.go`

```go
// Chỉ lấy suggestion_code từ Step[0]
suggestion := ""
if len(steps) > 0 {
    suggestion = steps[0].SuggestionCode
}
// ...
out = append(out, CommentOut{
    ...
    SuggestionCode: suggestion,  // ← Chỉ từ step đầu tiên
    ...
})
```

Các step sau (create_file, modify) không được truyền `suggestion_code` vào output.

### 3.4 Các vấn đề phụ

1. **Không có import management**: Khi tạo file mới + sửa file cũ (thêm import), không có cơ chế tự động thêm import
2. **Không đảm bảo thứ tự build**: File mới được tạo nhưng file cũ đã bị xóa code → undefined symbols
3. **Rollback không hoàn hảo**: File mới tạo (`create_file`) không có backup để rollback, nhưng code hiện tại xử lý được việc này (backup với `data: nil` → xóa file khi rollback)

---

## 4. Giải Pháp Đề Xuất

### 4.1 Phương án A: Thêm Phase X3 Transform (⭐ Khuyến nghị)

Triển khai đầy đủ X3 Transformer như mô tả trong ADR F3.

**Pipeline mới:**
```
runCrossArchitect() → RefactorPlan[]
    ↓
runCrossTransform() → RefactorPlan[] (có suggestion_code đầy đủ)
    ↓  
runCrossApply() → áp dụng + verify
```

**Chi tiết X3 Transform:**
- Input: `RefactorPlan` + `Cluster` (file contents)
- Với mỗi step, gọi LLM để sinh code thực tế:
  - `create_file`: sinh toàn bộ nội dung file mới
  - `modify`: sinh toàn bộ nội dung file sau khi sửa
  - `rewrite_callsite`: sinh nội dung đã sửa
  - `delete`: không cần code (giữ nguyên empty)
- Output: `RefactorPlan` với `suggestion_code` được điền đầy đủ
- Dùng prompt riêng: `CROSS_TRANSFORM_TASK`

| Ưu điểm | Nhược điểm |
|---|---|
| Giải quyết triệt để vấn đề thiếu code | Tốn thêm LLM calls (1 call/plan) |
| Phù hợp với kiến trúc ADR đã định | Cần thêm prompt template mới |
| Có thể verify code trước khi apply | Thời gian phát triển: ~3-5 ngày |
| Tận dụng LLM để sinh code chất lượng cao | |

### 4.2 Phương án B: Dùng LLM-as-editor cho cross-file

Dùng cơ chế giống Review `--apply`: LLM tool-use loop với `file_edit` + `file_write` + `shell_run`.

| Ưu điểm | Nhược điểm |
|---|---|
| Tận dụng code có sẵn (`apply_task_system.md`) | Tốn nhiều LLM calls hơn |
| LLM tự quyết định cách sửa file | Không đảm bảo thứ tự thực thi giữa các file |
| Đã có E2E tests (`apply_e2e_test.go`) | Prompt hiện tại không hỗ trợ multi-file orchestration |

### 4.3 Phương án C: Hybrid — X3 nhẹ + atomic apply

Kết hợp X3 Transform đơn giản (1 LLM call cho toàn bộ plan) + cải thiện `ApplyPlanSteps` để xử lý atomic multi-file.

| Ưu điểm | Nhược điểm |
|---|---|
| Ít LLM calls hơn Phương án A | Chất lượng code có thể thấp hơn |
| An toàn hơn nhờ dry-run | Import management phức tạp |

### 4.4 So sánh tổng hợp

| Tiêu chí | A: X3 Transform | B: LLM-as-editor | C: Hybrid |
|---|---|---|---|
| Độ hoàn thiện | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐ |
| Đúng kiến trúc ADR | ✅ | ❌ | ⚠️ |
| Số LLM calls bổ sung | N calls (1/plan) | M calls (1/file) | 1 call/plan |
| An toàn (atomic) | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Thời gian phát triển | 3-5 ngày | 1-2 ngày | 2-3 ngày |
| Rủi ro chính | Prompt phức tạp | Sai thứ tự file | Import management |

---

## 5. Kế Hoạch Triển Khai (Phương Án A — Khuyến Nghị)

### 5.1 Giai đoạn 1: Cơ sở hạ tầng (1-2 ngày)

| # | Task | File | Mô tả |
|---|---|---|---|
| 1.1 | Thêm `cross_transform_task` vào config | `refactor_template.json` | Section mới với messages system/user |
| 1.2 | Thêm field vào struct | `template.go` | `CrossTransformTask *LlmConversation` |
| 1.3 | Tạo prompt transform system | `prompts/transform_task_system.md` | Hướng dẫn LLM sinh code từ plan |
| 1.4 | Tạo prompt transform user | `prompts/transform_task_user.md` | Template với `{{plan_json}}`, `{{file_contents}}` |
| 1.5 | Thêm validation | `template.go` | Validate `CrossTransformTask` không nil khi `mode=cross` |

### 5.2 Giai đoạn 2: X3 Transform Engine (1-2 ngày)

| # | Task | File | Mô tả |
|---|---|---|---|
| 2.1 | Tạo `runCrossTransform()` | `cross_phase.go` | Gọi LLM với CROSS_TRANSFORM_TASK, parse kết quả |
| 2.2 | Tạo `ParseTransformedPlan()` | `parse.go` | Parse JSON output từ transformer |
| 2.3 | Tạo `RenderTransformPrompt()` | `render.go` | Render prompt context cho transformer |
| 2.4 | Kết nối vào pipeline | `cross_phase.go` | Chèn `runCrossTransform()` giữa architect và apply |
| 2.5 | Thêm X3 rules (optional) | `rules/refactor_resolve.go` | Load `CROSS_TRANSFORM_RULES` nếu có |

### 5.3 Giai đoạn 3: Cải thiện Apply (1 ngày)

| # | Task | File | Mô tả |
|---|---|---|---|
| 3.1 | Pre-apply validation | `apply.go` | Kiểm tra tất cả step có `suggestion_code` TRƯỚC KHI apply |
| 3.2 | Cải thiện error messages | `apply.go` | Báo rõ step nào thiếu code, step nào fail |
| 3.3 | ValidatePlanSteps() | `apply.go` | Hàm riêng kiểm tra readiness của plans |

### 5.4 Giai đoạn 4: Testing (1 ngày)

| # | Task | File | Mô tả |
|---|---|---|---|
| 4.1 | Unit tests cho X3 | `crossfile/transform_test.go` | Test parse, render, transform flow |
| 4.2 | E2E test multi-file | `apply_e2e_test.go` | Test: extract → create file → modify callers |
| 4.3 | Test rollback scenarios | `apply_test.go` | Test rollback khi verify fail |
| 4.4 | Integration test | `cross_phase_test.go` | Full pipeline detect → architect → transform → apply |

### 5.5 Tổng thời gian ước tính: **4-6 ngày**

---

## 6. Chi Tiết Kỹ Thuật

### 6.1 Prompt X3 Transform (đề xuất)

```markdown
## Role
You are a code transformation engine. You receive a refactoring plan and source files.
Generate the COMPLETE implementation code for each step in the plan.

## Input
- Refactoring plan (JSON with steps)
- Source file contents (files affected by the plan)

## Instructions
For each plan step:
1. **create_file**: Write COMPLETE file content (imports, package declaration, full implementation)
2. **modify**: Write COMPLETE modified file content (ENTIRE file after changes)
3. **rewrite_callsite**: Write COMPLETE content of the affected section
4. **delete**: No code needed (leave suggestion_code empty)

## Output Format
Return a JSON array of plans with suggestion_code filled:
{
  "plans": [{
    "plan_id": "...",
    "steps": [
      {"order": 1, "action": "create_file", "path": "...", "suggestion_code": "<FULL FILE>"},
      ...
    ]
  }]
}

## Rules
- suggestion_code must be COMPLETE, COMPILABLE code
- Include ALL imports needed
- Maintain consistent code style with the existing codebase
- Do NOT use placeholders or "..." — write actual implementation
```

### 6.2 Cấu trúc dữ liệu bổ sung

```go
// cross_phase.go — hàm mới
func (a *Agent) runCrossTransform(
    ctx context.Context, 
    c crossfile.Cluster, 
    plans []crossfile.RefactorPlan,
    crossRules string,
) ([]crossfile.RefactorPlan, error) {
    // 1. Render prompt với plans + file contents
    // 2. Gọi LLM
    // 3. Parse kết quả → plans với suggestion_code đã điền
    // 4. Return
}
```

### 6.3 Pre-apply validation

```go
// apply.go — hàm mới
func ValidatePlanSteps(plans []RefactorPlan) error {
    for _, p := range plans {
        for _, s := range p.Steps {
            if s.SuggestionCode == "" && s.Action != "delete" {
                return fmt.Errorf(
                    "plan %q step %d (%s %s): missing suggestion_code — "+
                    "run X3 transform before apply", 
                    p.PlanID, s.Order, s.Action, s.Path)
            }
        }
    }
    return nil
}
```

---

## 7. File Cần Thay Đổi

### 7.1 File mới cần tạo

| File | Mô tả |
|---|---|
| `internal/config/template/prompts/transform_task_system.md` | System prompt cho X3 |
| `internal/config/template/prompts/transform_task_user.md` | User prompt template |
| `internal/refactor/crossfile/transform_test.go` | Unit tests cho X3 |

### 7.2 File cần sửa

| File | Thay đổi | Mức độ |
|---|---|---|
| `internal/config/template/refactor_template.json` | Thêm `cross_transform_task` section | Nhỏ |
| `internal/config/template/template.go` | Thêm field `CrossTransformTask`, update Load/Validate | Trung bình |
| `internal/refactor/cross_phase.go` | Thêm `runCrossTransform()`, sửa pipeline | Lớn |
| `internal/refactor/crossfile/apply.go` | Thêm `ValidatePlanSteps()`, cải thiện messages | Nhỏ |
| `internal/refactor/crossfile/render.go` | Thêm `RenderTransformPrompt()` | Trung bình |
| `internal/refactor/crossfile/parse.go` | Thêm `ParseTransformedPlans()` | Trung bình |

---

## 8. Rủi Ro & Giảm Thiểu

| Rủi ro | Xác suất | Tác động | Giảm thiểu |
|---|---|---|---|
| LLM sinh code sai syntax | Trung bình | Cao | Verify AST parse trước apply |
| Token limit khi sinh nhiều file | Trung bình | Trung bình | Giới hạn plan size, tách plan lớn |
| Import paths sai | Cao | Trung bình | Post-apply `goimports` |
| Thứ tự apply gây lỗi build | Thấp | Cao | Dry-run atomic, rollback tự động |
| Performance (thêm LLM calls) | — | Thấp | Chỉ gọi X3 khi có create_file/modify |

---

## 9. Kết Luận

### 9.1 Tóm tắt

Vấn đề `--apply` không hoạt động cho refactor đa file bắt nguồn từ **thiếu Phase X3 Transformer** trong pipeline cross-file:

1. **Architect (X2)** chỉ tạo plan kiến trúc, không sinh code thực tế (`suggestion_code` rỗng)
2. **`ApplyPlanSteps()`** yêu cầu `suggestion_code` nhưng không có phase nào điền code này
3. **Kết quả:** tất cả step `create_file`/`modify` bị skip → `"nothing applied"`

### 9.2 Khuyến nghị

| Ưu tiên | Hành động | Effort |
|---|---|---|
| 🔴 Ngay | Thêm validation sớm — báo lỗi rõ ràng thay vì "nothing applied" | 1-2 giờ |
| 🟡 Tuần này | Triển khai Phương án A — Phase X3 Transform | 4-6 ngày |
| 🟢 Sau | Cải thiện atomic apply với dry-run | 1-2 ngày |

### 9.3 Impact

- **Trước sửa:** `ocr refactor --mode=cross --apply` không hoạt động với refactor tạo file mới → mất code
- **Sau sửa:** Pipeline hoạt động end-to-end: detect → architect → **transform** → apply → verify
## 10. Kiến Trúc Độc Lập — Big Review (4 Bước)

### 10.1 Bước 1: Liệt Kê và Phân Rã (Neutral Listing)

Dưới đây là **7 phương án** để giải quyết vấn đề `--apply` cho refactor đa file (extract code → tạo file mới → sửa file cũ). Mỗi phương án được mô tả thuần túy về kỹ thuật, không đánh giá tốt/xấu.

#### P1: X3 Transform — LLM sinh code riêng cho từng PlanStep (đề xuất ban đầu)

| Đặc điểm | Mô tả |
|---|---|
| Cơ chế | Gọi LLM riêng cho mỗi `RefactorPlan`, prompt chứa toàn bộ plan + file contents, yêu cầu LLM sinh `suggestion_code` cho từng `PlanStep` |
| Số LLM call | 1 call/plan (có thể tách thành 1 call/step nếu plan lớn) |
| Output | `RefactorPlan` với `suggestion_code` được điền đầy đủ cho mỗi step |
| Apply sau đó | `ApplyPlanSteps()` ghi trực tiếp (không cần LLM nữa) |
| Code cần viết | Prompt mới (`transform_task_system.md`), `runCrossTransform()`, parsing |

#### P2: LLM-as-Editor — tool-use loop giống Review --apply

| Đặc điểm | Mô tả |
|---|---|
| Cơ chế | Chuyển `RefactorPlan` → comment format → đưa vào APPLY_TASK prompt → LLM dùng `file_edit` + `file_write` + `shell_run` tự sửa từng file |
| Số LLM call | 1 tool-use loop (nhiều round) cho toàn bộ plan |
| Output | File được sửa trực tiếp bởi LLM qua tool calls |
| Apply sau đó | Không cần — LLM đã tự apply |
| Code cần viết | Convert plans → apply_comments JSON, tái sử dụng `executeApplyPhase()` |

#### P3: Differential Patching — sinh diff/patch thay vì full file

| Đặc điểm | Mô tả |
|---|---|
| Cơ chế | X3 sinh unified diff (diff -u) cho từng file thay vì toàn bộ nội dung. Apply dùng `git apply` hoặc manual patch |
| Số LLM call | 1 call/plan |
| Output | Diff string cho mỗi file |
| Apply sau đó | `git apply` hoặc manual patching từng file |
| Code cần viết | Prompt yêu cầu diff format, `applyPatch()` function |

#### P4: Symbolic Transformation — AST-based thay vì text-based

| Đặc điểm | Mô tả |
|---|---|
| Cơ chế | Dùng Go AST parser để phân tích code, sinh transformation rules (move function X từ A.go sang B.go), apply bằng code (không LLM) |
| Số LLM call | 0 cho apply phase (LLM chỉ dùng cho detect + architect) |
| Output | AST transformation rules (MoveFunction, CreateFile, AddImport...) |
| Apply sau đó | Go code thực hiện AST manipulation + code generation |
| Code cần viết | AST engine cho Go, transformation rules, code formatter |

#### P5: Two-Phase Hybrid — Architect sinh code thô → Editor refine

| Đặc điểm | Mô tả |
|---|---|
| Cơ chế | Phase 1: Architect (X2) được yêu cầu sinh `suggestion_code` thô kèm plan. Phase 2: Editor agent (P2-style) đọc code thô, refine, và apply |
| Số LLM call | 2 calls: 1 architect (có code thô) + 1 editor loop |
| Output | Plan có code thô → Editor refine → file đã sửa |
| Apply sau đó | Editor đã apply, hoặc cần thêm bước verify |
| Code cần viết | Thay đổi architect prompt, editor prompt, kết nối 2 phase |

#### P6: Git Worktree Sandbox — apply trong sandbox, commit nếu pass

| Đặc điểm | Mô tả |
|---|---|
| Cơ chế | Tạo git worktree tạm, apply toàn bộ changes ở đó, chạy full test suite. Nếu pass → merge vào branch chính. Nếu fail → discard worktree |
| Số LLM call | Tùy chọn: có thể kết hợp với P1, P2, hoặc P5 |
| Output | Worktree với changes đã apply |
| Apply sau đó | `git merge` hoặc `git cherry-pick` |
| Code cần viết | Worktree manager, test runner, merge logic |

#### P7: Incremental Apply với Checkpoint — từng step một, checkpoint sau mỗi step

| Đặc điểm | Mô tả |
|---|---|
| Cơ chế | Thay vì apply tất cả step rồi verify, apply từng step → verify ngay → nếu pass thì checkpoint (commit tạm), nếu fail thì rollback step đó và skip |
| Số LLM call | Tùy chọn: có thể kết hợp với P1, P2 |
| Output | Các step thành công được áp dụng, step fail bị skip |
| Apply sau đó | Từng step đã được áp dụng incremental |
| Code cần viết | Checkpoint manager (git commit --allow-empty), step-by-step loop |

---

### 10.2 Bước 2: Phân Tích Theo Tiêu Chí (Attribute Mapping)

#### Tiêu chí 1: Chất lượng code sinh ra (Generated Code Quality)

Code sinh ra có biên dịch được không? Có giữ nguyên style không? Có đầy đủ import không?

**Mạnh nhất: P4 (Symbolic Transformation)** — AST manipulation đảm bảo code luôn valid về mặt cú pháp, import được quản lý tự động, không phụ thuộc vào "đoán" của LLM.

**Yếu nhất: P1 (X3 Transform)** — LLM sinh toàn bộ nội dung file, rủi ro thiếu import, sai syntax, sai style.

**Xếp hạng (từ mạnh đến yếu):** P4 > P5 > P2 > P6/P7 (tùy base) > P1 > P3

#### Tiêu chí 2: Chi phí vận hành (Operational Cost — LLM tokens)

**Mạnh nhất: P4 (Symbolic)** — 0 LLM call cho apply phase. Chỉ tốn CPU cho AST manipulation.

**Yếu nhất: P2 (LLM-as-Editor)** — Mỗi tool-use loop có thể 10-15 rounds, mỗi round gửi toàn bộ context. Token cost cao nhất.

**Xếp hạng (từ rẻ đến đắt):** P4 > P7 > P1 > P3 > P6 > P5 > P2

#### Tiêu chí 3: Độ an toàn / rủi ro mất code (Safety / Rollback Integrity)

**Mạnh nhất: P6 (Git Worktree Sandbox)** — Changes nằm trong sandbox, chỉ merge khi pass toàn bộ test. Không thể mất code trên branch chính.

**Yếu nhất: P2 (LLM-as-Editor)** — LLM tự quyết định sửa file, có thể sửa sai file, sai scope, hoặc bỏ sót step.

**Xếp hạng (từ an toàn đến rủi ro):** P6 > P7 > P1 > P5 > P3 > P4 > P2

#### Tiêu chí 4: Khả năng tổng quát hóa — hỗ trợ đa ngôn ngữ (Multi-language Support)

**Mạnh nhất: P2 / P1 / P3** — Hoàn toàn text-based, hoạt động với mọi ngôn ngữ. LLM tự hiểu syntax của ngôn ngữ đó.

**Yếu nhất: P4 (Symbolic Transformation)** — Yêu cầu viết AST engine riêng cho từng ngôn ngữ. Go có sẵn, nhưng Python, TypeScript, Rust... cần engine khác.

**Xếp hạng (từ đa năng đến giới hạn):** P2 = P1 = P3 = P5 > P6 = P7 > P4

#### Tiêu chí 5: Khả năng bảo trì và mở rộng (Maintainability / Extensibility)

**Mạnh nhất: P1 (X3 Transform)** — Prompt-based, dễ điều chỉnh bằng cách sửa prompt. Thêm rule mới = thêm text vào system prompt. Không cần code mới.

**Yếu nhất: P4 (Symbolic)** — Mỗi rule transformation mới cần code Go mới. AST manipulation phức tạp, dễ gây regression.

**Xếp hạng (từ dễ bảo trì đến khó):** P1 > P5 > P3 > P2 > P7 > P6 > P4

#### Tiêu chí 6: Thời gian phát triển (Development Time)

**Mạnh nhất (nhanh nhất): P5 (Two-Phase Hybrid)** — Tận dụng architect có sẵn + editor có sẵn (`executeApplyPhase`). Chỉ cần thay đổi prompt + kết nối.

**Yếu nhất (chậm nhất): P4 (Symbolic Transformation)** — Cần viết AST engine, transformation rules, code formatter. Hàng tháng, không phải hàng ngày.

**Xếp hạng (từ nhanh đến chậm):** P5 > P2 > P1 > P7 > P3 > P6 > P4

#### Bảng tổng hợp xếp hạng

| Tiêu chí | P1: X3 | P2: Editor | P3: Diff | P4: AST | P5: Hybrid | P6: Sandbox | P7: Checkpoint |
|---|---|---|---|---|---|---|---|
| Code Quality | ⭐⭐ | ⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ | (tùy base) | (tùy base) |
| Token Cost | ⭐⭐⭐ | ⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐ | (tùy base) | ⭐⭐⭐ |
| Safety | ⭐⭐⭐ | ⭐ | ⭐⭐ | ⭐ | ⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐ |
| Multi-language | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐⭐⭐⭐ |
| Maintainability | ⭐⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ | ⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐⭐⭐ |
| Dev Speed | ⭐⭐⭐ | ⭐⭐⭐⭐ | ⭐⭐⭐ | ⭐ | ⭐⭐⭐⭐⭐ | ⭐⭐ | ⭐⭐⭐ |
| **TỔNG** | **21** | **17** | **18** | **17** | **22** | **16** | **19** |

#### Phân tích Trade-off

**Nếu chọn P5 (Hybrid) thay vì P1 (X3 Transform):**
- **Được:** Dev speed nhanh hơn (5/5 vs 3/5), tận dụng code có sẵn (`executeApplyPhase`, `apply_task_system.md`)
- **Mất:** Không đúng kiến trúc ADR F3 (3-layer: detect → architect → transform), architect prompt phải thay đổi để sinh code thô (tăng token architect)

**Nếu chọn P1 (X3 Transform) thay vì P5 (Hybrid):**
- **Được:** Đúng kiến trúc ADR, separation of concerns rõ ràng (architect chỉ plan, transformer chỉ code), maintainability cao nhất
- **Mất:** Phát triển chậm hơn, phải viết prompt + parsing mới, architect không cần thay đổi nhưng transformer phải đủ mạnh

**Nếu chọn P6 (Sandbox) thay vì P1/P5:**
- **Được:** Safety tuyệt đối (5/5), không lo mất code
- **Mất:** Không tự giải quyết vấn đề thiếu code — sandbox chỉ là cơ chế an toàn bọc ngoài, vẫn cần P1/P2/P5 để sinh code

**Nếu chọn P4 (AST) thay vì text-based (P1/P2/P3/P5):**
- **Được:** Code quality tuyệt đối, token cost = 0 cho apply
- **Mất:** Multi-language support (1/5), maintainability thấp nhất (1/5), dev speed chậm nhất (1/5)

---

### 10.3 Bước 3: Đề Xuất Dựa Trên Context (Contextual Recommendation)

#### Context của dự án open-code-review

| Yếu tố | Thực tế |
|---|---|
| **Ngôn ngữ chính** | Go 1.25.5 |
| **Hỗ trợ đa ngôn ngữ** | Có — hệ thống rules hỗ trợ ~80 ngôn ngữ qua `rule_docs/` |
| **Codebase hiện tại** | Đã có X1 (detect), X2 (architect), `ApplyPlanSteps`, `executeApplyPhase`, `file_edit`, `file_write` |
| **Pattern đã dùng** | Backup-then-mutate, defer rollback, verify-then-commit, MainLoopStop enum, tool-use loop qua `llmloop.Runner` |
| **Thời gian yêu cầu** | Chưa có deadline cứng, nhưng hệ thống đang thiếu chức năng |
| **ADR hiện tại** | ADR F3 định nghĩa 3-layer: Detect(X1) → Architect(X2) → Transform(X3) → Verify |
| **Team** | Go developers, đã quen với prompt engineering và LLM tool-use |
| **Mức độ rủi ro chấp nhận được** | Thấp — `--apply` là destructive operation, cần safety cao |

#### Phân tích context

1. **Hỗ trợ ~80 ngôn ngữ** → P4 (AST) bị loại ngay vì không scale được. Mỗi ngôn ngữ cần AST engine riêng. P1/P2/P3/P5 đều text-based → hỗ trợ mọi ngôn ngữ.

2. **Đã có `executeApplyPhase` + `file_edit` + `file_write`** → P2 và P5 có lợi thế tái sử dụng code. P1 yêu cầu viết mới toàn bộ X3 transform.

3. **Pattern backup-then-mutate đã dùng** → P6 (sandbox) và P7 (checkpoint) phù hợp với philosophy hiện tại.

4. **ADR F3 đã định nghĩa 3-layer** → P1 (X3 Transform) là phương án duy nhất tuân thủ đúng kiến trúc đã được phê duyệt trong ADR.

5. **Safety là yêu cầu cao** → cần kết hợp P1/P5 với P6 hoặc P7.

#### Đề xuất: Phương án Lai (P1 + P6 + P7) — "X3 Transform + Sandbox + Incremental Checkpoint"

**Đây là phương án tối ưu nhất dựa trên context.**

**Kiến trúc:**
```
runCrossArchitect() → RefactorPlan[]
    ↓
runCrossTransform() (X3) — P1: LLM sinh suggestion_code cho từng step
    ↓
ValidatePlanSteps() — kiểm tra tất cả code đã sẵn sàng
    ↓
ApplyPlanStepsIncremental() — P7: từng step một
    ├── Step 1: áp dụng → verify ngay → OK thì checkpoint (git commit tạm)
    ├── Step 2: áp dụng → verify ngay → FAIL thì rollback step này, skip
    ├── Step 3: ...
    └── Nếu tất cả OK: merge checkpoint → branch chính
    └── Nếu có step fail: các step OK vẫn được giữ, step fail bị skip
```

**Lý do chọn:**

1. **P1 (X3)** vì tuân thủ ADR F3, separation of concerns (architect plan, transformer code), maintainability cao nhất (21/30 điểm).

2. **P6 (Sandbox)** vì safety — tất cả thay đổi được thực hiện trong git worktree hoặc branch tạm, chỉ merge khi pass toàn bộ verify.

3. **P7 (Incremental Checkpoint)** vì resilience — nếu 1 step fail, các step khác vẫn thành công. Không bị "all or nothing". Phù hợp với pattern backup-then-mutate đã có.

4. **Tổng điểm tổ hợp: 21 + safety của P6 + resilience của P7 = tối ưu toàn diện**

---

### 10.4 Bước 4: Chế Độ Phản Biện Nghịch Đảo (Adversarial Mode)

**Giả định:** Tôi quyết định chọn **Phương án Lai (P1 + P6 + P7)** như đề xuất ở Bước 3.

**Với tư cách Devil's Advocate, tôi sẽ tấn công lựa chọn này.**

---

#### 🔴 Kịch bản thất bại #1: "X3 Transformer sinh code không đủ tốt, verify fail liên tục"

**Mô tả:** LLM trong X3 Transform được yêu cầu sinh TOÀN BỘ nội dung file mới. Với file phức tạp (nhiều import, dependency nội bộ), LLM có thể:
- Thiếu import package nội bộ → `go vet` fail
- Sai type signature → compile error
- Thiếu dependency injection → runtime error (không phát hiện khi verify)

**Hậu quả:** Với P7 (incremental), step create_file fail → skip → các step modify sau đó cũng fail vì file mới không tồn tại → **toàn bộ plan thất bại, không step nào được áp dụng.**

**Xác suất:** Trung bình-Cao với codebase phức tạp. LLM không có context về toàn bộ dependency graph.

**Giảm thiểu:** Thêm "dependency-aware prompting" — inject import graph của toàn bộ project vào X3 prompt. Nhưng điều này làm tăng token cost đáng kể.

---

#### 🔴 Kịch bản thất bại #2: "Checkpoint contamination — step sau phụ thuộc step trước đã fail"

**Mô tả:** P7 áp dụng từng step, skip step fail. Vấn đề: step 2 (modify `user.go` để import `validator.go`) phụ thuộc vào step 1 (create `validator.go`). Nếu step 1 fail và bị skip, step 2 vẫn chạy và thêm import đến một package không tồn tại → **codebase bị hỏng.**

**Kịch bản cụ thể:**
```
Plan: [create validator.go, modify user.go (import validator), modify admin.go (import validator)]
Step 1: create validator.go → LLM sinh code sai syntax → verify fail → SKIP
Step 2: modify user.go → thêm import "pkg/validator" → verify OK → CHECKPOINT
Step 3: modify admin.go → thêm import "pkg/validator" → verify OK → CHECKPOINT

Kết quả: user.go và admin.go import package không tồn tại → compile error toàn project!
```

**Hậu quả:** Tệ hơn cả "all or nothing" — codebase bị corrupt một phần.

**Xác suất:** Cao. Đây là vấn đề cố hữu của incremental apply với dependency giữa các step.

**Giảm thiểu:** Trước khi apply, xây dựng dependency graph giữa các step. Nếu step N phụ thuộc step M và M fail, tự động skip N. Nhưng điều này làm tăng độ phức tạp đáng kể.

---

#### 🔴 Kịch bản thất bại #3: "Sandbox merge conflict — changes không merge được vào branch chính"

**Mô tả:** P6 dùng git worktree hoặc branch tạm. Trong thời gian apply (có thể vài phút do LLM calls), developer khác có thể push changes vào branch chính. Khi merge sandbox → main, conflict xảy ra.

**Kịch bản cụ thể:**
1. T0: Tạo sandbox từ `main` (commit A)
2. T1-T5: X3 Transform + Apply trong sandbox
3. T3: Developer khác push commit B vào `main`
4. T5: Merge sandbox → main → **CONFLICT: `user.go` đã bị sửa bởi cả sandbox và commit B**

**Hậu quả:** Merge conflict, cần người giải quyết thủ công. Mất tính tự động của `--apply`.

**Xác suất:** Thấp với team nhỏ, trung bình với team lớn hoặc CI/CD pipeline.

**Giảm thiểu:** Rebase sandbox trước khi merge. Nhưng rebase có thể fail nếu conflict.

---

#### 🔴 Kịch bản thất bại #4: "Token explosion — X3 prompt quá lớn với cluster nhiều file"

**Mô tả:** P1 yêu cầu gửi toàn bộ file contents cho X3 Transform. Với cluster 8 files (default), mỗi file trung bình 500 dòng → 4000 dòng code. Prompt có thể 50K-100K tokens. Với plan phức tạp (5-7 steps), output có thể 20K-40K tokens.

**Hậu quả:**
- Vượt context window → LLM từ chối hoặc cắt output
- Token cost cao (100K input + 40K output = ~$0.40-$1.00/plan với GPT-4o)
- Thời gian phản hồi chậm (30-60 giây)

**Xác suất:** Cao với cluster lớn hoặc file dài.

**Giảm thiểu:** Giới hạn cluster size, dùng skeleton thay vì full content, hoặc chỉ gửi file bị ảnh hưởng.

---

#### 🔴 Kịch bản thất bại #5: "False positive verify — code pass go vet nhưng sai logic"

**Mô tả:** Verify hiện tại chỉ kiểm tra:
- AST parse (Go files) → đảm bảo syntax đúng
- Balanced braces (non-Go) → heuristic thô
- `go test` (optional, nếu `--apply-run-tests`)

**Không kiểm tra:**
- Logic correctness: function có hoạt động đúng không?
- Type compatibility: kiểu trả về có khớp với caller không?
- Interface satisfaction: type mới có implement interface không?
- Race conditions, deadlocks, memory leaks

**Kịch bản:** X3 sinh `validator.go` với function `ValidateEmail`, syntax đúng, `go vet` pass. Nhưng function signature sai (nhận `*User` thay vì `string`) → `user.go` gọi `ValidateEmail(user)` → **compile fail ở step sau, hoặc tệ hơn: compile pass nhưng runtime panic.**

**Hậu quả:** Verify pass, code được merge, nhưng broken ở runtime.

**Xác suất:** Thấp-Trung bình. LLM thường sinh đúng signature nếu context đủ. Nhưng rủi ro có thật với codebase phức tạp.

**Giảm thiểu:** Thêm `go build ./...` vào verify (không chỉ `go vet`). Nhưng vẫn không phát hiện được lỗi logic.

---

#### 🔴 Rủi ro kiến trúc bị bỏ qua

1. **Coupling giữa Transformer và Architect prompt**: Nếu architect thay đổi format output (thêm field mới, đổi tên field), transformer phải thay đổi theo. Hai prompt tightly coupled → dễ break khi thay đổi một bên.

2. **Silent data loss trong incremental rollback**: P7 rollback từng step bằng cách restore backup. Nhưng nếu step 2 sửa file mà step 1 đã backup → backup của step 2 không phải là trạng thái gốc mà là trạng thái sau step 1. Rollback step 2 sẽ restore về trạng thái sau step 1 (đã bị thay đổi), không phải trạng thái ban đầu.

3. **State explosion**: P1 + P6 + P7 tạo ra 3 loại state cần quản lý: (a) LLM conversation state, (b) git worktree state, (c) checkpoint chain. Debug khi có lỗi sẽ rất phức tạp — cần trace qua cả 3 hệ thống.

4. **Prompt injection / hallucination**: X3 nhận plan từ X2 architect. Nếu architect hallucinate (tạo plan không khả thi, sai file path), X3 sẽ cố gắng implement plan không khả thi → code rác. Không có validation logic nào kiểm tra tính khả thi của plan trước khi đưa vào X3.

---

### 10.5 Phán Quyết Cuối Cùng

Sau khi cân nhắc cả 4 bước — đặc biệt là các kịch bản thất bại ở Bước 4 — tôi điều chỉnh đề xuất:

#### Phương án tối ưu thực tế: P5 (Two-Phase Hybrid) + P6 (Sandbox wrapper)

**Lý do điều chỉnh:**

1. **P5 thực tế hơn P1:** Tận dụng `executeApplyPhase()` đã được test kỹ qua E2E tests (`apply_e2e_test.go`). Editor agent (LLM-as-editor) đã chứng minh khả năng tự sửa lỗi qua retry loop (TestE2E_EditWrongOldStr, TestE2E_ShellRunFailThenFix).

2. **Tránh "token explosion" của P1:** Không cần gửi toàn bộ file contents trong 1 prompt. Editor agent tự `file_read` khi cần, tiết kiệm token.

3. **Tránh "dependency failure cascade" của P7:** Editor agent xử lý toàn bộ plan trong 1 session, có thể tự điều chỉnh thứ tự. Nếu file A phụ thuộc file B, editor có thể tạo B trước, verify, rồi mới sửa A.

4. **P6 là safety net:** Mọi thay đổi được thực hiện trong sandbox. Nếu editor fail hoặc kết quả không như mong đợi → discard sandbox, không ảnh hưởng branch chính.

**Tổng điểm tổ hợp P5+P6: Quality 4 + Cost 2 + Safety 5 + Multi-lang 5 + Maintain 4 + Speed 5 = 25/30** (vượt P1 đơn thuần 21 điểm)

#### Implementation plan điều chỉnh:

| Giai đoạn | Nhiệm vụ | Effort |
|---|---|---|
| **G1 (2 ngày)** | Sửa architect prompt để sinh `suggestion_code` thô + Sửa `apply_task_system.md` để nhận multi-file plan | 2 ngày |
| **G2 (1 ngày)** | Implement sandbox (git worktree) wrapper trong `runCrossApply` | 1 ngày |
| **G3 (1 ngày)** | Kết nối: plans → apply_comments JSON → `executeApplyPhase` trong sandbox | 1 ngày |
| **G4 (1 ngày)** | E2E tests: extract → create file → modify callers → verify → merge | 1 ngày |
| **Tổng** | | **5 ngày** |

---

## 11. Bug Phân Tích: Review --apply Sửa Sai Dù Cùng 1 File

**Ngày:** 2026-08-09  
**Mức độ:** 🔴 Cao — --apply sửa code không khớp suggestion_code  
**Phạm vi:** Review mode --apply (LLM-as-editor qua xecuteApplyPhase)

### 11.1 Mô Tả Vấn Đề

Khi chạy ocr review --apply:
- Một số file được sửa **đúng** — code sau apply khớp với suggestion_code
- Một số file khác bị sửa **sai** — code không khớp suggestion_code, output markdown không hiển thị sai lệch
- **Không có cơ chế phát hiện** sự sai lệch giữa kết quả apply và suggestion_code gốc

### 11.2 Flow Hiện Tại (xecuteApplyPhase, agent.go)

| Bước | Hành động | Bug? |
|---|---|---|
| B1 | uildApplyCommentsJSON() — serialize 5 field: path, content, start_line, end_line, suggestion_code | ⚠️ Thiếu existing_code |
| B2 | Render APPLY_TASK prompt với {{apply_comments}} | ✅ |
| B3 | Backup file gốc (map[absPath][]byte) | ✅ |
| B4 | pplyRunner.RunPerFile() — LLM dùng file_read/file_edit/file_write/shell_run, max 15 rounds | ⚠️ Prompt không ràng buộc exact match |
| B5 | completed=true → xóa backup, giữ thay đổi | ⚠️ Không verify suggestion_code có trong file |
| B6 | fail/stop → restore backup | ✅ |

### 11.3 Root Cause — 5 Nguyên Nhân

**#1: Thiếu xisting_code trong apply prompt JSON** 🔴

uildApplyCommentsJSON không serialize cm.ExistingCode → LLM chỉ nhận suggestion_code (code mới) mà không có xisting_code (code cũ cần thay thế). LLM phải tự ile_read rồi **đoán** old_str dựa trên start_line/nd_line và content message. Đoán sai → edit sai vị trí.

Ví dụ:
`json
{
  "path": "handler/user.go",
  "start_line": 42,
  "end_line": 48,
  "content": "Consider adding context parameter",
  "suggestion_code": "func (h *Handler) GetUser(ctx context.Context, id int) (*User, error) { ... }"
  // ⚠️ existing_code BỊ THIẾU: "func (h *Handler) GetUser(id int) (*User, error) { ... }"
}
`

**#2: Prompt không ràng buộc exact match với suggestion_code** 🔴

Prompt hiện tại: "apply the fix" — không yêu cầu 
ew_str phải khớp CHÍNH XÁC suggestion_code. LLM có thể "sáng tạo" → tự cải thiện code → output khác suggestion_code. Code compile pass, nhưng markdown output vẫn hiển thị suggestion_code gốc (không được update).

**#3: Thiếu post-apply diff verification** 🟡

Sau khi applyRunner hoàn thành (completed=true), code chỉ kiểm tra: LLM có gọi 	ask_done không. Không kiểm tra: file sau apply có chứa suggestion_code không.

**#4: Multiple comments trên cùng file — conflict ngầm** 🟡

Khi file có 2+ comments: Comment A edit trước → file thay đổi → line numbers của Comment B lệch → old_str không còn khớp. LLM phải ile_read lại nhưng prompt không nhấn mạnh việc này.

**#5: Line numbers từ diff có thể không chính xác** 🟢

start_line/nd_line từ review (dựa trên git diff) có thể lệch so với file thực tế khi apply (file đã bị thay đổi giữa lúc review và apply).

### 11.4 Đề Xuất Sửa — 4 Hướng

| # | Hướng | Impact | Effort | Priority |
|---|---|---|---|---|
| 1 | Thêm xisting_code vào apply JSON | Trung bình | 30 phút | 🔴 P0 |
| 2 | Cứng hóa prompt: exact match + dùng existing_code | Cao | 1 giờ | 🔴 P0 |
| 3 | Post-apply verify: check suggestion_code trong file | Cao | 2 giờ | 🟡 P1 |
| 4 | Sequential per-comment apply, sort bottom-up | Rất cao | 1-2 ngày | 🟢 P2 |

### 11.5 Code Changes Cụ Thể

**Sửa 1: uildApplyCommentsJSON — thêm xisting_code** (agent.go ~dòng 1588)

`go
type applyComment struct {
    Path           string json:"path"
    StartLine      int    json:"start_line"
    EndLine        int    json:"end_line"
    Content        string json:"content"
    ExistingCode   string json:"existing_code"    // ← THÊM
    SuggestionCode string json:"suggestion_code"
}
// Trong vòng lặp:
items[i] = applyComment{
    Path:           cm.Path,
    StartLine:      cm.StartLine,
    EndLine:        cm.EndLine,
    Content:        cm.Content,
    ExistingCode:   cm.ExistingCode,    // ← THÊM
    SuggestionCode: cm.SuggestionCode,
}
`

**Sửa 2: pply_task_system.md — ràng buộc exact match**

`markdown
## CRITICAL RULE: EXACT MATCH
- new_str in file_edit MUST match suggestion_code EXACTLY (character-by-character)
- DO NOT improve, modify, or refactor the suggestion_code
- Use existing_code to locate the EXACT old_str in the file
- After file_edit, use file_read to verify the change matches suggestion_code

## Instructions
For each review comment:
1. file_read the target file
2. Verify existing_code exists in file (if not found, skip with warning)
3. file_edit with old_str = existing_code, new_str = suggestion_code
4. file_read to confirm edit matches suggestion_code exactly
5. shell_run to verify compilation
6. If any step fails, retry once then skip
`

**Sửa 3: Post-apply verification** (agent.go, xecuteApplyPhase, sau dòng completed check)

`go
if completed {
    // Post-apply verify: check suggestion_code is in the file
    for _, cm := range actionable {
        if cm.SuggestionCode == "" { continue }
        abs := filepath.Join(a.args.RepoDir, filepath.FromSlash(cm.Path))
        current, err := os.ReadFile(abs)
        if err != nil {
            fmt.Fprintf(stdout.Writer(), "[ocr] Agentic apply: cannot verify %s: %v\n", cm.Path, err)
            continue
        }
        if !strings.Contains(string(current), cm.SuggestionCode) {
            fmt.Fprintf(stdout.Writer(), 
                "[ocr] Agentic apply: WARNING — suggestion_code not found in %s after apply\n", cm.Path)
        }
    }
    // Clear backups
    for abs := range backup { delete(backup, abs) }
}
`

### 11.6 Kế Hoạch Triển Khai

| # | Task | File | Effort | Priority |
|---|---|---|---|---|
| 11.1 | Thêm xisting_code vào uildApplyCommentsJSON | gent.go | 30 phút | P0 |
| 11.2 | Cập nhật pply_task_system.md — exact match + existing_code | pply_task_system.md | 30 phút | P0 |
| 11.3 | Post-apply verify: check suggestion_code trong file | gent.go | 1 giờ | P1 |
| 11.4 | Sort comments bottom-up trước khi apply | gent.go | 30 phút | P1 |
| 11.5 | E2E test: verify exact match sau apply | pply_e2e_test.go | 1 ngày | P1 |
| 11.6 | Sequential per-comment apply (nếu P1 chưa đủ) | gent.go | 1-2 ngày | P2 |

**Tổng P0+P1:** ~1-2 ngày | **Tổng P0+P1+P2:** ~3-4 ngày
