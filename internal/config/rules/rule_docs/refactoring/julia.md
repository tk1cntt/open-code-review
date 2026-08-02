#### Julia Refactoring Rules (delta)

Overrides common where noted.

##### Type Stability
- Stabilize return types; avoid runtime-dependent return type branches in hot code
- Prefer concrete field types or type parameters over abstract fields (`Any`, `Real`)
- Initialize containers with concrete element types
- Avoid loop variables that change type across iterations

##### Function and Method Design
- Narrow multiple-dispatch methods typed as `::Any` when domain allows
- Prefer parametric methods over copy-pasted bodies per type

##### Performance
- Avoid non-const globals in hot functions — pass as args or use `const`
- Reduce allocations in hot loops; use `sizehint!` when final size is known
- Keep performance-critical struct fields concrete

##### Naming
- snake_case functions/vars; PascalCase types/modules
- Boolean prefixes: `is_`/`has_`/`can_`/`should_`
- Avoid accidental Base name clashes unless intentionally overloading

##### Error Handling
- Prefer explicit `throw(ArgumentError(...))` over `@assert` for validation
- Prefer one failure strategy (`throw` vs `nothing`) per API
