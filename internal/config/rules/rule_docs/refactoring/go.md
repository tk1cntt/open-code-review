#### Go Refactoring Rules

##### Complexity
- Functions exceeding 80 lines should be split by responsibility (Go idiom: small, focused functions)
- Functions with >4 parameters should group related params into a struct
- Nested `if` >3 levels deep should use guard clauses with early returns (`if err != nil { return }`)
- Switch statements with >7 cases may benefit from a map-based dispatch table

##### Naming
- Exported identifiers must follow Go naming conventions and be self-documenting
- Single-letter variable names only for receivers (`func (s *Service)`) or short-loop iterators
- Boolean variables should use `is`/`has`/`can`/`should`/`ok` prefixes
- Avoid stutter in names: `user.UserService` → `user.Service`

##### Error Handling
- Never ignore errors silently — always handle or wrap with `fmt.Errorf("context: %w", err)`
- Repeated error-wrapping patterns should be extracted into helper functions
- Generic error messages should include what operation failed
- Prefer `errors.Is()` / `errors.As()` over direct comparison of error values
- Deferred resource cleanup with error checking: `defer func() { if err := f.Close(); err != nil { ... } }()`

##### Duplication
- Identical code blocks in the same file should be extracted into unexported helper functions
- Similar logic differing only by type suggests generics (Go 1.18+ type parameters)
- Repeated test setup should use table-driven tests or `t.Helper()` helpers
- Repeated `if err != nil { return ... }` blocks in sequence may benefit from error-group patterns

##### Concurrency
- Goroutines must have a clear lifecycle — use `sync.WaitGroup`, `errgroup`, or channels
- Avoid "fire and forget" goroutines — always coordinate completion
- Shared mutable state must be protected by `sync.Mutex` or use channels
- Channel ownership (who closes) should be clearly documented

##### Dead Code and Simplification
- Unused exported functions and types should be removed
- Unused imports should be cleaned (gofmt handles this automatically)
- Redundant `nil` checks before `len()` are unnecessary in Go
- Redundant type conversions should be removed
- Empty `init()` functions should be deleted

##### Control Flow
- Nested `if err != nil` can often be flattened with early returns
- Repeated condition checks suggest missing interface abstraction
- `if val, ok := m[key]; ok { ... }` is preferred over nested map access checks
- `select` with many cases may benefit from splitting into separate goroutines

##### Data and State
- Primitive types used as domain concepts (e.g., `string` for email) should be defined as named types
- Struct fields should use consistent alignment; group related fields
- Exported struct fields expose implementation details — use getter methods when abstraction matters
- Slice/map fields returned from exported methods invite mutation — consider returning copies

##### Coupling and Dependency
- Interfaces should be defined at the call site, not alongside the implementation
- `interface{}` / `any` should be avoided unless strictly necessary — prefer generics or concrete types
- `context.Context` should never be stored in a struct; pass as first parameter
- Package-level mutable variables reduce testability — consider dependency injection

##### Testability
- Direct `time.Now()` calls should use a `clock` interface for testing
- Direct `os.Open`/`os.ReadFile` — consider `fs.FS` interface for testability
- Direct `http.Get` / `http.DefaultClient` — use injectable HTTP client
- Global random source (`math/rand.Intn`) — pass `*rand.Rand` for determinism

For each finding, indicate severity (blocker/critical/major/minor/info), confidence (VERY_HIGH/HIGH/MEDIUM/LOW), rule reference, and include before/after code snippets.
