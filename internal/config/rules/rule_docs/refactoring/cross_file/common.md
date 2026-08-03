#### Cross-File Refactoring Rules

Focus **only** on multi-file boundaries. Do **not** report pure single-file local refactors (guard clauses, local renames, in-file extract) — those belong to the per-file pass.

##### Priority
- **P0**: Shared mutable state / unsafe coupling across files with clear evidence in the cluster
- **P1**: Verbatim or near-duplicate logic across ≥2 files; extract-shared candidates; data clumps; dead exports when graph-backed
- **P2**: Large package redesign, Strategy/pattern introductions — only if confidence is VERY_HIGH

##### Evidence
- Only cite files present in the cluster / provided slices
- Prefer signature + slice evidence over speculation
- One finding per clone/smell group (not one comment per file copy)

##### Extract shared
- Prefer **unexported helper in the same package** before new public APIs or new packages
- Propose target path + symbol name; list call sites in related_locations
- Preserve observable behavior; note breaking changes explicitly

##### Catalog IDs
- `REF-XDUP-001` — duplicated flow / near-clone across files
- `REF-XEXTRACT-001` — extract shared helper/module
- `REF-XCLUMP-001` — data clump across APIs
- `REF-XCOUPLE-001` — feature envy / tight coupling
- `REF-XAPI-001` — inconsistent cross-file API naming
- `REF-XDEAD-001` — dead export (requires graph/search evidence)

##### Output discipline
- Detector: JSON smells only (no prose wrapper)
- Architect: JSON plan with topological step order (base/helper before callers)
- Do not invent files outside the cluster
