---
name: openmemory-sync
description: "[PhapChe] OpenMemory skill — learn từ tài liệu, query-before-decide mỗi khi ra quyết định, capture sau hành động quan trọng. Chỉ chạy trong project PhapChe."
version: 1.0.0
project: PhapChe
---

# OpenMemory Sync — PhapChe

Skill 3 chức năng dành riêng cho project PhapChe.

---

## QUY TẮC CỨNG: Query-Before-Decide

**Trước mỗi quyết định quan trọng**, PHẢI gọi `mcp__openmemory__openmemory_query` để tra cứu context cũ.

### Trigger (khi nào phải query)
- Chọn library, pattern, architecture approach
- Đụng đến area đã có gotcha/rules
- User nhắc đến việc đã làm trước đó
- Trước khi viết code mới trong phase/feature

### Cách query
```
mcp__openmemory__openmemory_query({ query: "<mô tả quyết định>", k: 10 })
```

### Output
```
[Memory] Found N items — áp dụng vào quyết định hiện tại:
- [gotcha] Không chạy taskkill /F /IM node.exe
- [decision] Dùng Better Auth thay NextAuth
...
```

---

## 1. Learn — `/memory-learn <path>`

Nạp knowhow từ file/URL vào OpenMemory.

### Flow
1. Đọc file được chỉ định
2. Trích xuất: decisions, patterns, gotchas, domain context
3. Store với tags phù hợp:
   - `type:decision` — quyết định kiến trúc
   - `type:pattern` — code pattern, convention
   - `type:gotcha` — cấm, lỗi đã gặp
   - `type:context` — domain knowledge
4. Tag thêm `source:<filename>` và `project:phapche`
5. Báo cáo: bao nhiêu items đã lưu

### Tài liệu mặc định nên học
- `CLAUDE.md`
- `.planning/phases/73-shared-foundation/73-SPEC.md`
- `src/docs/*.md`
- `src/styles/tokens.css`

---

## 2. Manual — `/memory-save <content> [--tags ...]`

```
/memory-save "Dùng Tailwind CSS, không dùng Ant Design" --tags type:pattern,source:manual
```

---

## 3. Find — `/memory-find <query>`

```
/memory-find "CSS architecture"
```

---

## Tag Taxonomy

| Tag | Ý nghĩa |
|-----|---------|
| `type:decision` | Quyết định kiến trúc |
| `type:pattern` | Code pattern, convention |
| `type:gotcha` | Cấm, lỗi, bẫy |
| `type:context` | Domain knowledge |
| `type:phase` | Tổng kết phase |
| `project:phapche` | Luôn có trong mọi memory |
