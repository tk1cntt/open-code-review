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