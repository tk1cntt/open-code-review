#### Julia Refactoring Rules

##### Type Stability
- Functions returning different types depending on runtime values should be stabilized with concrete return types
- Struct fields using abstract types (`Real`, `AbstractArray`, `Any`) should use concrete types or type params
- Containers initialized without element types (`[]`, `Vector{Any}()`, `Dict()`) should declare concrete types
- Loop variables that change type across iterations should be stabilized

##### Function and Method Design
- Functions exceeding 80 lines should be decomposed by responsibility
- Multiple dispatch methods typed as `::Any` (or untyped) should narrow their type constraints
- Overly broad method signatures that silently capture unrelated types should be constrained
- Functions with >4 parameters should consider grouping into a struct or named tuple

##### Performance
- Non-const global variables read inside hot functions should be `const` or passed as arguments
- Unnecessary allocations in hot loops: repeated array creation, `String` concatenation, slicing copies
- Growing arrays without `sizehint!` when final size is known
- Performance-critical struct fields left abstract should be concrete types

##### Naming
- Use snake_case for functions and variables; PascalCase for types and modules
- Single-letter names only for short-loop iterators; use descriptive names in wider scopes
- Boolean functions should use `is_`/`has_`/`can_`/`should_` prefixes
- Avoid name conflicts with `Base` functions unless intentionally overloading

##### Duplication
- Identical function bodies with different type signatures may suggest parametric methods
- Repeated setup/teardown patterns should use `do` blocks or helper functions
- Similar dispatch methods should be consolidated with union types or abstract types

##### Error Handling
- `@assert` for input validation (may be disabled) should be explicit `throw(ArgumentError(...))`
- Empty catch blocks should at minimum log; swallowing exceptions silently is risky
- Functions mixing `throw` and `return nothing` for the same failure should pick one strategy
- Broad `catch` blocks should narrow to specific exception types

##### Dead Code
- Unused exported functions and types should be removed
- Unreachable code branches should be eliminated
- Commented-out code blocks should be deleted

##### Control Flow
- Deeply nested if-else chains should use guard clauses with early returns
- Repeated condition checks across methods may benefit from dispatch
- Boolean flag parameters suggest separate functions

##### Data and State
- Global mutable state should be minimized; pass explicitly as function arguments
- Shared mutable state across tasks needs synchronization
- Groups of related values suggest a struct rather than separate variables

##### Testability
- Direct `time()` calls should use injectable clock
- Direct file I/O should be behind injectable paths or streams
- Random number usage without seeding for reproducibility should allow seed injection

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
