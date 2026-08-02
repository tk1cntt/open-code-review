#### Kotlin Refactoring Rules (delta)

Overrides common where noted.

##### Naming and Style
- camelCase functions; PascalCase classes; UPPER_SNAKE_CASE constants
- Avoid stutter: `UserValidator.validateUser()` → `UserValidator.validate()`
- Prefer `it` only for clear single-param lambdas

##### Function and Expression Conciseness
- Single-expression functions should use `=` body syntax
- Prefer `when` over long `if-else if` on the same value
- Prefer trailing-lambda syntax for last-parameter lambdas

##### Null Safety
- Avoid `!!` — prefer `?.`, Elvis `?:`, or `?: return`
- Repeated null checks → `?.let` or early guard
- Prefer `by lazy {}` or defaults over unsafe `lateinit` when possible

##### Collection Operation Optimization
- Long `map`/`filter` chains on large collections → `asSequence()` when intermediate lists hurt
- Prefer stdlib (`groupBy`, `associate`, `partition`) over manual loops
- Avoid constructing heavy objects (e.g. `Regex`) inside hot loops

##### Coroutine Correctness
- Replace `GlobalScope` with structured concurrency (`coroutineScope`, lifecycle scopes)
- `async` without `await`/error handling risks silent failure
- Blocking calls in coroutines → `delay` or `withContext(Dispatchers.IO)`
- Long suspend work should respect cancellation (`yield` / cooperative checks)

##### Class and Object Design
- Data-only types → `data class`
- Restricted hierarchies → `sealed class` / `sealed interface` for exhaustive `when`
- Prefer property delegation (`by lazy`, `observable`) over manual getters
- Prefer class delegation (`by`) over manual decorator forwarding

##### Resource Management
- `Closeable` resources must use `.use { }`

##### Error Handling
- Prefer `require` / `check` at entry over manual if-throw
- Do not ignore `runCatching` results

##### Testability
- Injectable `Clock` instead of `System.currentTimeMillis()` / `Instant.now()`
- Inject filesystem and HTTP clients
