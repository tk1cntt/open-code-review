#### F# Refactoring Rules (delta)

Overrides common where noted.

##### Naming
- camelCase functions/values; PascalCase types/modules/DU cases
- Active patterns use descriptive names

##### Immutability
- Prefer immutable records and DUs over mutable classes
- Prefer `with` updates over `mutable` fields when practical
- Prefer F# `list`/`array` over mutable BCL lists unless needed

##### Type Design
- Prefer DUs for closed state machines
- Prefer `Result<'T, 'E>` at boundaries
- Prefer single-case DUs for newtypes
- Keep exhaustive matches (enable warnings)

##### Option and Error Handling
- Prefer `Option`/`Result` map/bind or computation expressions over nested matches
- Push errors to boundaries rather than try/with everywhere

##### Async Patterns
- Standardize on `async {}` or `task {}` per boundary
- Prefer parallel combinators when work is independent
- Do not `Async.Ignore` failing work without observation

##### Pipeline
- Prefer `|>` pipelines over deep nesting
