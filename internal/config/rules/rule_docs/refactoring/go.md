#### Go Refactoring Rules (delta)

Overrides common where noted. Prefer small, focused functions (Go idiom may prefer smaller than common's 80-line threshold).

##### Naming
- Exported identifiers must be self-documenting and follow Go conventions
- Single-letter names only for receivers or short-loop iterators
- Boolean variables may use `ok` in addition to `is`/`has`/`can`/`should`
- Avoid stutter: `user.UserService` → `user.Service`

##### Error Handling
- Never ignore errors — handle or wrap with `fmt.Errorf("context: %w", err)`
- Prefer `errors.Is` / `errors.As` over direct error value comparison
- Deferred close with error check: `defer func() { if err := f.Close(); err != nil { ... } }()`
- Sequential `if err != nil` chains may use `errgroup` where concurrent work is intended

##### Concurrency
- Goroutines must have a clear lifecycle — `sync.WaitGroup`, `errgroup`, or channels
- Avoid fire-and-forget goroutines
- Shared mutable state must use `sync.Mutex` or channels
- Channel ownership (who closes) should be clear

##### Dead Code and Simplification
- Redundant `nil` checks before `len()` are unnecessary
- Empty `init()` functions should be deleted

##### Control Flow
- Prefer `if val, ok := m[key]; ok { ... }` over nested map access checks
- `select` with many cases may split into separate goroutines

##### Coupling and Dependency
- Interfaces should be defined at the call site, not alongside the implementation
- Avoid `interface{}` / `any` unless necessary — prefer generics or concrete types
- `context.Context` must never be stored in a struct; pass as first parameter
- Package-level mutable variables hurt testability

##### Testability
- `time.Now()` → injectable clock
- `os.Open` / `os.ReadFile` → `fs.FS` where practical
- `http.Get` / `http.DefaultClient` → injectable client
- Global `math/rand` → pass `*rand.Rand`
