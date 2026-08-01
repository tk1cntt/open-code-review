#### F# Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be decomposed by responsibility
- Functions with >4 parameters should use a record type for parameter grouping
- Deeply nested match expressions (>3 levels) should use active patterns or helper functions
- Long discriminated union types with many cases (>10) may need splitting by concern
- Complex computation expressions should be extracted to named functions

##### Naming
- Use camelCase for functions, values, locals; PascalCase for types, modules, discriminated union cases
- Boolean functions should use `is`/`has`/`can`/`should` prefix (e.g., `isValid`, `hasChildren`)
- Modules should use PascalCase; avoid generic names like `Utils`, `Helpers`
- Active patterns should use descriptive case names (`|Valid|Invalid|` not `|Case1|Case2|`)

##### Immutability
- Prefer immutable records and discriminated unions over mutable classes
- `mutable` fields in records — consider `with` copy-and-update instead
- `ref` cells in modern F# — use `mutable` or accumulator pattern
- `System.Collections.Generic.List<T>` used when `list` or `array` would suffice

##### Type Design and Sealed Types
- Discriminated unions should be preferred for modeling closed state machines
- Use `Result<'TOk, 'TError>` for operations that can fail with known error types
- Single-case discriminated unions for newtype wrappers around primitives
- Exhaustive match used on discriminated unions — compiler warnings enabled

##### Option and Error Handling
- Nested `match` on `Option`/`Result` chains — use `Option.map`/`bind` or `Result` computation expression
- `try/with` in every function — push error handling to boundaries with `Result`
- Repeated `Option.defaultValue` with the same default — extract to a helper

##### Async Patterns
- Mixing `async {}` and `task {}` inconsistently — standardize on one per module boundary
- Sequential `let!` where `Async.Parallel` or `Task.WhenAll` would be concurrent
- `Async.Ignore` on tasks that may fail — add error observation

##### Duplication
- Identical function logic across modules should be parameterized or extracted to shared module
- Repeated type annotations on let bindings — use module-level type annotations or inferred types
- Copy-pasted match expressions across functions — extract to shared active pattern

##### Dead Code and Simplification
- Unused let bindings, types, and open declarations should be removed
- Commented-out code blocks should be deleted
- Lambda wrapping a function call (`fun x -> f x`) — use function directly when signatures match
- `match x with | true -> ... | false -> ...` on boolean — use `if/then/else`

##### Pipeline and Composition
- Deeply nested function calls — use `|>` pipeline for readability
- Long pipeline chains with anonymous functions — extract named functions
- `x |> f` used when `f x` is simpler for a single application

##### Data and State
- Primitive obsession (string for email, int for status) — use single-case DUs
- Mutable `Dictionary`/`HashSet` in functional pipeline — use `Map`/`Set`/`seq` expressions
- Passing groups of related data as separate parameters — use a record

##### Coupling and Dependency
- Direct `DateTime.Now`/`File.ReadAllText`/`HttpClient` — inject or pass as function parameter
- Static dependencies in modules — prefer dependency injection via function parameters
- Hardcoded configuration paths — pass config as parameter or reader monad

##### Testability
- Direct `DateTime.UtcNow` — pass as function parameter or use `TimeProvider` (.NET)
- Direct file system access — inject `IFileSystem` or pass by parameter
- Tests that rely on real network — inject HTTP handler factory

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
